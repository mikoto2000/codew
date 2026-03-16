SHELL := /bin/bash

APP := codew
OUT_DIR := build
BIN := $(OUT_DIR)/$(APP)
VERSION_STAMP = $(OUT_DIR)/.version-$(VERSION)
GOCACHE ?= $(PWD)/.gocache
GO_SOURCES := $(shell find . -type f -name '*.go' -not -path './.gocache/*' -not -path './build/*' -not -path './.git/*')
VERSION := 0.1.0
LDFLAGS := -X 'github.com/mikoto2000/codew/cmd.appVersion=$(VERSION)'

.PHONY: help all run chat doctor fmt vet test check clean

help:
	@echo "Targets:"
	@echo "  make build   - Build binary to $(BIN)"
	@echo "                 version=$(VERSION)"
	@echo "  make fmt     - Format Go source"
	@echo "  make vet     - Run go vet"
	@echo "  make test    - Run go test"
	@echo "  make check   - Run fmt + vet + test"
	@echo "  make clean   - Remove build artifacts"

all: build

build: $(BIN)

$(BIN): go.mod go.sum $(GO_SOURCES) $(VERSION_STAMP)
	mkdir -p $(OUT_DIR)
	GOCACHE=$(GOCACHE) go build -ldflags "$(LDFLAGS)" -o $(BIN) .

$(VERSION_STAMP):
	mkdir -p $(OUT_DIR)
	rm -f $(OUT_DIR)/.version-*
	touch $(VERSION_STAMP)

fmt:
	gofmt -w $$(find . -type f -name '*.go' -not -path './.gocache/*')

vet:
	GOCACHE=$(GOCACHE) go vet ./...

test:
	GOCACHE=$(GOCACHE) go test ./...

check: fmt vet test

clean:
	rm -rf $(OUT_DIR)
