BINARY   := gomacs
MODULE   := github.com/skybert/gomacs
BUILDDIR := build
GOFLAGS  :=

.PHONY: all build test lint vulncheck fmt clean install

all: fmt lint test vulncheck build

build: fmt
	mkdir -p $(BUILDDIR)
	go build $(GOFLAGS) -o $(BUILDDIR)/$(BINARY) .

test:
	go test ./...

lint:
	golangci-lint run ./...

vulncheck:
	govulncheck ./...

fmt:
	gofmt -w -s .

clean:
	rm -rf $(BUILDDIR)

install: build
	mkdir -p ~/.local/bin
	cp $(BUILDDIR)/$(BINARY) ~/.local/bin/$(BINARY)
