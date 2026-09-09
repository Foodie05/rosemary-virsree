# Gateway API

The examples use one gateway origin, such as `https://storage.example.com`. Send the user's locale in `Accept-Language`. VirSree chooses Chinese for languages beginning with `zh` and English otherwise, while always returning both translations:

```json
{
  "error": "VirSree 已连接到存储服务，但找不到刚上传的验证对象……",
  "code": "storage_probe_not_found",
  "message_zh": "VirSree 已连接到存储服务，但找不到刚上传的验证对象……",
  "message_en": "VirSree reached the storage service but could not find the verification object…",
  "trace_id": "req_..."
}
```

`error` contains the selected language for backward compatibility. Applications should use `code` for program logic and show `error` to the user. Report `trace_id` when an operator needs to investigate; provider responses, credentials and signed URLs are never returned in the error body.

The `/s3` compatibility endpoint keeps the standard XML error shape expected by S3 SDKs. Its `Code` remains machine-readable, `Message` follows `Accept-Language`, and `RequestID` matches the `X-Request-ID` response header. Provider internals are omitted there as well.

## Authentication

The browser console uses an OIDC session cookie. Administrative automation may use `Authorization: Bearer <RVS_ADMIN_TOKEN>`.

Virtual application routes require:

```http
X-RVS-Access-Key: RVS...
X-RVS-Secret-Key: rvs_...
```

The `/s3` routes use AWS Signature Version 4 with the virtual AK/SK. Permissions are `read`, `write`, `delete`, and `manage`. A key may contain any combination. Permissions are independent; `manage` does not imply read, write, or delete.

## Administrative API

| Method | Route | Purpose |
|---|---|---|
| `GET` | `/api/v1/version` | Current server build version for client cache reconciliation |
| `GET` | `/api/v1/overview` | Capacity, bucket, object, key and backend status |
| `GET` | `/api/v1/setup/status` | OOBE completion and source count |
| `GET` | `/api/v1/storage-sources` | List redacted S3/WebDAV source metadata |
| `POST` | `/api/v1/storage-sources` | Verify and add an encrypted storage source |
| `GET` | `/api/v1/storage-sources/{id}` | Read editable connection settings without credential values |
| `PUT` | `/api/v1/storage-sources/{id}` | Revalidate and atomically replace a storage source |
| `GET` | `/api/v1/buckets` | List virtual buckets and usage |
| `POST` | `/api/v1/buckets` | Create a virtual bucket and one-time owner credential |
| `GET` | `/api/v1/access-keys` | List key metadata without secrets |
| `POST` | `/api/v1/buckets/{bucket}/access-keys` | Issue a key with selected permissions; return its secret once |
| `DELETE` | `/api/v1/access-keys/{id}` | Revoke a virtual key |
| `POST` | `/api/v1/bootstrap-tokens` | Create a one-use Agent token |
| `GET` | `/api/v1/admin/buckets/{bucket}/objects` | Browse objects by optional `prefix` |
| `POST` | `/api/v1/admin/buckets/{bucket}/objects/download` | Create an admin-selected direct download URL |
| `POST` | `/api/v1/admin/buckets/{bucket}/objects/invalidate-links` | Rotate one physical key |
| `DELETE` | `/api/v1/admin/buckets/{bucket}/objects/{key...}` | Delete one object |

An update completes the same full data-plane probe as creation before any persisted or runtime configuration changes. Empty credential fields retain the existing encrypted values. A failed probe leaves the active source untouched. Changing an S3 physical bucket requires `acknowledge_bucket_change: true`; clients should show a clear data-availability warning because VirSree does not move existing objects to the new bucket.

The console embeds its release version at build time and compares it with `/api/v1/version` on startup, once per minute, and whenever the tab regains focus. A mismatch reloads the page through a versioned cache-busting URL. The version endpoint and SPA HTML use `no-store`; fingerprinted `/assets/` files use a one-year immutable cache policy.

Create bucket body:

```json
{
  "name": "Media assets",
  "slug": "media-assets",
  "visibility": "private",
  "quota_bytes": 10737418240
}
```

