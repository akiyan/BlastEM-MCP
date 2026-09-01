package mcpserver

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/akiyan/BlastEM-MCP/internal/artifact"
	"github.com/akiyan/BlastEM-MCP/internal/gdbrsp"
	"github.com/akiyan/BlastEM-MCP/internal/session"
	"github.com/akiyan/BlastEM-MCP/internal/vdp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type App struct {
	server  *mcp.Server
	session *session.Manager
	serial  atomic.Uint64
}

func New(version string) *App {
	a := &App{session: session.NewManager()}
	a.server = mcp.NewServer(&mcp.Implementation{Name: "blastem-mcp", Version: version}, nil)
	a.registerTools()
	return a
}

func (a *App) Server() *mcp.Server { return a.server }
func (a *App) Close() error        { return a.session.Close() }

type emptyInput struct{}

type startInput struct {
	BinaryPath string `json:"binary_path" jsonschema:"absolute or working-directory-relative path to the BlastEM executable"`
	ROMPath    string `json:"rom_path" jsonschema:"absolute or working-directory-relative path to a local ROM"`
	Debug      bool   `json:"debug,omitempty" jsonschema:"start the localhost GDB Remote stub"`
}

type statusOutput struct {
	Session session.Status `json:"session"`
}

type inputArgs struct {
	Pad    int    `json:"pad" jsonschema:"controller port number, 1 or 2"`
	Button string `json:"button" jsonschema:"button: up, down, left, right, a, b, c, x, y, z, start, or mode"`
}

type operationOutput struct {
	OK bool `json:"ok"`
}

type artifactOutput struct {
	Artifact artifact.Info `json:"artifact"`
}

type vdpSnapshotOutput struct {
	Artifact artifact.Info    `json:"artifact"`
	Snapshot vdp.SnapshotInfo `json:"snapshot"`
}

type vramTilesInput struct {
	Scale int `json:"scale,omitempty" jsonschema:"pixel scale from 1 through 8; defaults to 2"`
}

type vramTilesOutput struct {
	ReferenceFrame artifact.Info       `json:"reference_frame"`
	Palettes       []vramPaletteOutput `json:"palettes"`
}

type vramPaletteOutput struct {
	Palette  int               `json:"palette"`
	Artifact artifact.Info     `json:"artifact"`
	Tiles    vdp.TilesheetInfo `json:"tiles"`
}

type memoryReadInput struct {
	Address uint32 `json:"address" jsonschema:"68000 address"`
	Size    int    `json:"size" jsonschema:"number of bytes from 0 through 65536"`
}

type memoryReadOutput struct {
	Address uint32 `json:"address"`
	Size    int    `json:"size"`
	Hex     string `json:"hex" jsonschema:"uppercase hexadecimal bytes"`
}

type memoryWriteInput struct {
	Address uint32 `json:"address" jsonschema:"68000 address"`
	Hex     string `json:"hex" jsonschema:"hexadecimal bytes, at most 65536 decoded bytes"`
}

type breakpointInput struct {
	Address uint32 `json:"address" jsonschema:"24-bit 68000 breakpoint address"`
}

type registersOutput struct {
	Registers gdbrsp.Registers `json:"registers"`
}

type stopOutput struct {
	Stop gdbrsp.Stop `json:"stop"`
}

