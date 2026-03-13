BINARY   := gomacs
MODULE   := github.com/skybert/gomacs
BUILDDIR := build
GOFLAGS  :=

.PHONY: all build test lint vulncheck fmt clean

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
