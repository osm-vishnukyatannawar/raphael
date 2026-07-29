GOLANGCI_VERSION := v2.12.2
VERSION          ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS          := -X main.version=$(VERSION)

# Wails defaults to webkit2gtk-4.0, which most current distros no longer ship.
# The webkit2_41 tag targets webkit2gtk-4.1 instead. Linux only — on Windows the
# tag is meaningless and WebView2 is used.
UNAME_S := $(shell uname -s 2>/dev/null)
ifeq ($(UNAME_S),Linux)
  TAGS := -tags webkit2_41
endif

.DEFAULT_GOAL := help
.PHONY: help setup tools dev build lint lint-go lint-frontend fmt test typecheck generate icon clean

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

setup: tools ## Install tooling and git hooks (run once per clone)
	cd frontend && pnpm install --frozen-lockfile
	go tool lefthook install

tools: ## Install pinned dev tools not managed by go.mod
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

dev: ## Run the app with hot reload
	wails dev $(TAGS)

build: ## Build a production binary into build/bin
	wails build $(TAGS) -ldflags "$(LDFLAGS)"

lint: lint-go lint-frontend ## Lint everything

lint-go: ## Lint Go
	golangci-lint run ./...

lint-frontend: ## Lint the frontend
	cd frontend && pnpm exec eslint . && pnpm exec prettier --check .

fmt: ## Format Go and frontend sources in place
	golangci-lint fmt ./...
	cd frontend && pnpm exec prettier --write . && pnpm exec eslint . --fix

test: ## Run Go tests (with -race, matching CI)
	go test -race ./...

typecheck: ## Typecheck the frontend
	cd frontend && pnpm exec tsc -b

generate: ## Regenerate sqlc queries and Wails TypeScript bindings
	go tool sqlc generate
	wails generate module

icon: ## Rasterize build/appicon.svg into the PNG and Windows .ico
	rsvg-convert -w 1024 -h 1024 build/appicon.svg -o build/appicon.png
	magick build/appicon.png -define icon:auto-resize=256,128,64,48,32,16 build/windows/icon.ico

clean: ## Remove build output
	rm -rf build/bin frontend/dist/* frontend/node_modules/.tmp
	touch frontend/dist/.gitkeep
