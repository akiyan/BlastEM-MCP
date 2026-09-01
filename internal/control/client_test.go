package control

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestButtonsAndReleaseAll(t *testing.T) {
	client, commands := fakeControlServer(t, nil)
	if err := client.ButtonDown(1, "A"); err != nil {
		t.Fatal(err)
	}
	if err := client.ButtonDown(2, "left"); err != nil {
		t.Fatal(err)
	}
	if err := client.ButtonUp(1, "a"); err != nil {
		t.Fatal(err)
	}
	if err := client.ReleaseAll(); err != nil {
		t.Fatal(err)
	}
	want := []string{"pad 1 down a", "pad 2 down left", "pad 1 up a", "pad 2 up left"}
	for i, expected := range want {
		select {
		case got := <-commands:
			if got != expected {
				t.Fatalf("command %d: got %q, want %q", i, got, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for command %d", i)
		}
	}
}

func TestArtifactCommands(t *testing.T) {
	tests := []struct {
		name, command, extension string
		data                     []byte
		call                     func(*Client, string) error
	}{
		{"screenshot", "screenshot", ".png", []byte("\x89PNG\r\n\x1a\nfixture"), (*Client).Screenshot},
		{"vdp", "vramdump", ".kitvdmp", []byte("KITVDMP1fixture"), (*Client).VDPSnapshot},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, commands := fakeControlServer(t, func(line string) {
				parts := strings.SplitN(line, " ", 2)
				if len(parts) == 2 {
					_ = os.WriteFile(parts[1], test.data, 0o600)
				}
			})
			path := filepath.Join(t.TempDir(), "artifact"+test.extension)
			if err := test.call(client, path); err != nil {
				t.Fatal(err)
			}
			got := <-commands
			if !strings.HasPrefix(got, test.command+" ") {
				t.Fatalf("got command %q", got)
			}
		})
	}
}

func TestVideoStateArmsBothArtifacts(t *testing.T) {
	client, commands := fakeControlServer(t, func(line string) {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			return
		}
		data := []byte("KITVDMP1fixture")
		if strings.HasPrefix(line, "screenshot ") {
			data = []byte("\x89PNG\r\n\x1a\nfixture")
		}
		_ = os.WriteFile(parts[1], data, 0o600)
	})
	dir := t.TempDir()
	if err := client.VideoState(filepath.Join(dir, "frame.png"), filepath.Join(dir, "vdp.kitvdmp")); err != nil {
		t.Fatal(err)
	}
	first, second := <-commands, <-commands
	if !strings.HasPrefix(first, "screenshot ") || !strings.HasPrefix(second, "vramdump ") {
		t.Fatalf("commands = %q, %q", first, second)
	}
}

func TestRejectsInvalidInput(t *testing.T) {
	client := New(filepath.Join(t.TempDir(), "missing.sock"))
	tests := []struct {
		pad    int
		button string
	}{
		{0, "a"}, {3, "a"}, {1, "fire"},
	}
	for _, test := range tests {
		if err := client.ButtonDown(test.pad, test.button); err == nil {
			t.Fatalf("ButtonDown(%d, %q) unexpectedly succeeded", test.pad, test.button)
		}
	}
}

func TestRejectsOpposingDirections(t *testing.T) {
	client, commands := fakeControlServer(t, nil)
	if err := client.ButtonDown(1, "left"); err != nil {
		t.Fatal(err)
	}
	if got := <-commands; got != "pad 1 down left" {
		t.Fatalf("got %q", got)
	}
	if err := client.ButtonDown(1, "right"); err == nil {
		t.Fatal("opposing direction unexpectedly succeeded")
	}
}

func TestButtonSet(t *testing.T) {
	want := []string{"a", "b", "c", "down", "left", "mode", "right", "start", "up", "x", "y", "z"}
	got := make([]string, 0, len(validButtons))
	for button := range validButtons {
		got = append(got, button)
	}
	// Keep the exported behavior intentional when the emulator protocol changes.
	slicesSort(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buttons = %v, want %v", got, want)
	}
}

func fakeControlServer(t *testing.T, onCommand func(string)) (*Client, <-chan string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	commands := make(chan string, 16)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := scanner.Text()
			commands <- line
			if onCommand != nil {
				onCommand(line)
			}
		}
	}()
	client := New(path)
	client.SetTimeout(time.Second)
	t.Cleanup(func() { _ = client.Close() })
	return client, commands
}

func slicesSort(values []string) {
	for i := range values {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
