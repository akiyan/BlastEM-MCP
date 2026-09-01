OUTPUT_ARCH(m68k)
ENTRY(reset)

SECTIONS
{
  . = 0x00000000;
  .vectors : { KEEP(*(.vectors)) }

  . = 0x00000100;
  .header : { KEEP(*(.header)) }

  . = 0x00000200;
  .text :
  {
    *(.text*)
    *(.rodata*)
  }

  . = 0x0000FFFF;
  .rom_end : { BYTE(0) }

  /DISCARD/ : { *(.comment) *(.note*) }
}

ASSERT(SIZEOF(.vectors) == 0x100, "vector table must be 256 bytes")
ASSERT(SIZEOF(.header) == 0x100, "Mega Drive header must be 256 bytes")
ASSERT(. <= 0x10000, "fixture ROM exceeds 64 KiB")
