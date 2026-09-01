AS := m68k-linux-gnu-as
LD := m68k-linux-gnu-ld
OBJCOPY := m68k-linux-gnu-objcopy
NM := m68k-linux-gnu-nm

BUILD_DIR := build
OBJECT := $(BUILD_DIR)/fixture.o
ELF := $(BUILD_DIR)/blastem-mcp-fixture.elf
ROM := $(BUILD_DIR)/blastem-mcp-fixture.bin
SYMBOLS := $(BUILD_DIR)/symbols.txt

.PHONY: all clean check

all: $(ROM) $(SYMBOLS)

$(BUILD_DIR):
	mkdir -p $@

$(OBJECT): fixture.s | $(BUILD_DIR)
	$(AS) -m68000 --register-prefix-optional -o $@ $<

$(ELF): $(OBJECT) linker.ld
	$(LD) --build-id=none -T linker.ld -Map $(BUILD_DIR)/fixture.map -o $@ $(OBJECT)

$(ROM): $(ELF)
	$(OBJCOPY) -O binary $< $@

$(SYMBOLS): $(ELF)
	$(NM) -n $< > $@

check: all
	test "$$(wc -c < $(ROM))" -eq 65536
	grep -q ' fixture_breakpoint$$' $(SYMBOLS)
	grep -q ' fixture_frame_counter$$' $(SYMBOLS)
	grep -q ' fixture_input_state$$' $(SYMBOLS)

clean:
	$(RM) -r $(BUILD_DIR)
