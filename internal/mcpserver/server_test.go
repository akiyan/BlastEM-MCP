package mcpserver

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAdvertisesImplementedTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	app := New("test")
	t.Cleanup(func() { _ = app.Close() })
	serverSession, err := app.Server().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)
	want := []string{
		"blastem_start", "blastem_status", "blastem_stop", "breakpoint_remove",
		"breakpoint_set", "button_down", "button_up", "cpu_continue",
		"cpu_registers", "cpu_step", "memory_read", "memory_write",
		"release_all_buttons", "screenshot", "vdp_snapshot",
		"vram_tiles",
	}
	if len(got) != len(want) {
		t.Fatalf("tools = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tools = %v, want %v", got, want)
		}
	}

	status, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "blastem_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if status.IsError {
		t.Fatalf("blastem_status returned tool error: %+v", status.Content)
	}

	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"blastem_stop", map[string]any{}},
		{"button_down", map[string]any{"pad": 1, "button": "a"}},
		{"screenshot", map[string]any{}},
		{"vdp_snapshot", map[string]any{}},
		{"vram_tiles", map[string]any{}},
		{"cpu_registers", map[string]any{}},
		{"memory_read", map[string]any{"address": 0, "size": 1}},
		{"breakpoint_set", map[string]any{"address": 0x200}},
	} {
		result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: test.name, Arguments: test.args})
		if err != nil {
			t.Fatalf("%s protocol error: %v", test.name, err)
		}
		if !result.IsError {
			t.Fatalf("%s succeeded without a running session", test.name)
		}
	}
}
