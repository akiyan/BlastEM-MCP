package gdbrsp

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	dialTimeout    = 5 * time.Second
	maxMemoryCall  = 64 * 1024
	readChunkSize  = 255 // BlastEM's GDB reply buffer is 512 bytes including framing.
	writeChunkSize = 1024
)

var ErrNotConnected = errors.New("GDB Remote client is not connected")

type Registers struct {
	D  [8]uint32 `json:"d"`
	A  [8]uint32 `json:"a"`
	SR uint32    `json:"sr"`
	PC uint32    `json:"pc"`
}

type Stop struct {
	Reply  string `json:"reply"`
	Signal uint8  `json:"signal"`
}

type Client struct {
	address string

	mu     sync.Mutex
	conn   net.Conn
	reader *bufio.Reader
}

func New(address string) *Client { return &Client{address: address} }

func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked(ctx)
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	c.reader = nil
	return err
}

func (c *Client) Registers(ctx context.Context) (Registers, error) {
	reply, err := c.command(ctx, "g")
	if err != nil {
		return Registers{}, err
	}
	if len(reply) != 18*8 {
		return Registers{}, fmt.Errorf("unexpected BlastEM register reply length %d", len(reply))
	}
	var registers Registers
	for i := range 8 {
		registers.D[i], err = parseHex32(reply[i*8 : (i+1)*8])
		if err != nil {
			return Registers{}, fmt.Errorf("parse D%d: %w", i, err)
		}
	}
	for i := range 8 {
		offset := (8 + i) * 8
		registers.A[i], err = parseHex32(reply[offset : offset+8])
		if err != nil {
			return Registers{}, fmt.Errorf("parse A%d: %w", i, err)
		}
	}
	registers.SR, err = parseHex32(reply[16*8 : 17*8])
	if err != nil {
		return Registers{}, fmt.Errorf("parse SR: %w", err)
	}
	registers.PC, err = parseHex32(reply[17*8 : 18*8])
	if err != nil {
		return Registers{}, fmt.Errorf("parse PC: %w", err)
	}
	return registers, nil
}

func (c *Client) ReadMemory(ctx context.Context, address uint32, size int) ([]byte, error) {
	if size < 0 || size > maxMemoryCall {
		return nil, fmt.Errorf("memory read size must be between 0 and %d bytes", maxMemoryCall)
	}
	result := make([]byte, 0, size)
	for len(result) < size {
		chunk := min(size-len(result), readChunkSize)
		reply, err := c.command(ctx, fmt.Sprintf("m%X,%X", address+uint32(len(result)), chunk))
		if err != nil {
			return nil, err
		}
		data, err := hex.DecodeString(reply)
		if err != nil {
			return nil, fmt.Errorf("decode memory reply: %w", err)
		}
		if len(data) != chunk {
			return nil, fmt.Errorf("memory reply has %d bytes, want %d", len(data), chunk)
		}
		result = append(result, data...)
	}
	return result, nil
}

func (c *Client) WriteMemory(ctx context.Context, address uint32, data []byte) error {
	if len(data) > maxMemoryCall {
		return fmt.Errorf("memory write size must not exceed %d bytes", maxMemoryCall)
	}
	for offset := 0; offset < len(data); {
		chunk := min(len(data)-offset, writeChunkSize)
		payload := fmt.Sprintf("M%X,%X:%s", address+uint32(offset), chunk, strings.ToUpper(hex.EncodeToString(data[offset:offset+chunk])))
		reply, err := c.command(ctx, payload)
		if err != nil {
			return err
		}
		if reply != "OK" {
			return fmt.Errorf("BlastEM rejected memory write: %s", reply)
		}
		offset += chunk
	}
	return nil
}

func (c *Client) SetBreakpoint(ctx context.Context, address uint32) error {
	if address > 0xFFFFFF {
		return errors.New("breakpoint address must fit in 24 bits")
	}
	return c.expectOK(ctx, fmt.Sprintf("Z0,%X,2", address))
}

func (c *Client) RemoveBreakpoint(ctx context.Context, address uint32) error {
	if address > 0xFFFFFF {
		return errors.New("breakpoint address must fit in 24 bits")
	}
	return c.expectOK(ctx, fmt.Sprintf("z0,%X,2", address))
}

func (c *Client) Step(ctx context.Context) (Stop, error)     { return c.run(ctx, "s") }
func (c *Client) Continue(ctx context.Context) (Stop, error) { return c.run(ctx, "c") }

