GO      ?= $(shell command -v go 2>/dev/null || echo /opt/homebrew/opt/go/bin/go)
BINARY  := loar
CMD     := ./cmd/loar

# Install destination — mirrors what Homebrew does on Apple Silicon.
# Override with: make install PREFIX=/usr/local
BREW_PREFIX := $(shell brew --prefix 2>/dev/null || echo /usr/local)
PREFIX      ?= $(BREW_PREFIX)
BINDIR      := $(PREFIX)/bin

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

.PHONY: all build install uninstall reinstall test clean fmt vet db-up db-down

## build: Compile the loar binary into ./loar
build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)
	@echo "Built ./$(BINARY)  ($(VERSION))"

## install: Build and install loar to $(BINDIR) (mirrors Homebrew)
install: build
	@echo "Installing $(BINARY) → $(BINDIR)/$(BINARY)"
	install -d $(BINDIR)
	install -m 755 $(BINARY) $(BINDIR)/$(BINARY)
	@echo ""
	@echo "$(BINARY) installed. Verify with: loar --help"
	@echo ""
	@echo "Next step: loar setup"

## uninstall: Remove loar from $(BINDIR)
uninstall:
	@if [ -f $(BINDIR)/$(BINARY) ]; then \
		rm -f $(BINDIR)/$(BINARY); \
		echo "Removed $(BINDIR)/$(BINARY)"; \
	else \
		echo "$(BINDIR)/$(BINARY) not found — nothing to remove"; \
	fi

## test: Run all tests
test:
	$(GO) test ./...

## fmt: Format all Go source
fmt:
	$(GO) fmt ./...

## vet: Run go vet
vet:
	$(GO) vet ./...

## clean: Remove build artefacts
clean:
	rm -f $(BINARY)

## reinstall: Clean, rebuild, and reinstall (full clean rebuild)
reinstall: uninstall clean install

## db-up: Start the local Postgres container
db-up:
	docker compose up -d
	@echo "Postgres available at localhost:5432 (user: postgres / password: postgres)"
	@echo "Run: loar setup"

## db-down: Stop and remove the local Postgres container (data is preserved in the Docker volume)
db-down:
	docker compose down

## help: Show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'
