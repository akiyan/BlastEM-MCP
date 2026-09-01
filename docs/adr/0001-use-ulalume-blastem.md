# ADR 0001: use `ulalume/blastem` as the initial emulator backend

- Status: accepted
- Date: 2026-09-01
- Starting revision: `4b71bfec3b5919367c5843f23c22c76ac24aa38c`

## Context

The project needs direct controller input, screenshots, hardware-state access,
and CPU debugging from an AI tool without desktop automation. Upstream BlastEM
already has strong native and GDB debugging, but lacks a general persistent
external control API.

The `ulalume/blastem` fork adds a Unix-domain control socket, localhost GDB
Remote support on Unix, screenshot and VDP snapshot commands, and scripting/
headless-oriented changes. This covers most of the first MCP milestone while
keeping emulator changes small.

## Decision

Pin `ulalume/blastem` as a Git submodule and treat its current protocols as two
separate adapters:

- Unix control socket for input, screenshots, and VDP snapshots.
- GDB Remote Serial Protocol for CPU registers, memory, breakpoints, step, and
  continue.

The MCP server will be a separate Go process. It will not be linked into
BlastEM and will not place JSON-RPC/MCP logic in the emulator.

## Consequences

Positive:

- The first end-to-end version can be built mostly as an adapter.
- Existing GDB semantics and tools remain usable independently of MCP.
- MCP and emulator release cycles stay separate.
- The fork's changes are small enough to review and potentially upstream.

Negative:

- Two backend protocols must be coordinated as one session.
- The current control socket has no response/acknowledgement framing.
- Deterministic frame advance and socket-driven save/load are still missing.
- The project depends on a small third-party fork, so the SHA and compatibility
  behavior must be pinned and tested.

## Revisit criteria

Reconsider this backend if the fork stops building on supported Linux systems,
cannot provide deterministic stepping without invasive changes, diverges too
far from upstream BlastEM, or another backend satisfies M1 and M2 with less
maintenance cost.
