.PHONY: build test test-race lint cover clean

BINARY := swarm-sentinel
BIN_DIR := bin

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) ./cmd/swarm-sentinel

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run ./...

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out
	@echo "\nTo view HTML report: go tool cover -html=coverage.out"

clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html