Create bootstrap token body:

```json
{
  "name": "Media service onboarding",
  "max_quota_bytes": 10737418240,
  "expires_in": 1800
}
```

`expires_in` here is the lifetime of the one-time onboarding token. It is separate from object signature lifetimes.

S3 storage-source endpoints are SDK service endpoints, such as `https://s3.example.com`. If an operator pastes a standard bucket-scoped host such as `https://media.s3.example.com` while the bucket field is `media`, VirSree removes the duplicated bucket label before verification. New cloud sources default to virtual-hosted addressing; enable Path-style explicitly for MinIO or another provider that requires it.

## Agent exchange

`POST /api/v1/agent/claim` is unauthenticated except for the one-time token:

```json
{
  "token": "rvs_boot_...",
  "name": "Media assets",
  "slug": "media-assets",
  "visibility": "private",
  "quota_bytes": 10737418240
}
```

It returns the gateway S3 endpoint, virtual bucket, region, virtual AK/SK, and permissions exactly once.

## Object API

| Permission | Method | Route | Body |
|---|---|---|---|
| `write` | `POST` | `/api/v1/buckets/{bucket}/objects/upload` | `key`, `size`, `content_type`, required `expires_in` |
| `write` | `POST` | `/api/v1/buckets/{bucket}/objects/commit` | `upload_id`, `key` |
| `read` | `POST` | `/api/v1/buckets/{bucket}/objects/download` | `key`, optional `filename`, required `expires_in` |
| `manage` | `POST` | `/api/v1/buckets/{bucket}/objects/public-link` | `key`, required `sign_expires_in`, optional `link_expires_in` |
| `manage` | `DELETE` | `/api/v1/buckets/{bucket}/public-links/{slug}` | none; immediately stops future redirects |
| `manage` | `POST` | `/api/v1/buckets/{bucket}/objects/invalidate-links` | `key` |
| `delete` | `DELETE` | `/api/v1/buckets/{bucket}/objects/{key...}` | none |

Every capability request requires an application-selected positive expiry. S3-compatible backends using SigV4 cap it at 604800 seconds. VirSree does not shorten a valid requested duration. Responses include `direct`: S3 is `true`; generic WebDAV is `false` because the capability URL relays bytes through VirSree.

The public-link response contains `slug`, `public_url`, and `direct_url`. `public_url` is a VirSree alias that can keep working until `link_expires_in`; each visit redirects to a new real URL valid for `sign_expires_in`. Revoke it with the `slug`. Already issued `direct_url` values bypass VirSree and remain usable until their chosen expiry unless the object physical key is rotated.

## S3 endpoint

Configure clients with endpoint `https://storage.example.com/s3`, the returned region, virtual credentials, path-style addressing, and virtual bucket name.

| Operation | State | Behavior |
|---|---|---|
| ListObjectsV2 | Basic | Prefix filtering and up to 1000 mapped objects; delimiter and pagination are not implemented |
| HeadObject | Supported | Returns mapped metadata |
| GetObject | Supported with redirect | `307` to a real S3 presigned URL; expiry comes from virtual presign `X-Amz-Expires` or `rvs-expires` / `X-RVS-Expires-In` |
| DeleteObject | Supported | Deletes real object, then metadata |
| PutObject | Rejected | The gateway returns `405`; request a signed upload URL through the Object API before sending bytes |
| Multipart upload | Not implemented | Planned as sign-each-part control operations |
| CopyObject | Not implemented | Internal server-side copy is used for link invalidation |
| Versioning, tags, ACL, lifecycle, Select | Not implemented | These require explicit virtual semantics |
| Create/Delete real bucket | Intentionally unavailable | The one configured private backing bucket is infrastructure-owned |

## Central gateway invariant

Applications only need the gateway origin. All authentication, policy, metadata, signing, quota, onboarding and link management enter that origin. For S3 sources, a successful signing response hands object bytes to the S3 direct/CDN endpoint. WebDAV has no generic presign standard and uses a short-lived relay capability instead.
