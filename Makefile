# taskwarrior-web-portal — Makefile with real file deps for incremental builds.
# Phony targets are workflow shortcuts; file targets do the actual work and
# only re-run when their inputs change.

GO_FILES       := $(shell find . -name '*.go' -not -name '*_templ.go' -not -path './bin/*' -not -path './tmp/*')
TEMPL_FILES    := $(shell find internal/views -name '*.templ' 2>/dev/null)
TEMPL_GO_FILES := $(TEMPL_FILES:.templ=_templ.go)

TAILWIND       := ./scripts/tailwindcss
TAILWIND_IN    := web/tailwind.input.css
TAILWIND_OUT   := web/static/app.css
TAILWIND_URL   := https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64

BIN            := bin/taskwarrior-web-portal

# templ regenerates ALL *_templ.go files in one pass; depending the group on
# the source group is good enough.
$(TEMPL_GO_FILES) &: $(TEMPL_FILES)
	templ generate

# Download the standalone Tailwind binary if missing. Gitignored; architecture
# is hard-coded to linux-x64 matching the dev environment. CI fetches its own
# copy via the build workflow so this target is only needed locally.
$(TAILWIND):
	@echo "Downloading tailwindcss binary..."
	curl -fsSL -o $(TAILWIND) $(TAILWIND_URL)
	chmod +x $(TAILWIND)

$(TAILWIND_OUT): $(TAILWIND) $(TAILWIND_IN) $(TEMPL_FILES) $(TEMPL_GO_FILES)
	@mkdir -p $(@D)
	$(TAILWIND) -i $(TAILWIND_IN) -o $(TAILWIND_OUT) --minify

STATIC_FILES   := $(shell find web/static -type f -not -name '*.css' 2>/dev/null) $(TAILWIND_OUT)

$(BIN): $(GO_FILES) $(TEMPL_GO_FILES) $(STATIC_FILES)
	@mkdir -p $(@D)
	go build -ldflags="-s -w" -o $(BIN) .

.PHONY: generate css css-setup build run test fmt clean check dev install uninstall

css-setup: $(TAILWIND)

generate: $(TEMPL_GO_FILES)
css: $(TAILWIND_OUT)
build: $(BIN)

run: build
	./$(BIN)

test:
	go test ./...

fmt:
	go fmt ./...
	templ fmt internal/views

clean:
	rm -rf bin tmp $(TAILWIND_OUT)
	find internal/views -name '*_templ.go' -delete

# check ensures generators produce valid, non-empty output. Useful before commit
# or in CI to catch templ/tailwind drift.
check: generate css
	@test -s $(TAILWIND_OUT) || (echo "tailwind output is empty"; exit 1)
	@for f in $(TEMPL_GO_FILES); do test -s $$f || (echo "templ output empty: $$f"; exit 1); done
	@echo "check ok"

# dev runs templ in watch mode AND tailwindcss in watch mode AND `go run`.
# Keeping these as separate processes avoids serialising slow watchers behind
# fast rebuilds. `trap 'kill 0' EXIT` ensures Ctrl-C tears them all down.
dev:
	@trap 'kill 0' EXIT; \
	templ generate --watch & \
	$(TAILWIND) -i $(TAILWIND_IN) -o $(TAILWIND_OUT) --watch & \
	go run . & \
	wait

# install/uninstall delegate to scripts so they're testable independently.
# install.sh branches on `uname -s`: macOS gets a launchd LaunchAgent,
# Linux (systemd-based distros only) gets a systemd --user unit.
install: build
	./scripts/install.sh

uninstall:
	./scripts/uninstall.sh
