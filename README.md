# BlastEM MCP

An MCP server for controlling and debugging Sega Mega Drive/Genesis software in
BlastEM from coding agents.

> [!IMPORTANT]
> This repository is in the planning/bootstrap phase. The implementation is
> tracked in GitHub issues and in [`docs/PLAN.md`](docs/PLAN.md).

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

## Planned MVP

- Start, inspect, and stop one managed BlastEM session.
- Press and release Mega Drive controller buttons without desktop automation.
- Capture the current frame and a VRAM/CRAM/VSRAM/VDP-register snapshot.
- Read and write 68000 memory and registers.
- Add/remove breakpoints, interrupt, continue, and single-step the 68000.
- Run locally over MCP stdio on Linux.

Deterministic `step_frames`, frame-counted button presses, reset, and save/load
state are the next emulator-extension milestone because the selected fork does
not currently expose them through its control socket.

## Repository layout

```text
docs/                  architecture, milestones, and decisions
vendor/blastem/        pinned upstream fork (Git submodule)
src/                   MCP server (planned)
test/                  protocol and integration tests (planned)
```

## Documentation

- [Implementation plan](docs/PLAN.md)
- [ADR 0001: use the ulalume BlastEM fork](docs/adr/0001-use-ulalume-blastem.md)

## ROM policy

No commercial ROMs are included. Automated tests will use a small,
redistributable homebrew fixture built from source.

## License

The MCP wrapper and original documentation in this repository are licensed
under the MIT License. The BlastEM submodule is a separate GPL-3.0 project and
retains its own license and copyright notices. Any derivative patches to
BlastEM must follow its applicable license.

