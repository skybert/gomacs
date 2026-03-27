
BINARY   := gomacs
MODULE   := github.com/skybert/gomacs
BUILDDIR := build
DISTDIR  := dist
GOFLAGS  :=
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
AUTHORS  := $(shell awk 'NR>1{printf ", "} {gsub(/[<>]/, "\\\\&"); printf "%s", $$0} END{print ""}' AUTHORS 2>/dev/null)
DATE     := $(shell date +%Y-%m-%d)

.PHONY: all build test lint vulncheck fmt clean install man doc dist

all: fmt lint test vulncheck build man

build: fmt
	mkdir -p $(BUILDDIR)
	go build $(GOFLAGS) -ldflags "-X main.Version=$(VERSION)" -o $(BUILDDIR)/$(BINARY) .

# Used by CI/CD to build release binaries. GOOS and GOARCH env vars
# are set in the CI/CD conf.
dist:
	@mkdir -p $(DISTDIR)
	CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION)" -o dist/$(BINARY)-$(VERSION)-${GOOS}-${GOARCH} .

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
	rm -rf $(BUILDDIR) $(DISTDIR)

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

doc: doc/gomacs.md

doc/gomacs.md: doc/gomacs.1.in AUTHORS doc/*.png
	go run ./cmd/man2md
