.PHONY: build run clean install

BINARY  := skillctl
PKG     := ./cmd/skillctl

build:
	go build -o $(BINARY) $(PKG)

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)

install:
	go install $(PKG)
