# Makefile sample
GO       ?= go
BIN_DIR  ?= bin
BIN      := $(BIN_DIR)/sample
GOFLAGS  ?= -trimpath
LDFLAGS  ?= -s -w
PACKAGES := $(shell $(GO) list ./...)

.PHONY: all build test lint clean

all: build

build: $(BIN)

$(BIN): $(shell find . -type f -name '*.go')
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $@ ./cmd/sample

test:
	$(GO) test -race -count=1 $(PACKAGES)

lint:
	$(GO) vet $(PACKAGES)
	gofmt -l . | tee /dev/stderr | (! grep .)

clean:
	rm -rf $(BIN_DIR)
