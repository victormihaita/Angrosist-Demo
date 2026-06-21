// Package handler serves the Server-Sent Events (SSE) stream for the web widget:
// GET /api/stream?conversation_id=... A client subscribes to a conversation and
// receives agent turn events (typing/message/error) live, as they are published
// by the chat use-case and the async turn worker.
//
// DEPLOYMENT NOTE: SSE needs a long-lived connection. It is served by the
// long-running server (local dev / Cloud Run), NOT by Vercel — Vercel serverless
// functions do not hold open streaming connections reliably. The Vercel demo
// falls back to the synchronous reply from POST /api/chat (unchanged); this
// endpoint is simply unavailable there. Do not attempt to make SSE work on
// Vercel.
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/ports"
)

// maxConversationID bounds the conversation_id query param (matches the chat
// handler's cap).
const maxConversationID = 128

// heartbeatInterval is how often a keep-alive comment is sent to keep proxies and
// the client from closing an idle connection.
const heartbeatInterval = 20 * time.Second

// Handler returns the SSE handler bound to the given broker. cmd/server builds it
// from the container's broker so the same in-process broker the use-cases publish
// to is the one this stream subscribes to.
func Handler(broker ports.Broker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Reuse the shared origin-aware CORS (the widget is cross-origin by design).
		if httputil.HandleOptions(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			httputil.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// Validate the conversation_id query param (Hard Rule #6).
		convID := r.URL.Query().Get("conversation_id")
		if convID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "conversation_id is required")
			return
		}
		if len(convID) > maxConversationID {
			httputil.WriteError(w, http.StatusBadRequest, "invalid conversation_id")
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			// The server must support flushing for streaming; if not, fail loudly
			// rather than buffer the whole response.
			httputil.WriteError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}

		// SSE response headers.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		events, unsubscribe := broker.Subscribe(convID)
		defer unsubscribe()

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				// Client disconnected (or server shutting down): unsubscribe (defer)
				// and return cleanly.
				return

			case ev, open := <-events:
				if !open {
					// Subscription closed underneath us.
					return
				}
				if err := writeEvent(w, ev); err != nil {
					// Write failed (client gone): stop.
					return
				}
				flusher.Flush()

			case <-ticker.C:
				// Heartbeat comment keeps the connection alive through proxies.
				if _, err := w.Write([]byte(": ping\n\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// writeEvent serializes ev as one SSE frame:
//
//	event: <type>\n
//	data: <json>\n
//	\n
func writeEvent(w http.ResponseWriter, ev ports.Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: " + ev.Type + "\n")); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n\n"))
	return err
}
