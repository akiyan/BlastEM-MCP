package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/akiyan/BlastEM-MCP/internal/control"
	"github.com/akiyan/BlastEM-MCP/internal/gdbrsp"
)

var (
	ErrAlreadyRunning = errors.New("a BlastEM session is already running")
	ErrNotRunning     = errors.New("no BlastEM session is running")
)

type StartConfig struct {
	BinaryPath string
	ROMPath    string
	Debug      bool
}

type Status struct {
	Running     bool   `json:"running" jsonschema:"whether the managed BlastEM process is running"`
	PID         int    `json:"pid,omitempty" jsonschema:"operating-system process ID"`
	ROMPath     string `json:"rom_path,omitempty" jsonschema:"absolute path of the loaded ROM"`
	RuntimeDir  string `json:"runtime_dir,omitempty" jsonschema:"private directory for sockets, logs, and artifacts"`
	ControlSock string `json:"control_socket,omitempty" jsonschema:"BlastEM Unix control socket path"`
	GDBAddress  string `json:"gdb_address,omitempty" jsonschema:"localhost GDB Remote address when debug mode is enabled"`
	ExitError   string `json:"exit_error,omitempty" jsonschema:"last unexpected process exit error"`
}

type processState struct {
	cmd         *exec.Cmd
	romPath     string
	runtimeDir  string
	controlSock string
	gdbAddress  string
	control     *control.Client
	gdb         *gdbrsp.Client
	done        chan struct{}
	exitErr     error
	stopping    bool
	stdout      *os.File
	stderr      *os.File
}

type Manager struct {
	mu   sync.Mutex
	proc *processState
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Start(cfg StartConfig) (Status, error) {
	binary, err := validateFile("BlastEM binary", cfg.BinaryPath, true)
	if err != nil {
		return Status{}, err
	}
	rom, err := validateFile("ROM", cfg.ROMPath, false)
	if err != nil {
		return Status{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proc != nil && processRunning(m.proc) {
		return Status{}, ErrAlreadyRunning
	}
	if m.proc != nil {
		m.cleanupLocked(m.proc)
		m.proc = nil
	}

	runtimeDir, err := os.MkdirTemp("", "blastem-mcp-")
	if err != nil {
		return Status{}, fmt.Errorf("create session runtime directory: %w", err)
	}
	controlSock := filepath.Join(runtimeDir, "control.sock")
	stdout, err := os.Create(filepath.Join(runtimeDir, "blastem.stdout.log"))
	if err != nil {
		_ = os.RemoveAll(runtimeDir)
		return Status{}, fmt.Errorf("create BlastEM stdout log: %w", err)
	}
	stderr, err := os.Create(filepath.Join(runtimeDir, "blastem.stderr.log"))
	if err != nil {
		_ = stdout.Close()
		_ = os.RemoveAll(runtimeDir)
		return Status{}, fmt.Errorf("create BlastEM stderr log: %w", err)
	}

	args := []string{rom}
	env := append(os.Environ(), "BLASTEM_CTRL_SOCK="+controlSock, "BLASTEM_NO_GUI=1")
	gdbAddress := ""
	if cfg.Debug {
		port, allocErr := freeLoopbackPort()
		if allocErr != nil {
			_ = stdout.Close()
			_ = stderr.Close()
			_ = os.RemoveAll(runtimeDir)
			return Status{}, allocErr
		}
		gdbAddress = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		env = append(env, "BLASTEM_GDB_PORT="+strconv.Itoa(port))
		args = append(args, "-D")
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = filepath.Dir(binary)
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = os.RemoveAll(runtimeDir)
		return Status{}, fmt.Errorf("start BlastEM: %w", err)
	}
	var debugger *gdbrsp.Client
	if cfg.Debug {
		debugger = gdbrsp.New(gdbAddress)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := debugger.Connect(ctx)
		cancel()
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			_ = stdout.Close()
			_ = stderr.Close()
			_ = os.RemoveAll(runtimeDir)
			return Status{}, fmt.Errorf("connect BlastEM GDB Remote: %w", err)
		}
	}

	p := &processState{
		cmd: cmd, romPath: rom, runtimeDir: runtimeDir, controlSock: controlSock,
		gdbAddress: gdbAddress, control: control.New(controlSock), done: make(chan struct{}),
		gdb: debugger, stdout: stdout, stderr: stderr,
	}
	m.proc = p
	go m.wait(p)

	return statusOf(p), nil
}

func (m *Manager) GDB() (*gdbrsp.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proc == nil || !processRunning(m.proc) {
		return nil, ErrNotRunning
	}
	if m.proc.gdb == nil {
		return nil, errors.New("BlastEM session was started without debug mode")
	}
	return m.proc.gdb, nil
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proc == nil {
		return Status{}
	}
	return statusOf(m.proc)
}

func (m *Manager) Control() (*control.Client, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proc == nil || !processRunning(m.proc) {
		return nil, "", ErrNotRunning
	}
	return m.proc.control, m.proc.runtimeDir, nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	p := m.proc
	if p == nil {
		m.mu.Unlock()
		return ErrNotRunning
	}
	if !processRunning(p) {
		m.cleanupLocked(p)
		m.proc = nil
		m.mu.Unlock()
		return nil
	}
	p.stopping = true
	_ = p.control.ReleaseAll()
	err := p.cmd.Process.Signal(os.Interrupt)
	m.mu.Unlock()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("interrupt BlastEM: %w", err)
	}

	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
		if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill BlastEM after shutdown timeout: %w", err)
		}
		<-p.done
	}

	m.mu.Lock()
	if m.proc == p {
		m.cleanupLocked(p)
		m.proc = nil
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) Close() error {
	err := m.Stop()
	if errors.Is(err, ErrNotRunning) {
		return nil
	}
	return err
}

func (m *Manager) wait(p *processState) {
	err := p.cmd.Wait()
	m.mu.Lock()
	p.exitErr = err
	_ = p.stdout.Close()
	_ = p.stderr.Close()
	close(p.done)
	m.mu.Unlock()
}

func (m *Manager) cleanupLocked(p *processState) {
	_ = p.control.Close()
	if p.gdb != nil {
		_ = p.gdb.Close()
	}
	_ = os.RemoveAll(p.runtimeDir)
}

func statusOf(p *processState) Status {
	status := Status{
		Running: processRunning(p), PID: p.cmd.Process.Pid, ROMPath: p.romPath,
		RuntimeDir: p.runtimeDir, ControlSock: p.controlSock, GDBAddress: p.gdbAddress,
	}
	if p.exitErr != nil && !p.stopping {
		status.ExitError = p.exitErr.Error()
	}
	return status
}

func processRunning(p *processState) bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func validateFile(label, path string, executable bool) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%s path is required", label)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s path: %w", label, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("inspect %s %q: %w", label, abs, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s %q is not a regular file", label, abs)
	}
	if executable && info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%s %q is not executable", label, abs)
	}
	return abs, nil
}

func freeLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate GDB loopback port: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
