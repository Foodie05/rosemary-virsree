# Changelog

## v0.2.7 — 2026-09-09

- Add Bitiful CDN advanced authentication with application-selected `_ts` expiry and server-side `_btf_tk` signing.
- Separate CDN authentication modes in storage source setup and explain where to find the Bitiful authentication key.
- Classify download CDN failures before generic network errors and add a redacted probe-stage log field.

## v0.2.6 — 2026-09-09

- Add a build-time version handshake between the VirSree console and gateway, with automatic cache-busting reloads when a newer server version is detected.
- Prevent the SPA entry HTML and version API from being cached while allowing fingerprinted static assets to use a one-year immutable cache policy.
- Publish matching frontend and backend version metadata in release and Docker builds, and show the active console version in the administrator identity area.

## v0.2.5 — 2026-09-09

- Add storage-source editing with full data-plane revalidation before an atomic configuration replacement; failed validation leaves the active source unchanged.
- Preserve encrypted credentials when edit fields are left blank, and prevent capacity from being reduced below used plus reserved space.
- Require an explicit acknowledgement when the physical S3 bucket changes, with a custom VirSree risk dialog that explains object migration and availability impact.
- Add redacted edit audit and rejection logs without exposing endpoints, bucket names, credentials, or provider error payloads.

## v0.2.4 — 2026-09-08

- Normalize bucket-scoped S3 hosts such as `bucket.s3.example.com` back to the SDK service endpoint so the bucket is not duplicated in the object path.
- Default new cloud S3 sources to virtual-hosted addressing while retaining an explicit Path-style option for MinIO and self-hosted services.
- Clean failed S3 probes through the same endpoint that accepted the direct upload, preventing misconfigured endpoints from leaving probe objects behind.
- Add request-specific guidance when a verification 404 used Path-style addressing.

## v0.2.3 — 2026-09-08

- Key endpoint correlation identifiers with the server master secret so redacted logs cannot be used to enumerate likely endpoint hostnames offline.
- Constrain logged storage-kind values to the known `s3`, `webdav`, or `invalid` labels.

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
