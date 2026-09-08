# Gateway API

The examples use one gateway origin, such as `https://storage.example.com`. JSON errors have the shape `{"error":"..."}`.

## Authentication

Administrative routes require `Authorization: Bearer <RVS_ADMIN_TOKEN>`.

Virtual application routes require:

```http
X-RVS-Access-Key: RVS...
X-RVS-Secret-Key: rvs_...
```

The `/s3` routes use AWS Signature Version 4 with the virtual AK/SK. Permissions are `read`, `write`, `delete`, and `manage`. A key may contain any combination. Permissions are independent; `manage` does not imply read, write, or delete.

## Administrative API

| Method | Route | Purpose |
|---|---|---|
| `GET` | `/api/v1/overview` | Capacity, bucket, object, key and backend status |
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

Every S3 signature request requires an application-selected positive expiry. AWS-compatible backends using SigV4 cap it at 604800 seconds. Rosemary does not shorten a valid requested duration.

The public-link response contains `slug`, `public_url`, and `direct_url`. `public_url` is a Rosemary alias that can keep working until `link_expires_in`; each visit redirects to a new real URL valid for `sign_expires_in`. Revoke it with the `slug`. Already issued `direct_url` values bypass Rosemary and remain usable until their chosen expiry unless the object physical key is rotated.

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

Applications only need the gateway origin. All authentication, policy, metadata, signing, quota, onboarding and link management enter that origin. A successful signing response is the handoff point: subsequent object bytes use the returned `RVS_S3_PUBLIC_ENDPOINT` URL directly.
