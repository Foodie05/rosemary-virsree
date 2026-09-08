# Changelog

## v0.2.2 — 2026-09-08

- Make VirSree the primary product identity across the console, OOBE, Agent prompt, documentation, CLI, and operational messages; retain Rosemary as the parent brand endorsement.
- Accept common endpoint input such as `.s3.example.com`, `s3.example.com`, `//s3.example.com`, and missing-scheme variants by normalizing them to a valid HTTPS URL in both the console and server.
- Replace raw provider failures with stable error codes, actionable Chinese and English messages selected from `Accept-Language`, and a per-request trace ID.
- Add structured, redacted request and storage-verification logs that omit credentials, signed URLs, raw endpoint hosts, bucket names, object keys, query strings, and bodies.

## v0.2.1 — 2026-09-08

- Validate OIDC ID Tokens through Discovery and JWKS, including RS256 signatures, issuer, audience, time claims, nonce, and UserInfo subject binding.

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
- Added presigned direct upload with commit verification and direct signed downloads, so object bodies bypass VirSree.
- Added stable public aliases, physical-key rotation for immediate old-link invalidation, and application-selected signature expiry.
- Added the React management console, object operations, one-time Agent onboarding, comprehensive integration documentation, Docker deployment, and cross-platform `rvsctl` releases.
