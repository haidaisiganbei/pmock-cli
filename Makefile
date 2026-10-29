BINARY  := pmock-cli
MODULE  := github.com/pmock/pmock-cli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(MODULE)/internal/build.Version=$(VERSION) \
	-X $(MODULE)/internal/build.Commit=$(COMMIT) \
	-X $(MODULE)/internal/build.Date=$(DATE)
PREFIX  ?= /usr/local

.PHONY: build test vet fmt check install clean snapshot run update-spec package

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

check: fmt vet test

install: build
	install -d $(PREFIX)/bin
	install -m755 bin/$(BINARY) $(PREFIX)/bin/$(BINARY)

snapshot:
	goreleaser release --snapshot --clean

run: build
	./bin/$(BINARY) --help

update-spec:
	./scripts/update-spec.sh

package:
	./scripts/package.sh

clean:
	rm -rf bin dist
