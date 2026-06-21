package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/angrosist/demo/internal/ports"
)

// CloudTasksConfig configures the Cloud Tasks push queue. All values come from
// config/env — nothing is hardcoded.
type CloudTasksConfig struct {
	// WorkerURL is the absolute URL of the worker's turn endpoint
	// (e.g. https://worker-xxx.run.app/worker/turn). Cloud Tasks POSTs the job
	// JSON here; in this implementation the adapter performs the HTTP push
	// directly (the real path), with the managed Cloud Tasks client wiring
	// deferred until GCP is provisioned.
	WorkerURL string
	// AuthToken, when set, is sent as a Bearer token on the push request so the
	// worker can authenticate Cloud Tasks. Optional for local pushes.
	AuthToken string
	// Timeout bounds a single push request.
	Timeout time.Duration
}

var _ ports.Queue = (*CloudTasks)(nil)

// CloudTasks is the production ports.Queue. It enqueues a turn job by POSTing the
// JSON-serialized job to the worker URL (the Cloud Tasks push target). Selected
// by QUEUE_PROVIDER=cloudtasks.
//
// Provisioning note: the managed Cloud Tasks client (which schedules a task that
// Cloud Tasks then pushes, providing durability + retry/backoff) is deferred
// until the GCP queue exists. To avoid a heavy GCP dependency before then, this
// adapter implements the real HTTP-push contract directly: the on-wire shape and
// the worker endpoint are identical to what the managed client will target, so
// swapping in the client later is contained to this file.
type CloudTasks struct {
	cfg    CloudTasksConfig
	client *http.Client
}

// NewCloudTasks constructs the Cloud Tasks push adapter. WorkerURL must be set.
func NewCloudTasks(cfg CloudTasksConfig) (*CloudTasks, error) {
	if cfg.WorkerURL == "" {
		return nil, fmt.Errorf("queue(cloudtasks): WORKER_URL is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &CloudTasks{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

// Enqueue pushes the job to the worker endpoint as a JSON POST. A non-2xx
// response or transport error is returned wrapped so the caller can decide
// whether to retry.
func (q *CloudTasks) Enqueue(ctx context.Context, job ports.TurnJob) error {
	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue(cloudtasks): marshal job: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.cfg.WorkerURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("queue(cloudtasks): build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if q.cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+q.cfg.AuthToken)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("queue(cloudtasks): push to worker: %w", err)
	}
	defer resp.Body.Close()
	// Drain so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("queue(cloudtasks): worker returned status %d", resp.StatusCode)
	}
	return nil
}
