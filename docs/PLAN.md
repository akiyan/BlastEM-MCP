# BlastEM MCP implementation plan

Status: M1 v0.1 MVP complete (2026-09-01)

## 1. Outcome

Build a local Linux MCP server that lets a coding agent operate and debug a
Mega Drive/Genesis program in BlastEM without keyboard emulation, window focus,
or desktop screenshots.

The first useful release is complete when an MCP client can launch a test ROM,
drive controller input, retrieve an emulator-produced screenshot, stop on a
68000 breakpoint, inspect/change registers and RAM, single-step, resume, and
shut down cleanly.

## 2. Scope

### v0.1: existing-fork MVP

Use the selected BlastEM fork without changing its wire protocols.

| Area | MCP tools | Backend |
|---|---|---|
| Session | `blastem_start`, `blastem_status`, `blastem_stop` | managed child process |
| Input | `button_down`, `button_up`, `release_all_buttons` | Unix control socket |
| Video/VDP | `screenshot`, `vdp_snapshot`, `vram_tiles` | Unix control socket + artifact decoding |
| CPU | `cpu_registers`, `cpu_step`, `cpu_continue` | GDB Remote |
| Memory | `memory_read`, `memory_write` | GDB Remote |
| Breakpoints | `breakpoint_set`, `breakpoint_remove` | GDB Remote |

`screenshot` returns an MCP image plus metadata. `vdp_snapshot` returns parsed
metadata and an artifact/resource reference; raw binary data is not dumped into
model context by default. `vram_tiles` decodes the snapshot into four separate
PNGs containing all 2048 8x8 tiles, one for each live CRAM palette. Palette
index zero is shown as a checkerboard. The reference screenshot and VDP dump
are armed together so palette animations and fades cannot mix adjacent frames.

### v0.2: deterministic automation

Add a small, upstreamable BlastEM control-protocol extension:

- acknowledged command responses and structured errors;
- `pause`, `resume`, and `advance <frames>`;
- `press <pad> <button> <frames>` or an equivalent atomic input timeline;
- `reset`;
- named `save_state` and `load_state`;
- capability/version negotiation;
- an emulator-side break/interrupt command suitable for cancelling a running
  GDB continue operation.

Then expose `step_frames`, `press_frames`, `reset`, `save_state`, and
`load_state` as MCP tools.

### Later, only after usage evidence

- symbol/ELF-aware source locations and disassembly;
- Z80 debugging;
- decoded planes, sprites, palettes, and tile maps;
- multi-session execution;
- macOS/Windows support;
- Streamable HTTP transport.

### Explicit non-goals for v0.1

- embedding an MCP implementation into BlastEM;
- replacing BlastEM's native debugger or GDB Remote;
- autonomous gameplay or computer vision;
- shipping copyrighted commercial ROMs;
- public/network-accessible emulator control.

## 3. Architecture

```text
Codex / Claude / VS Code
          |
          | MCP over stdio
          v
     blastem-mcp (Go)
    |       |          |
    |       |          +-- session/artifact manager
    |       +------------- GDB Remote client (localhost TCP)
    +--------------------- BlastEM control client (Unix socket)
                               |
                               v
                     ulalume/blastem child process
```

The MCP server owns one BlastEM process and a private runtime directory per
session. Socket paths, screenshots, dumps, state files, and logs live there.
The directory is deleted only after the child exits; on abnormal termination it
is retained long enough to report diagnostics.

### Technology baseline

- Go 1.27 or newer, producing a single `blastem-mcp` executable.
- Official MCP Go SDK (`github.com/modelcontextprotocol/go-sdk/mcp`) using its
  local `StdioTransport`.
- Typed Go structs with JSON/JSON-Schema tags for tool arguments and structured
  results.
- Standard `testing` package, table-driven tests, and `go test -race` for the
  session and protocol packages.
- Direct, minimal GDB Remote Serial Protocol client rather than shelling out to
  interactive GDB. Restrict the implementation to packets required by the MVP.

Go packages should follow these boundaries:

```text
cmd/blastem-mcp/       process entry point; MCP stdio wiring only
internal/mcpserver/    tool registration, schemas, and error mapping
internal/session/      BlastEM child lifecycle and runtime directory
internal/control/      Unix control-socket client
internal/gdbrsp/       minimal GDB Remote Serial Protocol client
internal/artifact/     screenshot and KITVDMP1 validation
```

## 4. Backend contracts and constraints

### BlastEM process

Launch the pinned fork with:

- `BLASTEM_CTRL_SOCK=<session>/control.sock`;
- `BLASTEM_GDB_PORT=<dynamically allocated localhost port>`;
- `BLASTEM_NO_GUI=1` where applicable;
- `-D` to enter the GDB remote debugger;
- an explicitly supplied ROM path.

The manager must detect readiness, startup failure, unexpected exit, and GDB
stop reasons. stdout/stderr are logs, never MCP protocol output.

### Existing control socket

The current protocol is newline-delimited, single-client, and fire-and-forget.
MVP supports its existing `pad`, `screenshot`, and `vramdump` commands.
Screenshot and dump completion are detected by watching a unique destination
file with a deadline and validating its format. Input state is mirrored in the
MCP server so all held buttons can be released during errors and shutdown.

