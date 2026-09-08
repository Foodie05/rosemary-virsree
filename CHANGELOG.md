# Changelog

## v0.2.0 — 2026-09-08

- Added OIDC Authorization Code + PKCE console login with an exact email allowlist and secure server-side sessions.
- Added the guided welcome → verified storage source → console OOBE.
- Added encrypted multi-source S3 and WebDAV configuration, source priority/fallback allocation, and physical source capacity accounting.
- Added full data-plane probes before a source is saved, separate S3 upload/download endpoints, and an optional S3-compatible CDN download endpoint.
- Added short-lived WebDAV transfer capabilities with explicit `direct: false` responses; S3 data remains direct with `direct: true`.
- Added a production configuration uploader, hardened systemd unit, isolated Apache vhost template, and a complete operator guide.

## v0.1.0 — 2026-09-08

- Added a centralized Go gateway with virtual buckets, virtual AK/SK credentials, independent read/write/delete/manage permissions, audit records, and total/per-bucket quotas.
- Added S3-compatible ListObjectsV2, HeadObject, redirected GetObject, and DeleteObject. PutObject is intentionally rejected.
- Added presigned direct upload with commit verification and direct signed downloads, so object bodies bypass Rosemary.
- Added stable public aliases, physical-key rotation for immediate old-link invalidation, and application-selected signature expiry.
- Added the React management console, object operations, one-time Agent onboarding, comprehensive integration documentation, Docker deployment, and cross-platform `rvsctl` releases.
