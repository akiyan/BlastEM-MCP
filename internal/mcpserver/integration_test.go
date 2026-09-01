package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPBlastEMIntegration(t *testing.T) {
	binary := os.Getenv("BLASTEM_INTEGRATION_BINARY")
	rom := os.Getenv("BLASTEM_INTEGRATION_ROM")
	if binary == "" || rom == "" {
		t.Skip("set BLASTEM_INTEGRATION_BINARY and BLASTEM_INTEGRATION_ROM to run")
	}
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	app := New("integration-test")
	t.Cleanup(func() { _ = app.Close() })
	serverSession, err := app.Server().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "integration-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	start := callOK(t, ctx, clientSession, "blastem_start", map[string]any{
		"binary_path": binary, "rom_path": rom, "debug": false,
	})
	time.Sleep(2 * time.Second)
	callOK(t, ctx, clientSession, "button_down", map[string]any{"pad": 1, "button": "a"})
	callOK(t, ctx, clientSession, "button_up", map[string]any{"pad": 1, "button": "a"})
	time.Sleep(3 * time.Second)
	screenshot := callOK(t, ctx, clientSession, "screenshot", map[string]any{})
	if len(screenshot.Content) != 1 {
		t.Fatalf("screenshot content count = %d", len(screenshot.Content))
	}
	image, ok := screenshot.Content[0].(*mcp.ImageContent)
	if !ok || len(image.Data) < 8 || string(image.Data[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("screenshot did not return a PNG image: %#v", screenshot.Content[0])
	}
	snapshot := callOK(t, ctx, clientSession, "vdp_snapshot", map[string]any{})
	tiles := callOK(t, ctx, clientSession, "vram_tiles", map[string]any{"scale": 2})
	if len(tiles.Content) != 4 {
		t.Fatalf("vram_tiles content count = %d", len(tiles.Content))
	}
	paletteImages := make([]*mcp.ImageContent, 4)
	for palette := range 4 {
		paletteImage, ok := tiles.Content[palette].(*mcp.ImageContent)
		if !ok || paletteImage.MIMEType != "image/png" || len(paletteImage.Data) < 8 {
			t.Fatalf("vram_tiles palette %d did not return a PNG image: %#v", palette, tiles.Content[palette])
		}
		paletteImages[palette] = paletteImage
	}
	showcaseDir := os.Getenv("BLASTEM_SHOWCASE_DIR")
	if showcaseDir != "" {
		if err := os.MkdirAll(showcaseDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(showcaseDir, "screenshot.png"), image.Data, 0o644); err != nil {
			t.Fatal(err)
		}
		for palette, paletteImage := range paletteImages {
			name := fmt.Sprintf("vram-palette-%d.png", palette)
			if err := os.WriteFile(filepath.Join(showcaseDir, name), paletteImage.Data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	callOK(t, ctx, clientSession, "blastem_stop", map[string]any{})

	debugStart := callOK(t, ctx, clientSession, "blastem_start", map[string]any{
		"binary_path": binary, "rom_path": rom, "debug": true,
	})
	registers := callOK(t, ctx, clientSession, "cpu_registers", map[string]any{})
	memory := callOK(t, ctx, clientSession, "memory_read", map[string]any{"address": 0, "size": 32})
	step := callOK(t, ctx, clientSession, "cpu_step", map[string]any{})
	registersAfterStep := callOK(t, ctx, clientSession, "cpu_registers", map[string]any{})
	if showcaseDir != "" {
		writeShowcaseJSON(t, filepath.Join(showcaseDir, "mcp-data.json"), map[string]any{
			"rom": filepath.Base(rom), "start": start.StructuredContent,
			"vdp_snapshot": snapshot.StructuredContent, "vram_tiles": tiles.StructuredContent,
			"debug_start": debugStart.StructuredContent, "cpu_registers": registers.StructuredContent,
			"memory_read_0x000000": memory.StructuredContent, "cpu_step": step.StructuredContent,
			"cpu_registers_after_step": registersAfterStep.StructuredContent,
		})
	}
	callOK(t, ctx, clientSession, "blastem_stop", map[string]any{})
}

func writeShowcaseJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func callOK(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s protocol error: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s tool error: %+v", name, result.Content)
	}
	return result
}
