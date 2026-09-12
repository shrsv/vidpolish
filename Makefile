.PHONY: build ui dev web-install web-build web-dev test vet fmt deps clean

# Build the vidpolish binary (embeds internal/server/webdist as-is;
# run `make web-build` first if you've changed the frontend).
build:
	go build -o vidpolish ./cmd/vidpolish

# Build and launch the local Projects UI at http://127.0.0.1:7890.
ui: build
	./vidpolish ui

# Frontend + backend dev loop: runs the Go server in the background
# (auto-restarts are not included; re-run `make dev` after Go changes)
# and the Vite dev server in the foreground with hot reload, proxying
# /api to the backend. Ctrl-C stops both.
dev: build
	./vidpolish ui --port 7890 --no-open & \
	backend_pid=$$!; \
	trap "kill $$backend_pid 2>/dev/null" EXIT INT TERM; \
	cd web && npm run dev

# Install frontend dependencies (only needed once, or after
# web/package.json changes).
web-install:
	cd web && npm install

# Build the frontend into internal/server/webdist for `make build` to
# embed. Run this after any change under web/src before rebuilding the
# Go binary.
web-build:
	cd web && npm run build

# Vite dev server alone (hot reload), proxying /api to a vidpolish ui
# server you run separately (e.g. `make ui` in another terminal).
web-dev:
	cd web && npm run dev

# Run all Go tests.
test:
	go test ./...

# Vet all Go packages.
vet:
	go vet ./...

# Format all Go source and fail if anything needed reformatting.
fmt:
	gofmt -l .

# Resolve/download all external tool dependencies (ffmpeg check,
# deep-filter, auto-editor, resvg, font).
deps: build
	./vidpolish deps

# Remove build artifacts. Does not touch web/node_modules or any
# ~/.vidpolish state.
clean:
	rm -f vidpolish
