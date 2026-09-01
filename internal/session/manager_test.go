package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerRejectsInvalidPaths(t *testing.T) {
	manager := NewManager()
	rom := writeTestFile(t, "rom.bin", []byte("fixture"), 0o600)
	nonExecutable := writeTestFile(t, "blastem", []byte("not executable"), 0o600)
	if _, err := manager.Start(StartConfig{BinaryPath: "missing", ROMPath: rom}); err == nil {
		t.Fatal("Start accepted a missing binary")
	}
	if _, err := manager.Start(StartConfig{BinaryPath: nonExecutable, ROMPath: rom}); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("non-executable binary error = %v", err)
	}
	executable := writeExecutable(t, "exit 0")
	if _, err := manager.Start(StartConfig{BinaryPath: executable, ROMPath: "missing"}); err == nil {
		t.Fatal("Start accepted a missing ROM")
	}
	if _, err := manager.Start(StartConfig{BinaryPath: executable, ROMPath: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory ROM error = %v", err)
	}
}

func TestManagerLifecycleAndDoubleStart(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	manager := NewManager()
	t.Cleanup(func() { _ = manager.Close() })
	rom := writeTestFile(t, "rom.bin", []byte("fixture"), 0o600)
	executable := writeExecutable(t, "trap 'exit 0' INT TERM\nwhile :; do sleep 0.05; done")
	status, err := manager.Start(StartConfig{BinaryPath: executable, ROMPath: rom})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running || status.PID == 0 || status.RuntimeDir == "" || status.ControlSock == "" {
		t.Fatalf("start status = %+v", status)
	}
	if _, err := os.Stat(status.RuntimeDir); err != nil {
		t.Fatalf("runtime directory: %v", err)
	}
	if _, err := manager.Start(StartConfig{BinaryPath: executable, ROMPath: rom}); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Start error = %v", err)
	}
	if _, _, err := manager.Control(); err != nil {
		t.Fatalf("Control while running: %v", err)
	}
	if _, err := manager.GDB(); err == nil || !strings.Contains(err.Error(), "without debug mode") {
		t.Fatalf("GDB for non-debug session error = %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(status.RuntimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime directory remains after Stop: %v", err)
	}
	if _, _, err := manager.Control(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Control after Stop error = %v", err)
	}
	if err := manager.Stop(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("second Stop error = %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("idempotent Close: %v", err)
	}
}

func TestManagerReportsUnexpectedExitAndCleansIt(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	manager := NewManager()
	rom := writeTestFile(t, "rom.bin", []byte("fixture"), 0o600)
	executable := writeExecutable(t, "exit 7")
	status, err := manager.Start(StartConfig{BinaryPath: executable, ROMPath: rom})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status = manager.Status()
		if !status.Running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.Running || !strings.Contains(status.ExitError, "exit status 7") {
		t.Fatalf("unexpected-exit status = %+v", status)
	}
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(status.RuntimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected-exit runtime directory remains: %v", err)
	}
}

func writeExecutable(t *testing.T, body string) string {
	t.Helper()
	return writeTestFile(t, "fake-blastem.sh", []byte("#!/bin/sh\n"+body+"\n"), 0o700)
}

func writeTestFile(t *testing.T, name string, data []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	return path
}
