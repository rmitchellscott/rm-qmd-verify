package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/rmitchellscott/rm-qmd-verify/internal/config"
	"github.com/rmitchellscott/rm-qmd-verify/internal/handlers"
	"github.com/rmitchellscott/rm-qmd-verify/internal/jobs"
	"github.com/rmitchellscott/rm-qmd-verify/pkg/hashtab"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmldiff"
	"github.com/rmitchellscott/rm-qmd-verify/internal/version"
)

var embeddedUI embed.FS

func SetEmbeddedUI(ui embed.FS) {
	embeddedUI = ui
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server",
	Long:  "Start the HTTP server with the web UI and API endpoints",
	Run:   runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) {
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

	qmldiffService := qmldiff.NewService("", hashtabService)
	jobStore := jobs.NewStore()

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	apiHandler := handlers.NewAPIHandler(qmldiffService, hashtabService, jobStore)
	r.Route("/api", func(r chi.Router) {
		r.Post("/compare", apiHandler.Compare)
		r.Get("/hashtables", apiHandler.ListHashtables)
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

	if err := http.ListenAndServe(addr, r); err != nil {
		logging.Error(logging.ComponentServer, "Failed to start server: %v", err)
		os.Exit(1)
	}
}
