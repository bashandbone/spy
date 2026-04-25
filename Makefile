# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0

.PHONY: build clean run test help

help:
	@echo "spy - CLI file viewer"
	@echo ""
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  clean      - Remove build artifacts"
	@echo "  run        - Run the application"
	@echo "  test       - Run tests"
	@echo "  install    - Install the binary to GOPATH/bin"
	@echo "  dev        - Build and run with current directory"
	@echo "  fmt        - Format code"
	@echo "  vet        - Run go vet"
	@echo "  help       - Show this help message"

build:
	@mkdir -p bin
	go build -o bin/spy ./cmd/spy

clean:
	@rm -rf bin/
	go clean

run: build
	./bin/spy

test:
	go test -v ./...

install: build
	cp bin/spy $(GOPATH)/bin/spy

dev: build
	./bin/spy .

fmt:
	go fmt ./...

vet:
	go vet ./...

all: fmt vet build test