func (c *Client) StopReason(ctx context.Context) (Stop, error) {
	reply, err := c.command(ctx, "?")
	if err != nil {
		return Stop{}, err
	}
	return parseStop(reply)
}

func (c *Client) run(ctx context.Context, payload string) (Stop, error) {
	reply, err := c.command(ctx, payload)
	if err != nil {
		return Stop{}, err
	}
	return parseStop(reply)
}

func (c *Client) expectOK(ctx context.Context, payload string) error {
	reply, err := c.command(ctx, payload)
	if err != nil {
		return err
	}
	if reply != "OK" {
		return fmt.Errorf("BlastEM GDB Remote replied %q", reply)
	}
	return nil
}

func (c *Client) command(ctx context.Context, payload string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.connectLocked(ctx); err != nil {
		return "", err
	}
	if err := applyDeadline(c.conn, ctx); err != nil {
		return "", err
	}
	stopWatching := watchCancellation(c.conn, ctx)
	defer stopWatching()
	packet := "$" + payload + "#" + checksumHex(payload)
	if _, err := io.WriteString(c.conn, packet); err != nil {
		c.disconnectLocked()
		return "", fmt.Errorf("write GDB packet: %w", err)
	}
	ack, err := c.reader.ReadByte()
	if err != nil {
		c.disconnectLocked()
		return "", fmt.Errorf("read GDB acknowledgement: %w", err)
	}
	if ack != '+' {
		return "", fmt.Errorf("unexpected GDB acknowledgement %q", ack)
	}
	reply, err := c.readPacketLocked()
	if err != nil {
		c.disconnectLocked()
		return "", err
	}
	if _, err := c.conn.Write([]byte{'+'}); err != nil {
		c.disconnectLocked()
		return "", fmt.Errorf("acknowledge GDB reply: %w", err)
	}
	return reply, nil
}

func (c *Client) connectLocked(ctx context.Context) error {
	if c.conn != nil {
		return nil
	}
	deadline := time.Now().Add(dialTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	var lastErr error
	for time.Now().Before(deadline) {
		dialer := net.Dialer{Timeout: 100 * time.Millisecond}
		conn, err := dialer.DialContext(ctx, "tcp", c.address)
		if err == nil {
			c.conn = conn
			c.reader = bufio.NewReader(conn)
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return fmt.Errorf("connect GDB Remote at %s: %w", c.address, lastErr)
}

func (c *Client) readPacketLocked() (string, error) {
	for {
		start, err := c.reader.ReadByte()
		if err != nil {
			return "", fmt.Errorf("read GDB packet start: %w", err)
		}
		if start == '$' {
			break
		}
	}
	payload, err := c.reader.ReadString('#')
	if err != nil {
		return "", fmt.Errorf("read GDB packet payload: %w", err)
	}
	payload = strings.TrimSuffix(payload, "#")
	checksum := make([]byte, 2)
	if _, err := io.ReadFull(c.reader, checksum); err != nil {
		return "", fmt.Errorf("read GDB packet checksum: %w", err)
	}
	if !strings.EqualFold(string(checksum), checksumHex(payload)) {
		return "", fmt.Errorf("GDB checksum mismatch: got %s, want %s", checksum, checksumHex(payload))
	}
	return payload, nil
}

func (c *Client) disconnectLocked() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.reader = nil
}

func parseStop(reply string) (Stop, error) {
	if len(reply) < 3 || (reply[0] != 'S' && reply[0] != 'T') {
		return Stop{}, fmt.Errorf("unexpected GDB stop reply %q", reply)
	}
	signal, err := strconv.ParseUint(reply[1:3], 16, 8)
	if err != nil {
		return Stop{}, fmt.Errorf("parse GDB stop signal: %w", err)
	}
	return Stop{Reply: reply, Signal: uint8(signal)}, nil
}

func parseHex32(value string) (uint32, error) {
	parsed, err := strconv.ParseUint(value, 16, 32)
	return uint32(parsed), err
}

func checksumHex(payload string) string {
	var sum byte
	for i := range len(payload) {
		sum += payload[i]
	}
	return fmt.Sprintf("%02x", sum)
}

func applyDeadline(conn net.Conn, ctx context.Context) error {
	if deadline, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(deadline)
	}
	return conn.SetDeadline(time.Time{})
}

func watchCancellation(conn net.Conn, ctx context.Context) func() {
	if ctx.Done() == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-done:
		}
	}()
	return func() { close(done) }
}
