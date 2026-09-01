package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBlastEMControlIntegration(t *testing.T) {
	binary := os.Getenv("BLASTEM_INTEGRATION_BINARY")
	rom := os.Getenv("BLASTEM_INTEGRATION_ROM")
	if binary == "" || rom == "" {
		t.Skip("set BLASTEM_INTEGRATION_BINARY and BLASTEM_INTEGRATION_ROM to run")
	}
	t.Setenv("SDL_AUDIODRIVER", "dummy")

	manager := NewManager()
	status, err := manager.Start(StartConfig{BinaryPath: binary, ROMPath: rom})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close session: %v", err)
		}
	})
	if !status.Running {
		t.Fatalf("session did not start: %+v", status)
	}

	client, runtimeDir, err := manager.Control()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ButtonDown(1, "a"); err != nil {
		t.Fatal(err)
	}
	if err := client.ButtonUp(1, "a"); err != nil {
		t.Fatal(err)
	}
	if err := client.Screenshot(filepath.Join(runtimeDir, "integration.png")); err != nil {
		t.Fatal(err)
	}
	if err := client.VDPSnapshot(filepath.Join(runtimeDir, "integration.kitvdmp")); err != nil {
		t.Fatal(err)
	}
}

func TestBlastEMGDBIntegration(t *testing.T) {
	binary := os.Getenv("BLASTEM_INTEGRATION_BINARY")
	rom := os.Getenv("BLASTEM_INTEGRATION_ROM")
	if binary == "" || rom == "" {
		t.Skip("set BLASTEM_INTEGRATION_BINARY and BLASTEM_INTEGRATION_ROM to run")
	}
	t.Setenv("SDL_AUDIODRIVER", "dummy")

	manager := NewManager()
	status, err := manager.Start(StartConfig{BinaryPath: binary, ROMPath: rom, Debug: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close session: %v", err)
		}
	})
	if status.GDBAddress == "" {
		t.Fatalf("debug session has no GDB address: %+v", status)
	}
	client, err := manager.GDB()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stop, err := client.StopReason(ctx)
	if err != nil || stop.Signal != 5 {
		t.Fatalf("StopReason = %+v, %v", stop, err)
	}
	registers, err := client.Registers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if registers.PC == 0 {
		t.Fatalf("unexpected zero PC: %+v", registers)
	}
	vectors, err := client.ReadMemory(ctx, 0, 8)
	if err != nil || len(vectors) != 8 {
		t.Fatalf("ReadMemory = %x, %v", vectors, err)
	}
	originalRAM, err := client.ReadMemory(ctx, 0xFF0000, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), time.Second)
		defer restoreCancel()
		_ = client.WriteMemory(restoreCtx, 0xFF0000, originalRAM)
	})
	written := []byte{0x42, 0x4C, 0x53, 0x54}
	if err := client.WriteMemory(ctx, 0xFF0000, written); err != nil {
		t.Fatal(err)
	}
	readBack, err := client.ReadMemory(ctx, 0xFF0000, len(written))
	if err != nil || string(readBack) != string(written) {
		t.Fatalf("RAM round trip = %x, %v", readBack, err)
	}
	if err := client.SetBreakpoint(ctx, registers.PC+2); err != nil {
		t.Fatal(err)
	}
	if err := client.RemoveBreakpoint(ctx, registers.PC+2); err != nil {
		t.Fatal(err)
	}
	step, err := client.Step(ctx)
	if err != nil || step.Signal != 5 {
		t.Fatalf("Step = %+v, %v", step, err)
	}
}
