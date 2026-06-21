// Package worker provides the HTTP handler for the asynchronous agent-turn
// worker. It is the Cloud Tasks push target: Cloud Tasks (or the Cloud Tasks
// adapter's HTTP push) POSTs a JSON ports.TurnJob, and the handler runs it
// through the TurnProcessor. The HTTP status it returns is the retry signal —
// 2xx acks the job, 5xx asks the transport to retry.
package worker

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/angrosist/demo/internal/ports"
)

const maxBodyBytes = 64 << 10 // 64 KB — turn jobs are small.

// Handler decodes a TurnJob and invokes the processor.
type Handler struct {
	processor ports.TurnProcessor
}

// NewHandler constructs the worker HTTP handler bound to a turn processor.
func NewHandler(processor ports.TurnProcessor) *Handler {
	return &Handler{processor: processor}
}

// ServeHTTP handles POST /worker/turn.
//
// Status mapping (the retry contract for the push transport):
//   - 400: malformed body (terminal — do not retry).
//   - 200: processed, or a terminal outcome that the processor logged and
//     swallowed (ack — do not retry).
//   - 500: retryable failure (the transport should retry with backoff).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var job ports.TurnJob
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		http.Error(w, "invalid job body", http.StatusBadRequest)
		return
	}
	if job.ConversationID == "" {
		http.Error(w, "conversation_id is required", http.StatusBadRequest)
		return
	}

	if err := h.processor.Process(r.Context(), job); err != nil {
		// Retryable: ask the transport to redeliver.
		log.Printf("worker: retryable turn failure conversation=%s provider_msg_id=%q: %v",
			job.ConversationID, job.ProviderMsgID, err)
		http.Error(w, "retry", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
