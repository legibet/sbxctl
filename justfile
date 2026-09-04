default: build

build:
    go build -o sbxctl ./cmd/sbxctl

test:
    go test ./...

fmt:
    golangci-lint fmt

lint:
    golangci-lint run

check: fmt lint test
