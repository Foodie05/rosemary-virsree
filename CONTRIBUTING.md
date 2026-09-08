# Contributing

Run the backend tests and frontend production build before opening a pull request:

```bash
go test ./...
cd web && pnpm install --frozen-lockfile && pnpm build
```

Keep examples synthetic. Do not commit credentials, bootstrap tokens, presigned URLs, SQLite databases, generated binaries, frontend build output, or local environment files.

Changes to object transfer behavior must preserve the core invariant: control requests may enter Rosemary, while upload and download bodies go directly between the application and the real private S3 endpoint. Standard `PutObject` to `/s3` remains disabled.
