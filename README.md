# BlastEM MCP

An MCP server for controlling and debugging Sega Mega Drive/Genesis software in
BlastEM from coding agents.

> [!IMPORTANT]
> The v0.1 implementation is in progress. Session management, controller input,
> screenshots, VDP snapshots, and the initial 68000 GDB tools are implemented;
> deterministic frame stepping and save/load state remain planned work.

The first implementation targets
[`ulalume/blastem`](https://github.com/ulalume/blastem), a BlastEM fork that
already exposes:

- a Unix-domain control socket for gamepad input, screenshots, profiling, and
  VDP memory snapshots;
- GDB Remote over a localhost TCP port for 68000 registers, memory,
  breakpoints, continue, and single-step;
- headless/batch execution support.

The MCP server remains a separate process. It translates MCP tools into the
fork's control-socket protocol and GDB Remote Serial Protocol, keeping
MCP-specific code out of the emulator.

## Current v0.1 tool surface

- Start, inspect, and stop one managed BlastEM session.
- Press and release Mega Drive controller buttons without desktop automation.
- Capture the current frame and a VRAM/CRAM/VSRAM/VDP-register snapshot.
- Read and write 68000 memory and registers.
- Add/remove breakpoints, continue-to-breakpoint, and single-step the 68000.
- Run locally over MCP stdio on Linux.

Deterministic `step_frames`, frame-counted button presses, reset, and save/load
state are the next emulator-extension milestone because the selected fork does
not currently expose them through its control socket.

## Build

```sh
git clone --recurse-submodules https://github.com/akiyan/BlastEM-MCP.git
cd BlastEM-MCP

# Ubuntu/Debian BlastEM dependencies
sudo apt-get install pkg-config libsdl2-dev libglew-dev libgl-dev

make -C third_party/blastem -j2 DEBUG=1
make check
make build
```

`blastem-mcp` serves MCP over stdin/stdout. Pass the built BlastEM executable
and a local ROM path to the `blastem_start` tool. ROM files under `roms/` are
intentionally ignored; only `roms/.gitkeep` is tracked.

For local emulator integration tests, start an X display (or Xvfb) and run:

```sh
BLASTEM_INTEGRATION_BINARY="$PWD/third_party/blastem/blastem" \
BLASTEM_INTEGRATION_ROM="/absolute/path/to/test.bin" \
go test -run 'TestBlastEM(Control|GDB)Integration' -v ./internal/session
```

## Repository layout

```text
docs/                  architecture, milestones, and decisions
third_party/blastem/   pinned upstream fork (Git submodule)
cmd/blastem-mcp/       Go MCP server entry point
internal/              session and protocol packages
testdata/              open integration fixtures (planned)
```

## Documentation

- [Implementation plan](docs/PLAN.md)
- [Pinned-backend compatibility evidence](docs/COMPATIBILITY.md)
- [ADR 0001: use the ulalume BlastEM fork](docs/adr/0001-use-ulalume-blastem.md)
- [ADR 0002: implement the MCP server in Go](docs/adr/0002-use-go-for-the-mcp-server.md)

## ROM policy

No commercial ROMs are included. Automated tests will use a small,
redistributable homebrew fixture built from source.

## License

The MCP wrapper and original documentation in this repository are licensed
under the MIT License. The BlastEM submodule is a separate GPL-3.0 project and
retains its own license and copyright notices. Any derivative patches to
BlastEM must follow its applicable license.