func (a *App) registerTools() {
	mcp.AddTool(a.server, &mcp.Tool{Name: "blastem_start", Description: "Start one managed BlastEM session with a local ROM."}, a.start)
	mcp.AddTool(a.server, &mcp.Tool{Name: "blastem_status", Description: "Inspect the managed BlastEM process and endpoints."}, a.status)
	mcp.AddTool(a.server, &mcp.Tool{Name: "blastem_stop", Description: "Release held input and stop the managed BlastEM process."}, a.stop)
	mcp.AddTool(a.server, &mcp.Tool{Name: "button_down", Description: "Hold a Mega Drive controller button through BlastEM's control socket."}, a.buttonDown)
	mcp.AddTool(a.server, &mcp.Tool{Name: "button_up", Description: "Release a Mega Drive controller button through BlastEM's control socket."}, a.buttonUp)
	mcp.AddTool(a.server, &mcp.Tool{Name: "release_all_buttons", Description: "Release every controller button held through this MCP session."}, a.releaseAll)
	mcp.AddTool(a.server, &mcp.Tool{Name: "screenshot", Description: "Capture the next BlastEM-rendered frame as a PNG image."}, a.screenshot)
	mcp.AddTool(a.server, &mcp.Tool{Name: "vdp_snapshot", Description: "Capture a KITVDMP1 VRAM/CRAM/VSRAM/VDP-register snapshot."}, a.vdpSnapshot)
	mcp.AddTool(a.server, &mcp.Tool{Name: "vram_tiles", Description: "Capture VRAM and return four PNG tilesheets, one for each live CRAM palette in palette 0 through 3 order."}, a.vramTiles)
	mcp.AddTool(a.server, &mcp.Tool{Name: "cpu_registers", Description: "Read all 68000 data/address registers, status register, and PC while stopped."}, a.cpuRegisters)
	mcp.AddTool(a.server, &mcp.Tool{Name: "memory_read", Description: "Read up to 64 KiB from the 68000 address space while stopped."}, a.memoryRead)
	mcp.AddTool(a.server, &mcp.Tool{Name: "memory_write", Description: "Write hexadecimal bytes into the 68000 address space while stopped."}, a.memoryWrite)
	mcp.AddTool(a.server, &mcp.Tool{Name: "breakpoint_set", Description: "Set a 68000 software breakpoint at a 24-bit address."}, a.breakpointSet)
	mcp.AddTool(a.server, &mcp.Tool{Name: "breakpoint_remove", Description: "Remove a 68000 software breakpoint."}, a.breakpointRemove)
	mcp.AddTool(a.server, &mcp.Tool{Name: "cpu_step", Description: "Execute one 68000 instruction and wait for the stop reply."}, a.cpuStep)
	mcp.AddTool(a.server, &mcp.Tool{Name: "cpu_continue", Description: "Continue 68000 execution until a previously configured breakpoint stops it."}, a.cpuContinue)
}

func (a *App) start(_ context.Context, _ *mcp.CallToolRequest, in startInput) (*mcp.CallToolResult, statusOutput, error) {
	status, err := a.session.Start(session.StartConfig{BinaryPath: in.BinaryPath, ROMPath: in.ROMPath, Debug: in.Debug})
	return nil, statusOutput{Session: status}, err
}

func (a *App) status(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, statusOutput, error) {
	return nil, statusOutput{Session: a.session.Status()}, nil
}

func (a *App) stop(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, operationOutput, error) {
	err := a.session.Stop()
	return nil, operationOutput{OK: err == nil}, err
}

func (a *App) buttonDown(_ context.Context, _ *mcp.CallToolRequest, in inputArgs) (*mcp.CallToolResult, operationOutput, error) {
	client, _, err := a.session.Control()
	if err == nil {
		err = client.ButtonDown(in.Pad, in.Button)
	}
	return nil, operationOutput{OK: err == nil}, err
}

func (a *App) buttonUp(_ context.Context, _ *mcp.CallToolRequest, in inputArgs) (*mcp.CallToolResult, operationOutput, error) {
	client, _, err := a.session.Control()
	if err == nil {
		err = client.ButtonUp(in.Pad, in.Button)
	}
	return nil, operationOutput{OK: err == nil}, err
}

func (a *App) releaseAll(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, operationOutput, error) {
	client, _, err := a.session.Control()
	if err == nil {
		err = client.ReleaseAll()
	}
	return nil, operationOutput{OK: err == nil}, err
}

func (a *App) screenshot(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, artifactOutput, error) {
	client, runtimeDir, err := a.session.Control()
	if err != nil {
		return nil, artifactOutput{}, err
	}
	path := filepath.Join(runtimeDir, fmt.Sprintf("screenshot-%06d.png", a.serial.Add(1)))
	if err := client.Screenshot(path); err != nil {
		return nil, artifactOutput{}, err
	}
	info, err := artifact.Inspect(path)
	if err != nil {
		return nil, artifactOutput{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, artifactOutput{}, err
	}
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{
		Data: data, MIMEType: "image/png",
	}}}
	return result, artifactOutput{Artifact: info}, nil
}

func (a *App) vdpSnapshot(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, vdpSnapshotOutput, error) {
	client, runtimeDir, err := a.session.Control()
	if err != nil {
		return nil, vdpSnapshotOutput{}, err
	}
	path := filepath.Join(runtimeDir, fmt.Sprintf("vdp-%06d.kitvdmp", a.serial.Add(1)))
	if err := client.VDPSnapshot(path); err != nil {
		return nil, vdpSnapshotOutput{}, err
	}
	info, err := artifact.Inspect(path)
	if err != nil {
		return nil, vdpSnapshotOutput{}, err
	}
	snapshot, err := vdp.Read(path)
	return nil, vdpSnapshotOutput{Artifact: info, Snapshot: snapshot.Info()}, err
}

