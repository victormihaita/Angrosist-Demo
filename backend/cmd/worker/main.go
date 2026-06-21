// Command worker is the Cloud Tasks push target that runs asynchronous agent
// turns.
//
// TODO(M2): implement the async agent-turn worker. Per
// docs/specs/ARCHITECTURE_DETAIL.md §4, this binary will: acquire a
// per-conversation advisory lock, re-check inbound idempotency, load
// conversation state, drive the LLM/tool loop through the agent core, persist
// results, send the reply through the resolved Channel, and classify failures as
// retryable vs terminal for Cloud Tasks. None of that lands until M2; this is a
// build-able skeleton only.
package main

import "log"

func main() {
	log.Println("worker: not yet implemented (lands in M2)")
}
