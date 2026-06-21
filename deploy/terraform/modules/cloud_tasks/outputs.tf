output "queue_id" {
  description = "Fully-qualified Cloud Tasks queue ID."
  value       = google_cloud_tasks_queue.agent_turns.id
}

output "queue_name" {
  description = "Cloud Tasks queue name."
  value       = google_cloud_tasks_queue.agent_turns.name
}
