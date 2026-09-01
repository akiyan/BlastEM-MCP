package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFixtureMCPContract(t *testing.T) {
	binary := os.Getenv("BLASTEM_INTEGRATION_BINARY")
	rom := os.Getenv("BLASTEM_FIXTURE_ROM")
	symbols := os.Getenv("BLASTEM_FIXTURE_SYMBOLS")
	if binary == "" || rom == "" || symbols == "" {
		t.Skip("set BLASTEM_INTEGRATION_BINARY, BLASTEM_FIXTURE_ROM, and BLASTEM_FIXTURE_SYMBOLS to run")
	}
	breakpoint := fixtureSymbol(t, symbols, "fixture_breakpoint")
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	app := New("fixture-integration-test")
	t.Cleanup(func() { _ = app.Close() })
	serverSession, err := app.Server().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	callOK(t, ctx, clientSession, "blastem_start", map[string]any{
		"binary_path": binary, "rom_path": rom, "debug": true,
	})
	callOK(t, ctx, clientSession, "breakpoint_set", map[string]any{"address": breakpoint})
	callOK(t, ctx, clientSession, "cpu_continue", map[string]any{})

	markers := fixtureMemory(t, callOK(t, ctx, clientSession, "memory_read", map[string]any{
		"address": 0xFF0000, "size": 20,
	}))
	if got := string(markers[0:4]); got != "BMCP" {
		t.Fatalf("fixture magic = %q, want BMCP", got)
	}
	if got := markers[16:20]; string(got) != "\x11\x22\x33\x44" {
		t.Fatalf("fixture scratch = %X, want 11223344", got)
	}

	callOK(t, ctx, clientSession, "button_down", map[string]any{"pad": 1, "button": "a"})
	callOK(t, ctx, clientSession, "cpu_continue", map[string]any{})
	input := fixtureMemory(t, callOK(t, ctx, clientSession, "memory_read", map[string]any{
		"address": 0xFF0008, "size": 2,
	}))
	if input[1]&0x40 == 0 {
		t.Fatalf("fixture input state = %02X%02X, A bit is clear", input[0], input[1])
	}

	callOK(t, ctx, clientSession, "memory_write", map[string]any{"address": 0xFF0010, "hex": "A5A55A5A"})
	scratch := fixtureMemory(t, callOK(t, ctx, clientSession, "memory_read", map[string]any{
		"address": 0xFF0010, "size": 4,
	}))
	if got := strings.ToUpper(jsonHex(scratch)); got != "A5A55A5A" {
		t.Fatalf("fixture scratch after write = %s", got)
	}

	callOK(t, ctx, clientSession, "button_up", map[string]any{"pad": 1, "button": "a"})
	callOK(t, ctx, clientSession, "cpu_continue", map[string]any{})
	input = fixtureMemory(t, callOK(t, ctx, clientSession, "memory_read", map[string]any{
		"address": 0xFF0008, "size": 2,
	}))
	if input[1]&0x40 != 0 {
		t.Fatalf("fixture input state = %02X%02X, A bit remains set after release", input[0], input[1])
	}
	callOK(t, ctx, clientSession, "breakpoint_remove", map[string]any{"address": breakpoint})
	callOK(t, ctx, clientSession, "blastem_stop", map[string]any{})
}

func fixtureSymbol(t *testing.T, path, name string) uint32 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 3 && fields[2] == name {
			value, err := strconv.ParseUint(fields[0], 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			return uint32(value)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("symbol %q not found in %s", name, path)
	return 0
}

func fixtureMemory(t *testing.T, result *mcp.CallToolResult) []byte {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output memoryReadOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, output.Size)
	for i := range data {
		value, err := strconv.ParseUint(output.Hex[i*2:i*2+2], 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		data[i] = byte(value)
	}
	return data
}

func jsonHex(data []byte) string {
	const digits = "0123456789ABCDEF"
	result := make([]byte, len(data)*2)
	for i, value := range data {
		result[i*2] = digits[value>>4]
		result[i*2+1] = digits[value&15]
	}
	return string(result)
}
