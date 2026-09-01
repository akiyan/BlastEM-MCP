.PHONY: build fixture integration-fixture test check clean

TMPDIR ?= $(CURDIR)/.tmp

build:
	mkdir -p $(TMPDIR)
	TMPDIR=$(TMPDIR) go build -o blastem-mcp ./cmd/blastem-mcp

fixture:
	$(MAKE) -C testdata/fixture check

integration-fixture: fixture
	test -x "$(CURDIR)/third_party/blastem/blastem"
	BLASTEM_INTEGRATION_BINARY="$(CURDIR)/third_party/blastem/blastem" \
	BLASTEM_INTEGRATION_ROM="$(CURDIR)/testdata/fixture/build/blastem-mcp-fixture.bin" \
	BLASTEM_FIXTURE_ROM="$(CURDIR)/testdata/fixture/build/blastem-mcp-fixture.bin" \
	BLASTEM_FIXTURE_SYMBOLS="$(CURDIR)/testdata/fixture/build/symbols.txt" \
	TMPDIR="$(TMPDIR)" \
	go test -count=1 -run 'Test(MCPBlastEMIntegration|FixtureMCPContract)$$' -v ./internal/mcpserver
	BLASTEM_INTEGRATION_BINARY="$(CURDIR)/third_party/blastem/blastem" \
	BLASTEM_INTEGRATION_ROM="$(CURDIR)/testdata/fixture/build/blastem-mcp-fixture.bin" \
	TMPDIR="$(TMPDIR)" \
	go test -count=1 -run 'TestBlastEM(Control|GDB)Integration$$' -v ./internal/session

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
