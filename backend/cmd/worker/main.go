// Command worker is the asynchronous agent-turn worker. It is the Cloud Tasks
// push target: per docs/specs/ARCHITECTURE_DETAIL.md §4 it acquires a
// per-conversation advisory lock, applies provider-message-id idempotency, and
// drives the agent turn through the shared agent core. It is built from the same
// composition root as the API server (internal/app), differing only in which
// HTTP surface it exposes.
//
// Endpoint:
//
//	POST /worker/turn   — decode a ports.TurnJob and run it through the processor.
//	                      2xx acks; 5xx asks the transport to retry (see §6).
//	GET  /healthz       — liveness.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/angrosist/demo/internal/api/worker"
	"github.com/angrosist/demo/internal/app"
)

func main() {
	c := app.GetContainer()

	mux := http.NewServeMux()
	mux.Handle("/worker/turn", worker.NewHandler(c.Worker))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	port := os.Getenv("WORKER_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8081"
	}

	log.Println("worker: listening on http://localhost:" + port + " (POST /worker/turn)")
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
