# Architecture decisions

## Multiple private storage sources

Virtual buckets are independent of physical storage sources. Each object records the source that owns its opaque physical key. New uploads try enabled sources by ascending priority and fall through when a source lacks reserved capacity. S3 and WebDAV are supported; adding either performs a write, read, copy and delete probe before encrypted configuration is committed.

## Split control and data paths

The Go gateway handles identity, permission checks, quotas, object mapping, audit records and signatures. S3 file bodies use presigned direct URLs and do not cross the gateway. Generic WebDAV has no presigned-URL standard, so its short-lived upload/download capability URLs relay bytes through VirSree.

Standard `PutObject` is disabled on the virtual `/s3` endpoint. Applications request an upload signature with the control API and PUT bytes only to the returned real S3 staging URL. The signature binds the declared content length. On commit, VirSree verifies the staging object, copies it to a fresh final physical key, switches the logical mapping, and deletes staging. Reusing an unexpired upload URL therefore cannot overwrite the committed object or upload more bytes than reserved.

## Application-selected signature duration

Signing calls reject missing or non-positive expiry values. The platform accepts the requested duration up to the provider's protocol limit. Stable public aliases save the duration explicitly selected when each alias is created and use it for later redirects.

## Immediate invalidation

A real presigned URL cannot be recalled because authorization is embedded in its query string. For immediate object-level invalidation, VirSree copies the current object to a new opaque physical key inside S3, deletes the old physical key, and updates its logical mapping. Stable aliases resolve through the mapping and continue unless revoked through their own API; previously returned direct URLs fail.

## Capacity accounting

Each virtual bucket has a hard byte quota. The sum of configured bucket quotas cannot exceed the platform quota. Upload signing creates a reservation; successful commit moves the delta into used bytes. Replacing an object only reserves growth over the current object size, and expired unsigned or abandoned upload reservations are reclaimed on the next reservation attempt. Per-bucket object and pending-upload count limits also bound metadata growth from zero-byte objects.