func (a *App) vramTiles(_ context.Context, _ *mcp.CallToolRequest, in vramTilesInput) (*mcp.CallToolResult, vramTilesOutput, error) {
	client, runtimeDir, err := a.session.Control()
	if err != nil {
		return nil, vramTilesOutput{}, err
	}
	scale := in.Scale
	if scale == 0 {
		scale = 2
	}
	serial := a.serial.Add(1)
	snapshotPath := filepath.Join(runtimeDir, fmt.Sprintf("vdp-%06d.kitvdmp", serial))
	referencePath := filepath.Join(runtimeDir, fmt.Sprintf("vram-reference-%06d.png", serial))
	if err := client.VideoState(referencePath, snapshotPath); err != nil {
		return nil, vramTilesOutput{}, err
	}
	referenceInfo, err := artifact.Inspect(referencePath)
	if err != nil {
		return nil, vramTilesOutput{}, err
	}
	snapshot, err := vdp.Read(snapshotPath)
	if err != nil {
		return nil, vramTilesOutput{}, err
	}
	output := vramTilesOutput{ReferenceFrame: referenceInfo, Palettes: make([]vramPaletteOutput, 0, 4)}
	content := make([]mcp.Content, 0, 4)
	for palette := 0; palette < 4; palette++ {
		imagePath := filepath.Join(runtimeDir, fmt.Sprintf("vram-palette-%d-%06d.png", palette, serial))
		tiles, err := snapshot.WritePaletteTilesheet(imagePath, scale, palette)
		if err != nil {
			return nil, vramTilesOutput{}, err
		}
		info, err := artifact.Inspect(imagePath)
		if err != nil {
			return nil, vramTilesOutput{}, err
		}
		data, err := os.ReadFile(imagePath)
		if err != nil {
			return nil, vramTilesOutput{}, err
		}
		output.Palettes = append(output.Palettes, vramPaletteOutput{Palette: palette, Artifact: info, Tiles: tiles})
		content = append(content, &mcp.ImageContent{Data: data, MIMEType: "image/png"})
	}
	return &mcp.CallToolResult{Content: content}, output, nil
}

func (a *App) cpuRegisters(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, registersOutput, error) {
	client, err := a.session.GDB()
	if err != nil {
		return nil, registersOutput{}, err
	}
	registers, err := client.Registers(ctx)
	return nil, registersOutput{Registers: registers}, err
}

func (a *App) memoryRead(ctx context.Context, _ *mcp.CallToolRequest, in memoryReadInput) (*mcp.CallToolResult, memoryReadOutput, error) {
	client, err := a.session.GDB()
	if err != nil {
		return nil, memoryReadOutput{}, err
	}
	data, err := client.ReadMemory(ctx, in.Address, in.Size)
	return nil, memoryReadOutput{Address: in.Address, Size: len(data), Hex: strings.ToUpper(hex.EncodeToString(data))}, err
}

func (a *App) memoryWrite(ctx context.Context, _ *mcp.CallToolRequest, in memoryWriteInput) (*mcp.CallToolResult, operationOutput, error) {
	client, err := a.session.GDB()
	if err != nil {
		return nil, operationOutput{}, err
	}
	data, err := hex.DecodeString(in.Hex)
	if err == nil {
		err = client.WriteMemory(ctx, in.Address, data)
	}
	return nil, operationOutput{OK: err == nil}, err
}

func (a *App) breakpointSet(ctx context.Context, _ *mcp.CallToolRequest, in breakpointInput) (*mcp.CallToolResult, operationOutput, error) {
	client, err := a.session.GDB()
	if err == nil {
		err = client.SetBreakpoint(ctx, in.Address)
	}
	return nil, operationOutput{OK: err == nil}, err
}

func (a *App) breakpointRemove(ctx context.Context, _ *mcp.CallToolRequest, in breakpointInput) (*mcp.CallToolResult, operationOutput, error) {
	client, err := a.session.GDB()
	if err == nil {
		err = client.RemoveBreakpoint(ctx, in.Address)
	}
	return nil, operationOutput{OK: err == nil}, err
}

func (a *App) cpuStep(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, stopOutput, error) {
	client, err := a.session.GDB()
	if err != nil {
		return nil, stopOutput{}, err
	}
	stop, err := client.Step(ctx)
	return nil, stopOutput{Stop: stop}, err
}

func (a *App) cpuContinue(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, stopOutput, error) {
	client, err := a.session.GDB()
	if err != nil {
		return nil, stopOutput{}, err
	}
	stop, err := client.Continue(ctx)
	return nil, stopOutput{Stop: stop}, err
}
