package control

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultTimeout = 5 * time.Second

var validButtons = map[string]struct{}{
	"up": {}, "down": {}, "left": {}, "right": {},
	"a": {}, "b": {}, "c": {}, "x": {}, "y": {}, "z": {},
	"start": {}, "mode": {},
}

var oppositeDirection = map[string]string{
	"left": "right", "right": "left", "up": "down", "down": "up",
}

type Client struct {
	path    string
	timeout time.Duration

	mu   sync.Mutex
	conn net.Conn
	held map[string]struct{}
}

func New(path string) *Client {
	return &Client{path: path, timeout: defaultTimeout, held: make(map[string]struct{})}
}

func (c *Client) SetTimeout(timeout time.Duration) { c.timeout = timeout }

func (c *Client) ButtonDown(pad int, button string) error {
	button, err := validateInput(pad, button)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if opposite, ok := oppositeDirection[button]; ok {
		if _, held := c.held[inputKey(pad, opposite)]; held {
			return fmt.Errorf("cannot hold %s while %s is held on pad %d", button, opposite, pad)
		}
	}
	if err := c.sendLocked(fmt.Sprintf("pad %d down %s", pad, button)); err != nil {
		return err
	}
	c.held[inputKey(pad, button)] = struct{}{}
	return nil
}

func (c *Client) ButtonUp(pad int, button string) error {
	button, err := validateInput(pad, button)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.sendLocked(fmt.Sprintf("pad %d up %s", pad, button)); err != nil {
		return err
	}
	delete(c.held, inputKey(pad, button))
	return nil
}

func (c *Client) ReleaseAll() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.held))
	for key := range c.held {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var errs []error
	for _, key := range keys {
		var pad int
		var button string
		if _, err := fmt.Sscanf(key, "%d:%s", &pad, &button); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := c.sendLocked(fmt.Sprintf("pad %d up %s", pad, button)); err != nil {
			errs = append(errs, err)
			continue
		}
		delete(c.held, key)
	}
	return errors.Join(errs...)
}

func (c *Client) Screenshot(path string) error {
	return c.requestArtifact("screenshot", path, func(data []byte) bool {
		return len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n"
	})
}

func (c *Client) VDPSnapshot(path string) error {
	return c.requestArtifact("vramdump", path, func(data []byte) bool {
		return len(data) >= 8 && string(data[:8]) == "KITVDMP1"
	})
}

// VideoState arms a screenshot and VDP snapshot back-to-back so BlastEM
// fulfills both at the next frame boundary instead of observing two frames of
// a palette animation or fade.
func (c *Client) VideoState(screenshotPath, snapshotPath string) error {
	for _, path := range []string{screenshotPath, snapshotPath} {
		if err := prepareArtifactPath(path); err != nil {
			return err
		}
	}
	c.mu.Lock()
	err := c.sendLocked("screenshot " + screenshotPath)
	if err == nil {
		err = c.sendLocked("vramdump " + snapshotPath)
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(c.timeout)
	if err := awaitArtifact(screenshotPath, "screenshot", deadline, func(data []byte) bool {
		return len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n"
	}); err != nil {
		return err
	}
	return awaitArtifact(snapshotPath, "vramdump", deadline, func(data []byte) bool {
		return len(data) >= 8 && string(data[:8]) == "KITVDMP1"
	})
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) requestArtifact(command, path string, valid func([]byte) bool) error {
	if err := prepareArtifactPath(path); err != nil {
		return err
	}
	c.mu.Lock()
	err := c.sendLocked(command + " " + path)
	c.mu.Unlock()
	if err != nil {
		return err
	}

	return awaitArtifact(path, command, time.Now().Add(c.timeout), valid)
}

func prepareArtifactPath(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("artifact path must be absolute")
	}
	if strings.ContainsAny(path, "\r\n") {
		return errors.New("artifact path contains a newline")
	}
	_ = os.Remove(path)
	return nil
}

func awaitArtifact(path, command string, deadline time.Time, valid func([]byte) bool) error {
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(path)
		if readErr == nil && valid(data) {
			return nil
		}
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return fmt.Errorf("read %s artifact: %w", command, readErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s artifact", command)
}

func (c *Client) sendLocked(command string) error {
	if strings.ContainsAny(command, "\r\n") {
		return errors.New("control command contains a newline")
	}
	if err := c.connectLocked(); err != nil {
		return err
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	w := bufio.NewWriter(c.conn)
	if _, err := w.WriteString(command + "\n"); err != nil {
		_ = c.conn.Close()
		c.conn = nil
		return fmt.Errorf("write BlastEM control command: %w", err)
	}
	if err := w.Flush(); err != nil {
		_ = c.conn.Close()
		c.conn = nil
		return fmt.Errorf("flush BlastEM control command: %w", err)
	}
	return nil
}

func (c *Client) connectLocked() error {
	if c.conn != nil {
		return nil
	}
	deadline := time.Now().Add(c.timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", c.path, 100*time.Millisecond)
		if err == nil {
			c.conn = conn
			return nil
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("connect BlastEM control socket %q: %w", c.path, lastErr)
}

func validateInput(pad int, button string) (string, error) {
	if pad < 1 || pad > 2 {
		return "", errors.New("pad must be 1 or 2")
	}
	button = strings.ToLower(button)
	if _, ok := validButtons[button]; !ok {
		return "", fmt.Errorf("unsupported button %q", button)
	}
	return button, nil
}

func inputKey(pad int, button string) string { return fmt.Sprintf("%d:%s", pad, button) }
