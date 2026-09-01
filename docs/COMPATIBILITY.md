# BlastEM backend compatibility

Last verified: 2026-09-01

## Pinned backend

- Repository: `ulalume/blastem`
- Revision: `4b71bfec3b5919367c5843f23c22c76ac24aa38c`
- Reported version: `0.6.3-pre`
- Host: Linux x86-64
- Build used for functional verification: `make DEBUG=1`

The fork builds with `pkg-config`, SDL2, GLEW, and OpenGL development packages.
Its large generated SH2 source makes optimized builds comparatively slow, so CI
uses a debug build for the backend compile check.

## Verified control-socket behavior

The automated integration test passed against two local SGDK-generated Mega
Drive ROMs. The ROM binaries remain local and are excluded by `.gitignore`.

- connect to `BLASTEM_CTRL_SOCK`;
- `pad 1 down a` and `pad 1 up a`;
- `screenshot <path>` producing a valid PNG signature;
- `vramdump <path>` producing a valid `KITVDMP1` signature;
- `vram_tiles` parsing the 65,840-byte snapshot and returning four separate
  live-CRAM-palette PNG tilesheets for all 2,048 VRAM tiles;
- release held input and stop without an orphan BlastEM process.

## Verified GDB Remote behavior

The automated integration test passed against both local ROMs with
`BLASTEM_GDB_PORT` bound to loopback and BlastEM started with `-D`.

- initial stop query returns signal 5;
- all D0-D7, A0-A7, SR, and PC values parse correctly;
- ROM vector memory read;
- work-RAM read/write/read-back and restoration;
- software breakpoint add/remove;
- single instruction step returning signal 5;
- packet checksums and acknowledgements.

## Known fork limitations

- The existing control socket is fire-and-forget and has no response framing.
- No persistent deterministic `advance <frames>` command is exposed.
- Save/load state and reset are not exposed over the control socket.
- The GDB stub ignores the raw Ctrl-C byte while the CPU is running. For v0.1,
  `cpu_continue` must be paired with a breakpoint. Arbitrary break/cancellation
  needs an emulator-side extension.
- Only one control-socket client and one GDB client are supported.

## Reproduce

With a local ROM and a running X display:

```sh
make -C third_party/blastem -j2 DEBUG=1

BLASTEM_INTEGRATION_BINARY="$PWD/third_party/blastem/blastem" \
BLASTEM_INTEGRATION_ROM="/absolute/path/to/local.bin" \
go test -race -run 'TestBlastEM(Control|GDB)Integration' -v ./internal/session

BLASTEM_INTEGRATION_BINARY="$PWD/third_party/blastem/blastem" \
BLASTEM_INTEGRATION_ROM="/absolute/path/to/local.bin" \
go test -race -run TestMCPBlastEMIntegration -v ./internal/mcpserver
```
