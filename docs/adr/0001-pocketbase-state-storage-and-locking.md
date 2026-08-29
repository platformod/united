# Use PocketBase for Terraform state storage and locking

**Status: accepted**

United will replace its Gin/S3/KMS/Redis persistence stack with a single PocketBase application. Terraform routes will use PocketBase custom handlers and record operations, with encrypted state versions stored in protected file fields and lock/current-version metadata stored on logical state records. This preserves the Terraform HTTP contract while removing AWS and Redis dependencies; the deliberate consequence is that PocketBase file cleanup is compensating rather than fully cross-store atomic, so unreachable encrypted files may remain until future retention cleanup.
