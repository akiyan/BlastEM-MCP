.PHONY: build fixture test check clean

TMPDIR ?= $(CURDIR)/.tmp

build:
	mkdir -p $(TMPDIR)
	TMPDIR=$(TMPDIR) go build -o blastem-mcp ./cmd/blastem-mcp

fixture:
	$(MAKE) -C testdata/fixture check

test:
	mkdir -p $(TMPDIR)
	TMPDIR=$(TMPDIR) go test ./...

check:
	mkdir -p $(TMPDIR)
	test -z "$$(gofmt -l cmd internal)"
	TMPDIR=$(TMPDIR) go vet ./...
	TMPDIR=$(TMPDIR) go test -race ./...

clean:
	$(RM) blastem-mcp
	$(MAKE) -C testdata/fixture clean
