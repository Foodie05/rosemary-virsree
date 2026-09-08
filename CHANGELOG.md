# Changelog

## v0.1.0 — 2026-09-08

- Added a centralized Go gateway with virtual buckets, virtual AK/SK credentials, independent read/write/delete/manage permissions, audit records, and total/per-bucket quotas.
- Added S3-compatible ListObjectsV2, HeadObject, redirected GetObject, and DeleteObject. PutObject is intentionally rejected.
- Added presigned direct upload with commit verification and direct signed downloads, so object bodies bypass Rosemary.
- Added stable public aliases, physical-key rotation for immediate old-link invalidation, and application-selected signature expiry.
- Added the React management console, object operations, one-time Agent onboarding, comprehensive integration documentation, Docker deployment, and cross-platform `rvsctl` releases.
