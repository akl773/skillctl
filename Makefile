.PHONY: build run clean install test cover

BINARY  := skillctl
PKG     := ./cmd/skillctl
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X akhilsingh.in/skillctl/internal/config.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)

install:
	go install -ldflags "$(LDFLAGS)" $(PKG)

test:
	go test ./... -v -count=1

cover:
	go test ./... -coverprofile=coverage.out -count=1
	go tool cover -func=coverage.out
