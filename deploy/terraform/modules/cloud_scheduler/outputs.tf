output "job_names" {
  description = "Names of the created scheduler jobs."
  value       = [for j in google_cloud_scheduler_job.jobs : j.name]
}
