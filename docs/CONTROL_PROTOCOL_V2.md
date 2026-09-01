# BlastEM acknowledged control protocol v2

Status: accepted for M2 implementation (2026-09-01)

The transport remains the `AF_UNIX` stream selected by `BLASTEM_CTRL_SOCK`.
V2 uses one UTF-8 JSON object plus LF per message. Lines are limited to 65,536
bytes, nesting to 16 levels, and strings to 4,096 bytes. Duplicate keys,
invalid UTF-8, non-integer numbers, and unknown top-level fields are errors.
One client and one outstanding request are allowed.

## Negotiation and replies

The first non-whitespace byte selects the connection mode: `{` selects v2;
anything else selects the unchanged legacy newline parser for that connection.
A v2 client first sends:

```json
{"id":1,"version":2,"command":"hello","params":{"client":"blastem-mcp"}}
```

The reply reports `server`, `protocol`, numeric limits, and capabilities chosen
from `frame_control`, `frame_input`, `reset`, `state`, and `artifacts`.
Unsupported versions receive `unsupported_version` before disconnect. Clients
gate optional commands on capabilities, not the version alone.

IDs are unique unsigned integers in 1..2^53-1 and are echoed. Every accepted
request gets exactly one ordered reply:

```json
{"id":2,"version":2,"ok":true,"result":{"frame":1234,"paused":true}}
{"id":3,"version":2,"ok":false,"error":{"code":"invalid_params","message":"frames must be 1..1000000"}}
```

Stable error codes are `invalid_request`, `invalid_params`, `unknown_command`,
`unsupported`, `invalid_state`, `not_found`, `io_error`, `busy`, and
`internal`. Diagnostic messages are not API. State-changing success replies
include the absolute unsigned 64-bit VDP `frame` and `paused` state.

## Boundary semantics

A boundary is immediately after rendering a complete frame and before CPU work
for the next one. Requests execute on the emulation thread.

- `status`: return frame, pause state, system, ROM SHA-256, and capabilities.
- `pause`: stop at the next boundary; already paused is a no-op.
- `resume`: resume after replying; already running is a no-op.
- `advance {"frames":N}`: while paused, run exactly N frames and pause again;
  N is 1..1,000,000.
- `press_frames {"pad":P,"button":B,"frames":N}`: while paused, press at the
  current boundary, advance N frames, release at the final boundary, and remain
  paused. Release is guaranteed on failure. P is 1 or 2 and B is a legacy
  button name.
- `reset {"kind":"soft"|"hard"}`: reset at a boundary, clear injected input,
  set frame to zero, and preserve the prior paused/running state. Hard reset
  reconstructs the system without changing ROM.
- `save_state {"name":S}`: while paused, atomically save complete deterministic
  machine state, frame, and injected input.
- `load_state {"name":S}`: while paused, atomically validate and restore it,
  then remain paused.
- `pad`, `screenshot`, and `vramdump`: acknowledged v2 forms; artifact replies
  identify the boundary frame and atomic-write result.

## State and path safety

`BLASTEM_STATE_DIR` names a private state directory. State commands never
accept paths. Names are 1..64 ASCII characters matching
`[A-Za-z0-9][A-Za-z0-9._-]*`, except `.` and `..`, and map to
`<dir>/<name>.state`. Symlinks and non-regular files are rejected. Writes use a
temporary regular file, `fsync`, and atomic rename. Headers contain format
version, ROM SHA-256, system ID, payload length, and checksum; mismatches are
rejected before machine mutation.

Artifact paths must be absolute and contain no CR/LF/NUL. MCP additionally
restricts them beneath its private artifact directory.

## Cancellation, compatibility, and determinism

Clients enforce wall-clock deadlines; the wire request has no host-time field.
Disconnect cancels work not started. An operation already running finishes at
its exact boundary, performs input cleanup, and drops its reply; a new client
can inspect `status`.

MCP probes with a short v2 `hello`. No reply means a legacy fork, exposing only
v0.1 tools. JSON and legacy commands are never mixed on one connection. All
existing legacy commands remain byte-for-byte compatible and unacknowledged.

For the same BlastEM revision, ROM hash, initial state, region, configuration,
and command sequence, boundary frame numbers and state hashes must match across
runs. Host pacing, timestamps, window state, and logs are excluded. Real-time,
network, and nondeterministic host devices must be disabled or reported as
unsupported.
