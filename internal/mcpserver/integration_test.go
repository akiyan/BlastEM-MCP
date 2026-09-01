package mcpserver

import (
	"context"
	"os"
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

	callOK(t, ctx, clientSession, "blastem_start", map[string]any{
		"binary_path": binary, "rom_path": rom, "debug": false,
	})
	callOK(t, ctx, clientSession, "button_down", map[string]any{"pad": 1, "button": "a"})
	callOK(t, ctx, clientSession, "button_up", map[string]any{"pad": 1, "button": "a"})
	screenshot := callOK(t, ctx, clientSession, "screenshot", map[string]any{})
	if len(screenshot.Content) != 1 {
		t.Fatalf("screenshot content count = %d", len(screenshot.Content))
	}
	image, ok := screenshot.Content[0].(*mcp.ImageContent)
	if !ok || len(image.Data) < 8 || string(image.Data[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("screenshot did not return a PNG image: %#v", screenshot.Content[0])
	}
	callOK(t, ctx, clientSession, "vdp_snapshot", map[string]any{})
	callOK(t, ctx, clientSession, "blastem_stop", map[string]any{})

	callOK(t, ctx, clientSession, "blastem_start", map[string]any{
		"binary_path": binary, "rom_path": rom, "debug": true,
	})
	callOK(t, ctx, clientSession, "cpu_registers", map[string]any{})
	callOK(t, ctx, clientSession, "memory_read", map[string]any{"address": 0, "size": 8})
	callOK(t, ctx, clientSession, "cpu_step", map[string]any{})
	callOK(t, ctx, clientSession, "blastem_stop", map[string]any{})
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
