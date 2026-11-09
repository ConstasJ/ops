BINARY_NAME=ops
BIN_DIR=bin

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "1.0.3")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE ?= $(shell date -u +%Y-%m-%d)
LDFLAGS = -s -w -X 'ops-go/cmd.Version=$(VERSION)' -X 'ops-go/cmd.GitCommit=$(COMMIT)' -X 'ops-go/cmd.BuildDate=$(DATE)'

.PHONY: all build install test clean build-all build-linux-amd64 build-linux-arm64 build-windows-amd64 build-darwin-arm64

all: test build

build:
	go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) .

install:
	go build -ldflags="$(LDFLAGS)" -o /usr/local/bin/$(BINARY_NAME) .

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 .

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 .

build-windows-amd64:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe .

build-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64 .

build-all: build-linux-amd64 build-linux-arm64 build-windows-amd64 build-darwin-arm64

test:
	go test -v -cover ./...

clean:
	rm -rf $(BIN_DIR)
