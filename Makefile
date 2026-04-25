# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0

.PHONY: help build build-fitz clean run test test-race \
        cover-default cover-fitz cover \
        lint vet fmt reuse install dev perf all

help:
	@echo "spy - CLI file viewer"
	@echo ""
	@echo "Available targets:"
	@echo "  build         - Build the default (pure-Go) binary"
	@echo "  build-fitz    - Build with -tags fitz (cgo PDF rasterization)"
	@echo "  test          - Run all tests"
	@echo "  test-race     - Run all tests with -race"
	@echo "  cover-default - Per-package coverage (default build)"
	@echo "  cover-fitz    - Per-package coverage (-tags fitz)"
	@echo "  cover         - Merge default+fitz coverage into coverage.out"
	@echo "  lint          - gofmt + goimports check (no rewrites)"
	@echo "  vet           - go vet (default and -tags fitz)"
	@echo "  fmt           - go fmt ./..."
	@echo "  reuse         - reuse lint (SPDX/license check)"
	@echo "  perf          - Nightly perf suite (-tags perf)"
	@echo "  clean         - Remove build artifacts"
	@echo "  install       - Install the binary to GOPATH/bin"
	@echo "  dev           - Build and run with current directory"
	@echo "  all           - fmt + vet + lint + test-race + cover + build"

build:
	@mkdir -p bin
	go build -o bin/spy ./cmd/spy

build-fitz:
	@mkdir -p bin
	go build -tags fitz -o bin/spy-fitz ./cmd/spy

test:
	go test ./...

test-race:
	go test ./... -race

cover-default:
	go test ./... -race -coverprofile=cov-default.out

cover-fitz:
	go test -tags fitz ./... -race -coverprofile=cov-fitz.out

# `cover` produces a single merged profile so files gated by build tags
# (notably internal/graphics/pdf_*.go) are visible to the ≥ 80%/package
# threshold check. Requires gocovmerge: see DEVELOPMENT.md for installation.
cover: cover-default cover-fitz
	gocovmerge cov-default.out cov-fitz.out > coverage.out

lint:
	@echo "==> gofmt -l ."
	@gofmt_out=$$(gofmt -l . 2>&1); \
	  if [ -n "$$gofmt_out" ]; then echo "$$gofmt_out"; exit 1; fi
	@echo "==> goimports -l ."
	@goimports_out=$$(goimports -l . 2>&1); \
	  if [ -n "$$goimports_out" ]; then echo "$$goimports_out"; exit 1; fi

vet:
	go vet ./...
	go vet -tags fitz ./...

fmt:
	go fmt ./...

reuse:
	reuse lint

perf:
	go test -tags perf ./tests/perf/...

clean:
	@rm -rf bin/ cov-default.out cov-fitz.out coverage.out
	go clean

run: build
	./bin/spy

install: build
	cp bin/spy $(GOPATH)/bin/spy

dev: build
	./bin/spy .

all: fmt vet lint test-race cover build
