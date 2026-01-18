.PHONY: build test lint

build:
	go build ./...

test:
	go test ./...

lint:
	@echo "no lint configured yet"
