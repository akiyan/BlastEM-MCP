# Linux setup and MCP host configuration

BlastEM MCP runs as a local stdio MCP server. It starts one pinned BlastEM
process, connects to the fork's private Unix control socket, and optionally
connects to its loopback-only GDB Remote endpoint.

## Supported environment

- Linux x86-64;
- Go 1.27 or newer;
- an X11 display, or Xvfb for headless use;
- the pinned `ulalume/blastem` submodule revision documented in
  [COMPATIBILITY.md](COMPATIBILITY.md).

Ubuntu/Debian runtime and build dependencies:

```sh
sudo apt-get update
sudo apt-get install -y \
  pkg-config libsdl2-dev libglew-dev libgl-dev
```

The source-built integration fixture additionally requires:

```sh
sudo apt-get install -y binutils-m68k-linux-gnu xvfb xauth
```

## Clean checkout and build

```sh
git clone --recurse-submodules https://github.com/akiyan/BlastEM-MCP.git
cd BlastEM-MCP

make -C third_party/blastem -j2 DEBUG=1
make check
make build
```

If the repository was cloned without submodules:

```sh
git submodule update --init --recursive
```

The resulting MCP executable is `./blastem-mcp`. The BlastEM backend is
`./third_party/blastem/blastem`.

To build and validate the redistributable test ROM:

```sh
make fixture
```

Generated fixture ROMs and local ROMs are ignored by Git. Do not add
copyrighted or otherwise non-redistributable ROMs to the repository.

## Start the stdio server

An MCP host should launch this command and communicate over stdin/stdout:

```sh
/absolute/path/to/BlastEm-MCP/blastem-mcp
```

Do not wrap the command with anything that writes banners or diagnostics to
stdout because stdout carries MCP protocol messages.

Many desktop MCP hosts use a configuration shaped like this:

```json
{
  "mcpServers": {
    "blastem": {
      "command": "/absolute/path/to/BlastEm-MCP/blastem-mcp",
      "args": []
    }
  }
}
```

Use the equivalent local stdio-server configuration if a host uses TOML, YAML,
or a graphical settings page. No server arguments or network listener are
required.

The first tool call starts the emulator:

```json
{
  "binary_path": "/absolute/path/to/BlastEm-MCP/third_party/blastem/blastem",
  "rom_path": "/absolute/path/to/local-or-fixture.bin",
  "debug": true
}
```

Set `debug` to `true` only when CPU registers, memory, breakpoints, or stepping
will be used. Video and controller tools do not require debug mode.

## Tool reference

| Tool | Purpose | Requirement |
|---|---|---|
| `blastem_start` | start one managed emulator process | BlastEM and ROM paths |
| `blastem_status` | inspect PID, ROM, runtime directory, and endpoints | none |
| `blastem_stop` | release held inputs and stop/clean the session | running session |
| `button_down` | hold a pad 1/2 button | running session |
| `button_up` | release a pad 1/2 button | running session |
| `release_all_buttons` | release all buttons held through MCP | running session |
| `screenshot` | return the next rendered frame as MCP PNG image content | running session |
| `vdp_snapshot` | capture and validate `KITVDMP1` VRAM/CRAM/VSRAM/register data | running session |
| `vram_tiles` | return four PNGs, one per live CRAM palette | running session |
| `cpu_registers` | read D0-D7, A0-A7, SR, and PC | debug session, CPU stopped |
| `memory_read` | read up to 64 KiB of 68000 memory | debug session, CPU stopped |
| `memory_write` | write up to 64 KiB of hexadecimal data | debug session, CPU stopped |
| `breakpoint_set` | install a 24-bit 68000 software breakpoint | debug session, CPU stopped |
| `breakpoint_remove` | remove a software breakpoint | debug session, CPU stopped |
| `cpu_step` | execute one 68000 instruction | debug session, CPU stopped |
| `cpu_continue` | continue until an installed breakpoint | debug session, breakpoint set |

Controller button names are `up`, `down`, `left`, `right`, `a`, `b`, `c`, `x`,
`y`, `z`, `start`, and `mode`. Opposing directions on the same pad are rejected.

`vram_tiles` captures a reference screenshot and VDP dump together at the next
frame boundary. Its four returned images are ordered by CRAM palette number
0-3. Palette colour index zero uses a checkerboard to represent transparency.

## Headless execution and acceptance tests

On a machine without an X server:

```sh
xvfb-run -a make integration-fixture
```

With an existing display:

```sh
DISPLAY=:99 make integration-fixture
```

This builds the fixture and runs the MCP, control-socket, and GDB acceptance
tests against the pinned BlastEM fork. CI runs the same command under Xvfb.

## Runtime files and logs

Each session receives a private temporary runtime directory. Its absolute path
is returned by `blastem_start` and `blastem_status`. It contains:

- `blastem.stdout.log` and `blastem.stderr.log`;
- the Unix control socket;
- screenshots and VDP/VRAM artifacts created during the session.

Normal `blastem_stop` removes the runtime directory. If BlastEM exits
unexpectedly, call `blastem_status` and inspect the reported runtime directory
and `exit_error` before starting another session or stopping it. Copy any logs
needed for diagnosis before cleanup.

The MCP server itself must keep stdout protocol-clean. Host-side server launch
errors should be read from the MCP host's stderr/server log facility.

## Security and operational limits

- The server accepts only local regular-file paths for the BlastEM binary and
  ROM. The backend must have executable permission.
- GDB Remote uses a dynamically allocated loopback address; it is not exposed
  on external interfaces.
- Control traffic uses a private Unix socket in the session runtime directory.
- Only one managed BlastEM session is supported per MCP server process.
- Memory reads and writes are capped at 64 KiB per tool call.
- Artifact paths generated by MCP remain inside the private runtime directory.
- ROM code is untrusted native-emulator input. Use ROMs from trusted sources
  and run the MCP server with the permissions of a non-privileged user.
- `blastem_stop` releases MCP-held controller inputs and escalates from SIGINT
  to process termination after the shutdown timeout.

## v0.1 timing limitation

Controller commands use the fork's fire-and-forget newline protocol. They are
not acknowledged by a frame number, and v0.1 cannot advance an exact number of
frames. Host scheduling can therefore change which frame observes a button
transition. The source fixture exposes RAM markers so tests can observe input,
but deterministic frame-counted input is planned for v0.2.

The fork also ignores the raw GDB Ctrl-C byte while the CPU is running.
`cpu_continue` must target an installed breakpoint; it cannot currently be
cancelled into an arbitrary running instruction.

## Troubleshooting

### BlastEM cannot open a display

Set `DISPLAY` to a working X server, or start the host/server under
`xvfb-run -a`. The current backend still requires an X display even with
`BLASTEM_NO_GUI=1`.

### ALSA or audio-device errors

For headless tests, set:

```sh
export SDL_AUDIODRIVER=dummy
```

The integration tests set this automatically for managed BlastEM sessions.

### The BlastEM binary is missing

Verify the submodule and rebuild:

```sh
git submodule status
make -C third_party/blastem -j2 DEBUG=1
```

### `cpu_continue` waits indefinitely

Install a reachable breakpoint first. Use the fixture's generated
`symbols.txt` for known addresses. Cancellation cannot interrupt a running CPU
with the pinned fork.

### Artifact requests time out

Confirm BlastEM is still running and rendering frames, inspect
`blastem.stderr.log`, and verify the runtime filesystem has free space. Set
`TMPDIR` to a writable filesystem with sufficient quota if `/tmp` is limited.

### A previous session appears stuck

Call `blastem_status`, then `blastem_stop`. Starting a second session is
rejected while the first process is running.
