# ADR 0002: implement the MCP server in Go

- Status: accepted
- Date: 2026-09-01

## Context

The MCP server must manage a long-lived BlastEM child process, coordinate a
Unix-domain control socket and a localhost GDB Remote connection, enforce
timeouts/cancellation, and ship as an easy-to-run local tool on Linux.

## Decision

Implement the server in Go, initially targeting Go 1.27 and the official MCP Go
SDK package `github.com/modelcontextprotocol/go-sdk/mcp`. Serve MCP over stdio
with `mcp.StdioTransport` and represent tool inputs/outputs as typed Go structs.

Use standard-library networking, process, context, encoding, and testing APIs
where practical. Keep emulator adapters in separate internal packages so their
protocol logic can be tested without starting an MCP server.

## Consequences

Positive:

- The project can release one executable without a Node runtime dependency.
- Goroutines, contexts, `net.UnixConn`, and `net.TCPConn` fit the two concurrent
  emulator protocols and cancellation requirements.
- The race detector can validate session and connection state handling.
- Protocol structs and binary parsing remain explicit and statically typed.

Negative:

- MCP SDK APIs and supported protocol versions must be pinned and covered by
  compatibility tests.
- Image/resource result construction needs focused tests because less compile-
  time schema tooling is available than in some dynamic SDK ecosystems.
- Cross-compilation does not remove the need to build BlastEM and SDL
  dependencies separately.

## Revisit criteria

Reconsider only if the official Go SDK cannot interoperate with the supported
MCP hosts or blocks a required tool/resource capability without a reasonable
adapter. Language preference alone is not a reason to change the architecture.
