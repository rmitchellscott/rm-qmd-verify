package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/rmitchellscott/rm-qmd-verify/internal/config"
	"github.com/rmitchellscott/rm-qmd-verify/internal/handlers"
	"github.com/rmitchellscott/rm-qmd-verify/internal/jobs"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmldiff"
	"github.com/rmitchellscott/rm-qmd-verify/internal/version"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/qmltree"
)

//go:embed ui/dist
//go:embed ui/dist/assets
var embeddedUI embed.FS

func main() {
	if err := godotenv.Load(); err != nil {
		logging.Info(logging.ComponentStartup, "No .env file found, using environment variables")
	}

	logging.Info(logging.ComponentStartup, "Starting rm-qmd-verify %s", version.GetFullVersion())

	hashtabDir := config.Get("HASHTAB_DIR", "./hashtables")
	logging.Info(logging.ComponentStartup, "Loading hashtables from: %s", hashtabDir)

	hashtabService, err := hashtab.NewService(hashtabDir)
	if err != nil {
		logging.Error(logging.ComponentStartup, "Failed to initialize hashtab service: %v", err)
		os.Exit(1)
	}

	hashtables := hashtabService.GetHashtables()
	logging.Info(logging.ComponentStartup, "Loaded %d hashtables", len(hashtables))
	for _, ht := range hashtables {
		logging.Info(logging.ComponentStartup, "  - %s (%d entries)", ht.Name, len(ht.Entries))
	}

	treeDir := config.Get("QML_TREE_DIR", "./qml-trees")
	logging.Info(logging.ComponentStartup, "Loading QML trees from: %s", treeDir)

	treeService := qmltree.NewService(treeDir)
	trees := treeService.GetTrees()
	logging.Info(logging.ComponentStartup, "Loaded %d QML trees", len(trees))
	for _, tree := range trees {
		logging.Info(logging.ComponentStartup, "  - %s (%s / %s, %d files)", tree.Name, tree.OSVersion, tree.Device, tree.FileCount)
	}

	qmldiffBinary := config.Get("QMLDIFF_BINARY", "./qmldiff")
	qmldiffService := qmldiff.NewService(qmldiffBinary, hashtabService, treeService)
	logging.Info(logging.ComponentStartup, "Initialized qmldiff service (binary: %s)", qmldiffBinary)

	jobStore := jobs.NewStore()

	maxConcurrentValidations := config.GetInt("MAX_CONCURRENT_VALIDATIONS", 15)
	logging.Info(logging.ComponentStartup, "Max concurrent validations: %d", maxConcurrentValidations)

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	apiHandler := handlers.NewAPIHandler(qmldiffService, hashtabService, treeService, jobStore, maxConcurrentValidations)
	r.Route("/api", func(r chi.Router) {
		r.Post("/compare", apiHandler.Compare)
		r.Post("/validate/tree", apiHandler.ValidateTree)
		r.Get("/hashtables", apiHandler.ListHashtables)
		r.Get("/trees", apiHandler.ListTrees)
		r.Get("/validated-versions", apiHandler.ListValidatedVersions)
		r.Get("/results/{jobId}", apiHandler.GetResults)
		r.Get("/status/ws/{jobId}", handlers.StatusWSHandler(jobStore))
		r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(version.Get())
		})
	})

	uiFS, err := fs.Sub(embeddedUI, "ui/dist")
	if err != nil {
		logging.Error(logging.ComponentStartup, "Failed to load embedded UI: %v", err)
		os.Exit(1)
	}

	fileServer := http.FileServer(http.FS(uiFS))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if _, err := uiFS.Open(path[1:]); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	port := config.Get("PORT", "8080")
	addr := fmt.Sprintf(":%s", port)
	logging.Info(logging.ComponentServer, "Starting server on %s", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Error(logging.ComponentServer, "Failed to start server: %v", err)
			os.Exit(1)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logging.Info(logging.ComponentServer, "Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logging.Error(logging.ComponentServer, "Error shutting down server: %v", err)
		os.Exit(1)
	}

	logging.Info(logging.ComponentServer, "Server shutdown complete")
}
