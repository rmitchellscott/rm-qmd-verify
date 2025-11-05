package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/rmitchellscott/rm-qmd-verify/internal/jobs"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
)

func StatusWSHandler(jobStore *jobs.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "jobId")
		if jobID == "" {
			http.Error(w, "Job ID required", http.StatusBadRequest)
			return
		}

		if _, ok := jobStore.Get(jobID); !ok {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			logging.Error(logging.ComponentHandler, "Failed to accept WebSocket: %v", err)
			return
		}

		ch, unsubscribe := jobStore.Subscribe(jobID)
		defer unsubscribe()

		ctx := r.Context()
		for job := range ch {
			if err := wsjson.Write(ctx, conn, job); err != nil {
				logging.Error(logging.ComponentHandler, "Failed to write WebSocket message: %v", err)
				return
			}

			if job.Status == "success" || job.Status == "error" {
				return
			}
		}
	}
}
