# Contributing

The project is currently executing the milestones in `docs/PLAN.md`.

Before opening a change:

1. Do not add commercial ROMs, BIOS images, or other copyrighted game assets.
2. Keep MCP code outside the BlastEM submodule.
3. Put BlastEM changes in focused commits/patches and preserve its license.
4. Add tests for protocol parsing, timeouts, cleanup, and error paths.
5. Never write diagnostic output to MCP stdio stdout; use stderr.

Bug reports should include the host OS, Go version, BlastEM commit, build
options/CPU core, MCP host, reproduction steps, and sanitized logs.
