output "registry_base" { value = module.platform.registry_base }
output "api_url" { value = module.platform.api_url }
output "worker_url" { value = module.platform.worker_url }
output "cloudsql_connection_name" { value = module.platform.cloudsql_connection_name }
output "documents_bucket" { value = module.platform.documents_bucket }
output "cloud_tasks_queue" { value = module.platform.cloud_tasks_queue }
output "deployer_sa_email" { value = module.platform.deployer_sa_email }
output "secret_ids" { value = module.platform.secret_ids }
