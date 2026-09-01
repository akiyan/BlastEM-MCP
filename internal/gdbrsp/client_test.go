package gdbrsp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRegistersAndMemory(t *testing.T) {
	registerReply := strings.Repeat("00000000", 16) + "00002700" + "00000100"
	client := fakeGDBServer(t, map[string]string{
		"g":             registerReply,
		"m100,3":        "010203",
		"M200,3:AABBCC": "OK",
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	registers, err := client.Registers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if registers.SR != 0x2700 || registers.PC != 0x100 {
		t.Fatalf("registers = %+v", registers)
	}
	data, err := client.ReadMemory(ctx, 0x100, 3)
	if err != nil || string(data) != "\x01\x02\x03" {
		t.Fatalf("ReadMemory = %x, %v", data, err)
	}
	if err := client.WriteMemory(ctx, 0x200, []byte{0xAA, 0xBB, 0xCC}); err != nil {
		t.Fatal(err)
	}
}

func TestRunAndBreakpoints(t *testing.T) {
	client := fakeGDBServer(t, map[string]string{
		"?":         "S05",
		"s":         "S05",
		"Z0,1234,2": "OK",
		"z0,1234,2": "OK",
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.SetBreakpoint(ctx, 0x1234); err != nil {
		t.Fatal(err)
	}
	stop, err := client.Step(ctx)
	if err != nil || stop.Signal != 5 {
		t.Fatalf("Step = %+v, %v", stop, err)
	}
	if err := client.RemoveBreakpoint(ctx, 0x1234); err != nil {
		t.Fatal(err)
	}
}

func TestChecksumMismatch(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = readClientPacket(reader)
		_, _ = io.WriteString(conn, "+$S05#00")
	}()
	client := New(listener.Addr().String())
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.StopReason(ctx); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("got error %v", err)
	}
}

func TestCommandHonorsCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = readClientPacket(reader)
		_, _ = io.WriteString(conn, "+")
		_, _ = io.Copy(io.Discard, conn)
	}()
	client := New(listener.Addr().String())
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if _, err := client.StopReason(ctx); err == nil {
		t.Fatal("cancelled command unexpectedly succeeded")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancelled command took %s", elapsed)
	}
}

func TestRejectsMemoryAndBreakpointBounds(t *testing.T) {
	client := New("127.0.0.1:1")
	ctx := context.Background()
	if _, err := client.ReadMemory(ctx, 0, maxMemoryCall+1); err == nil {
		t.Fatal("ReadMemory accepted more than 64 KiB")
	}
	if err := client.WriteMemory(ctx, 0, make([]byte, maxMemoryCall+1)); err == nil {
		t.Fatal("WriteMemory accepted more than 64 KiB")
	}
	if err := client.SetBreakpoint(ctx, 0x1000000); err == nil {
		t.Fatal("SetBreakpoint accepted a 25-bit address")
	}
	if err := client.RemoveBreakpoint(ctx, 0x1000000); err == nil {
		t.Fatal("RemoveBreakpoint accepted a 25-bit address")
	}
}

func fakeGDBServer(t *testing.T, replies map[string]string) *Client {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for {
			payload, err := readClientPacket(reader)
			if err != nil {
				return
			}
			reply, ok := replies[payload]
			if !ok {
				reply = ""
			}
			_, _ = io.WriteString(conn, "+$"+reply+"#"+checksumHex(reply))
			_, _ = reader.ReadByte() // client acknowledgement
		}
	}()
	client := New(listener.Addr().String())
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func readClientPacket(reader *bufio.Reader) (string, error) {
	start, err := reader.ReadByte()
	if err != nil {
		return "", err
	}
	if start != '$' {
		return "", fmt.Errorf("unexpected packet start %q", start)
	}
	payload, err := reader.ReadString('#')
	if err != nil {
		return "", err
	}
	checksum := make([]byte, 2)
	if _, err := io.ReadFull(reader, checksum); err != nil {
		return "", err
	}
	return strings.TrimSuffix(payload, "#"), nil
}
