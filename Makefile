BINARY_NAME=ops
BIN_DIR=bin

.PHONY: all build test clean build-linux-amd64 build-linux-arm64

all: test build

build:
	go build -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME) .

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 .

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 .

test:
	go test -v -race -cover ./...

clean:
	rm -rf $(BIN_DIR)