Because there is no acknowledgement or frame-control command, v0.1 must not
claim deterministic frame timing.

### GDB Remote

Implement packet checksum/acknowledgement, stop replies, and only these command
families:

- `g`/`p`/`P` for registers;
- `m`/`M` or `X` for memory;
- `Z0`/`z0` for software breakpoints;
- `s`, `c`, and stop-query handling.

The selected fork currently ignores the raw GDB Remote Ctrl-C byte while the
CPU is running. Therefore v0.1 `cpu_continue` must be used with a breakpoint;
arbitrary interrupt/cancellation requires the v0.2 emulator protocol extension.

All register names and byte order are covered by fixture tests. Unknown packets,
malformed replies, timeouts, and a terminated emulator become typed MCP errors.

## 5. Safety and API rules

- Bind GDB only to loopback; never accept a remote host in v0.1.
- Resolve and validate the BlastEM binary, ROM, output, and state paths.
- Default artifact writes to the session directory. Explicit output paths must
  be absolute and pass an allowlist policy.
- Cap memory reads/writes (initially 64 KiB per call) and screenshot/dump waits.
- Reject conflicting directional input (`left+right`, `up+down`) unless a caller
  explicitly opts into raw input in a later API.
- Serialize mutating/debug execution commands. Read-only calls may run only when
  the backend state makes them safe.
- Always release held inputs and terminate the managed child on MCP shutdown.
- Do not log ROM bytes, RAM contents, or MCP messages unless debug logging is
  explicitly enabled.

## 6. Delivery milestones

### M0 — project bootstrap

- Public repository, license, contribution guide, pinned fork, CI skeleton.
- Architecture and protocol decisions recorded.

Exit: a clean checkout identifies the exact BlastEM source revision and the
next work is represented by actionable GitHub issues.

### M1 — v0.1 control and debug MVP

Completed on 2026-09-01. The full acceptance scenario below runs in Linux CI
against a source-built redistributable fixture ROM and the pinned BlastEM fork.

1. Go MCP stdio server and typed tool schemas.
2. Session/runtime directory manager.
3. Control-socket client with input cleanup and artifact deadlines.
4. Minimal GDB Remote client.
5. Tool adapters and structured errors.
6. Unit tests plus a redistributable homebrew integration ROM.
7. Linux CI build/test and user setup documentation.

Exit acceptance scenario:

1. `blastem_start` launches the fixture ROM and returns a session description.
2. `button_down`/`button_up` changes an observable value in the fixture.
3. `screenshot` returns a valid image generated by BlastEM.
4. A breakpoint stops at a known fixture address.
5. `cpu_registers` and `memory_read` match known values.
6. `memory_write`, `cpu_step`, and `cpu_continue` work and report stop/running
   state correctly.
7. `vdp_snapshot` validates the `KITVDMP1` artifact.
8. `blastem_stop` exits and removes socket/input state without orphan processes.

### M2 — v0.2 deterministic control

- Specify and implement the acknowledged control protocol.
- Add deterministic frame advance, frame-counted input, reset, and save/load.
- Add emulator-side C tests where practical and MCP integration tests.
- Prepare the BlastEM changes as a focused upstream contribution or maintained
  patch series.

Exit: the same input/state sequence produces the same checkpoint hashes in
repeated test runs, with documented exceptions for nondeterministic peripherals.

### M3 — v1.0 hardening

- API stability review, version/capability reporting, complete setup docs.
- Failure recovery and compatibility matrix for supported fork revisions.
- Release artifacts and end-to-end validation in at least two MCP hosts.

## 7. Testing strategy

- Unit tests: parsers, checksums, endian conversion, schemas, path policy.
- Fake-server tests: fragmented socket reads, bad checksums, disconnects,
  timeouts, delayed artifacts, and unexpected process exit.
- Integration tests: compiled open homebrew fixture with known symbols,
  framebuffer output, RAM markers, and breakpoint locations.
- Smoke tests: build the pinned BlastEM revision and run the acceptance scenario
  under a virtual display if rendering requires it.
- No test depends on a commercial game ROM.

## 8. Main risks and mitigations

| Risk | Mitigation |
|---|---|
| Existing control socket has no replies | unique artifacts + deadlines in v0.1; acknowledged protocol in v0.2 |
| Headless `-b` is finite-batch, not a persistent deterministic harness | use normal managed execution for v0.1; add explicit pause/advance in v0.2 |
| GDB packet/register behavior differs by CPU core | pin fork SHA and test both supported build modes before widening support |
| Rendering may still require SDL/display setup | CI smoke test with a virtual display; document software renderer fallback |
| Fork drifts from upstream BlastEM | pinned submodule, compatibility probe, scheduled/manual update issue |
| Tool surface grows too quickly | keep raw primitives in v0.1 and require evidence before adding decoded/high-level tools |

## 9. Definition of done for the project plan

- The public repository exists under `akiyan/BlastEM-MCP`.
- The selected BlastEM fork and exact starting revision are recorded and pinned.
- M0/M1/M2 work is represented as milestones and scoped issues.
- Every v0.1 tool has a backend and observable acceptance criterion.
- Known gaps—especially frame advance, save/load, and missing socket replies—are
  explicit rather than hidden inside implementation tasks.
