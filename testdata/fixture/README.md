# Mega Drive integration fixture

This directory contains the source for a small, deterministic Sega Mega
Drive/Genesis ROM used by BlastEM MCP integration tests. The generated ROM is
not checked in; CI builds it from the GNU m68k binutils assembler source.

## Build

On Ubuntu/Debian:

```sh
sudo apt-get install binutils-m68k-linux-gnu
make -C testdata/fixture check
```

Outputs are written below `build/`:

- `blastem-mcp-fixture.bin`: 64 KiB ROM;
- `blastem-mcp-fixture.elf`: debugger symbols;
- `symbols.txt`: stable symbol addresses;
- `fixture.map`: linker map.

## Observable contract

The ROM initializes 16 known tiles and four distinct CRAM palettes, displays
the combinations in four horizontal bands, and polls controller port 1 once per
frame. Work RAM exposes these stable locations:

| Address | Symbol | Meaning |
|---:|---|---|
| `0xFF0000` | `fixture_magic` | ASCII `BMCP` (`0x424D4350`) |
| `0xFF0004` | `fixture_frame_counter` | incremented once per observed VBlank |
| `0xFF0008` | `fixture_input_state` | active-high U,D,L,R,B,C,A,Start bits 0-7 |
| `0xFF000A` | `fixture_raw_high` | raw controller TH-high byte |
| `0xFF000B` | `fixture_raw_low` | raw controller TH-low byte |
| `0xFF0010` | `fixture_scratch` | writable marker initialized to `0x11223344` |

`fixture_breakpoint` executes once per frame immediately before the counter and
input update. Tests should resolve its address from `symbols.txt`, avoiding a
hard-coded code address.

Run the MCP contract test against a built BlastEM backend with:

```sh
DISPLAY=:99 \
BLASTEM_INTEGRATION_BINARY="$PWD/third_party/blastem/blastem" \
BLASTEM_FIXTURE_ROM="$PWD/testdata/fixture/build/blastem-mcp-fixture.bin" \
BLASTEM_FIXTURE_SYMBOLS="$PWD/testdata/fixture/build/symbols.txt" \
go test -run TestFixtureMCPContract -v ./internal/mcpserver
```

To run the complete v0.1 acceptance suite with an existing X display:

```sh
DISPLAY=:99 make integration-fixture
```

The fixture source and build files are licensed under the repository's MIT
License. It contains no proprietary code, graphics, audio, or other assets.
