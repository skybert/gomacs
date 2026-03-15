
BINARY   := gomacs
MODULE   := github.com/skybert/gomacs
BUILDDIR := build
GOFLAGS  :=
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
AUTHORS  := $(shell awk 'NR>1{printf ", "} {gsub(/[<>]/, "\\\\&"); printf "%s", $$0} END{print ""}' AUTHORS 2>/dev/null)
DATE     := $(shell date +%Y-%m-%d)

.PHONY: all build test lint vulncheck fmt clean install man

all: fmt lint test vulncheck build man

build: fmt
	mkdir -p $(BUILDDIR)
	go build $(GOFLAGS) -o $(BUILDDIR)/$(BINARY) .

run: build
	./$(BUILDDIR)/$(BINARY)

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

man: $(BUILDDIR)/gomacs.1

$(BUILDDIR)/gomacs.1: doc/gomacs.1.in AUTHORS
	mkdir -p $(BUILDDIR)
	sed -e 's/@VERSION@/$(VERSION)/g' \
	    -e 's/@AUTHORS@/$(AUTHORS)/g' \
	    -e 's/@DATE@/$(DATE)/g' \
	    $< > $@
