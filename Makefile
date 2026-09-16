.PHONY: build ui dev web-install web-build web-dev test vet fmt deps clean \
	version bump-patch bump-minor bump-major tag-release release-build release-publish \
	gui-toolchain-check gui-build gui-build-windows

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
	rm -rf cmd/vidpolish-gui/build/bin

# --- Native GUI (Wails) -------------------------------------------------

# Verify the GUI/installer toolchain (wails CLI, mingw-w64, nsis) is
# installed and matches versions.env; see scripts/install-gui-toolchain.sh.
gui-toolchain-check:
	@./scripts/install-gui-toolchain.sh

# Build the native GUI app for the host OS (dev-machine smoke test only;
# needs the host's own GUI toolkit dev headers, e.g. libgtk-3-dev and
# libwebkit2gtk-4.0-dev on Linux - see `wails doctor`). Not used for the
# Windows installer; see gui-build-windows for that.
gui-build: web-build
	cd cmd/vidpolish-gui && wails build

# Cross-compile the Windows GUI + CLI and package them into a single NSIS
# installer, dropped at ~/Downloads/vidpolish-setup-<version>.exe for
# testing on a real/VM Windows machine. Requires
# `make gui-toolchain-check` to pass first.
gui-build-windows:
	@./scripts/build-gui-windows.sh "$$(cat VERSION)"

# --- Release process ---------------------------------------------------
#
# Typical flow:
#   make bump-patch          # or bump-minor / bump-major
#   git diff VERSION         # review
#   make tag-release         # commits VERSION + creates annotated tag
#   git push && git push --tags
#   make release-publish     # cross-compiles + publishes a GitHub Release

# Print the current version (from the VERSION file).
version:
	@cat VERSION

# Bump the patch/minor/major component of VERSION in place (semver, no
# commit/tag — review the change before running `make tag-release`).
bump-patch:
	@awk -F. '{printf "%d.%d.%d\n", $$1, $$2, $$3+1}' VERSION > VERSION.tmp
	@mv VERSION.tmp VERSION
	@cat VERSION

bump-minor:
	@awk -F. '{printf "%d.%d.%d\n", $$1, $$2+1, 0}' VERSION > VERSION.tmp
	@mv VERSION.tmp VERSION
	@cat VERSION

bump-major:
	@awk -F. '{printf "%d.%d.%d\n", $$1+1, 0, 0}' VERSION > VERSION.tmp
	@mv VERSION.tmp VERSION
	@cat VERSION

# Commit VERSION and create an annotated tag vX.Y.Z for the current
# contents of VERSION. Refuses to run on a dirty tree or if the tag
# already exists. Does not push.
tag-release:
	@V=$$(cat VERSION); \
	if [ -n "$$(git status --porcelain -- . ':!VERSION')" ]; then \
		echo "error: working tree has uncommitted changes outside VERSION; commit or stash first" >&2; \
		exit 1; \
	fi; \
	if git rev-parse "v$$V" >/dev/null 2>&1; then \
		echo "error: tag v$$V already exists" >&2; \
		exit 1; \
	fi; \
	git add VERSION && \
	if ! git diff --cached --quiet -- VERSION; then \
		git commit -m "Release v$$V" || exit 1; \
	fi && \
	git tag -a "v$$V" -m "v$$V" && \
	echo "tagged v$$V (run: git push && git push --tags)"

# Cross-compile release binaries for all supported platforms into
# dist/v<VERSION>/ (see scripts/release-build.sh).
release-build:
	@./scripts/release-build.sh "$$(cat VERSION)"

# Build (if needed) and publish a GitHub Release with the cross-compiled
# binaries and checksums.txt attached. Requires `gh` to be authenticated.
# Pass DRAFT=1 to publish as a draft (not publicly visible until you
# publish it from the GitHub UI or `gh release edit --draft=false`).
release-publish: release-build
	@V=$$(cat VERSION); \
	DRAFT_FLAG=""; \
	if [ "$(DRAFT)" = "1" ]; then DRAFT_FLAG="--draft"; fi; \
	gh release create "v$$V" dist/v$$V/vidpolish-* dist/v$$V/checksums.txt \
		--title "v$$V" --generate-notes $$DRAFT_FLAG
