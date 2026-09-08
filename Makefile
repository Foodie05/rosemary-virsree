.PHONY: dev build test release-cli
dev:
	go run ./cmd/server
build:
	cd web && pnpm build
	go build -trimpath -o bin/rosemary-virsree ./cmd/server
	go build -trimpath -o bin/rvsctl ./cmd/rvsctl
test:
	go test ./...
	cd web && pnpm build
release-cli:
	./scripts/build-rvsctl.sh
