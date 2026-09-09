FROM node:22-alpine AS web
ARG VIRSREE_VERSION=dev
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web/ ./
RUN VIRSREE_VERSION="$VIRSREE_VERSION" pnpm build

FROM golang:1.24-alpine AS go
ARG VIRSREE_VERSION=dev
ARG VIRSREE_COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist /src/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X rosemary-virsree/internal/buildinfo.Version=$VIRSREE_VERSION -X rosemary-virsree/internal/buildinfo.Commit=$VIRSREE_COMMIT" -o /out/rosemary-virsree ./cmd/server
RUN mkdir -p /out/downloads/rvsctl && \
    for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/arm64 windows/amd64; do \
      os=${target%/*}; arch=${target#*/}; dir=/out/downloads/rvsctl/$os/$arch; name=rvsctl; \
      if [ "$os" = windows ]; then name=rvsctl.exe; fi; \
      mkdir -p "$dir"; CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$dir/$name" ./cmd/rvsctl; \
    done && cd /out/downloads/rvsctl && find . -type f -name 'rvsctl*' -print | sort | xargs sha256sum > SHA256SUMS

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=go /out/rosemary-virsree /app/rosemary-virsree
COPY --from=web /src/web/dist /app/web/dist
COPY --from=go /out/downloads /app/downloads
COPY docs /app/docs
EXPOSE 8080
ENTRYPOINT ["/app/rosemary-virsree"]
