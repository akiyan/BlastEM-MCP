package vdp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
)

const (
	Magic    = "KITVDMP1"
	TileSize = 32
)

type Snapshot struct {
	VRAM  []byte
	CRAM  []uint16
	VSRAM []uint16
	Regs  []byte
}

type SnapshotInfo struct {
	VRAMBytes  int `json:"vram_bytes"`
	CRAMWords  int `json:"cram_words"`
	VSRAMWords int `json:"vsram_words"`
	VDPRegs    int `json:"vdp_registers"`
}

func (s Snapshot) Info() SnapshotInfo {
	return SnapshotInfo{VRAMBytes: len(s.VRAM), CRAMWords: len(s.CRAM), VSRAMWords: len(s.VSRAM), VDPRegs: len(s.Regs)}
}

func Parse(data []byte) (Snapshot, error) {
	if len(data) < 24 || string(data[:8]) != Magic {
		return Snapshot{}, errors.New("invalid KITVDMP1 header")
	}
	vramSize := uint64(binary.LittleEndian.Uint32(data[8:12]))
	cramCount := uint64(binary.LittleEndian.Uint32(data[12:16]))
	vsramCount := uint64(binary.LittleEndian.Uint32(data[16:20]))
	regCount := uint64(binary.LittleEndian.Uint32(data[20:24]))
	total := uint64(24) + vramSize + cramCount*2 + vsramCount*2 + regCount
	if total != uint64(len(data)) {
		return Snapshot{}, fmt.Errorf("KITVDMP1 size mismatch: header describes %d bytes, got %d", total, len(data))
	}
	if vramSize == 0 || vramSize%TileSize != 0 || cramCount < 64 {
		return Snapshot{}, fmt.Errorf("unsupported KITVDMP1 dimensions: VRAM=%d CRAM=%d", vramSize, cramCount)
	}

	offset := uint64(24)
	snapshot := Snapshot{VRAM: append([]byte(nil), data[offset:offset+vramSize]...)}
	offset += vramSize
	snapshot.CRAM = make([]uint16, cramCount)
	for i := range snapshot.CRAM {
		snapshot.CRAM[i] = binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
	}
	snapshot.VSRAM = make([]uint16, vsramCount)
	for i := range snapshot.VSRAM {
		snapshot.VSRAM[i] = binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
	}
	snapshot.Regs = append([]byte(nil), data[offset:offset+regCount]...)
	return snapshot, nil
}

func Read(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read VDP snapshot: %w", err)
	}
	return Parse(data)
}

type TilesheetInfo struct {
	TileCount    int `json:"tile_count"`
	PaletteCount int `json:"palette_count"`
	Columns      int `json:"columns"`
	Rows         int `json:"rows"`
	Scale        int `json:"scale"`
	Width        int `json:"width"`
	Height       int `json:"height"`
}

// RenderTilesheet renders every VRAM tile once with each of the four Mega Drive
// CRAM palettes. Tiles are 8x8 pixels and use packed high-nibble-first 4bpp data.
func (s Snapshot) RenderTilesheet(w io.Writer, scale int) (TilesheetInfo, error) {
	if scale < 1 || scale > 8 {
		return TilesheetInfo{}, errors.New("scale must be from 1 through 8")
	}
	tileCount := len(s.VRAM) / TileSize
	if tileCount == 0 || len(s.CRAM) < 64 {
		return TilesheetInfo{}, errors.New("snapshot does not contain renderable VRAM and CRAM")
	}
	const columns = 32
	rows := (tileCount + columns - 1) / columns
	panelWidth := columns * 8 * scale
	width := panelWidth * 4
	height := rows * 8 * scale
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for palette := 0; palette < 4; palette++ {
		for tile := 0; tile < tileCount; tile++ {
			tileX := palette*panelWidth + (tile%columns)*8*scale
			tileY := (tile / columns) * 8 * scale
			base := tile * TileSize
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					packed := s.VRAM[base+y*4+x/2]
					index := packed & 0x0f
					if x%2 == 0 {
						index = packed >> 4
					}
					c := cramColor(s.CRAM[palette*16+int(index)])
					for sy := 0; sy < scale; sy++ {
						for sx := 0; sx < scale; sx++ {
							img.SetRGBA(tileX+x*scale+sx, tileY+y*scale+sy, c)
						}
					}
				}
			}
		}
	}
	if err := png.Encode(w, img); err != nil {
		return TilesheetInfo{}, fmt.Errorf("encode VRAM tilesheet: %w", err)
	}
	return TilesheetInfo{TileCount: tileCount, PaletteCount: 4, Columns: columns, Rows: rows, Scale: scale, Width: width, Height: height}, nil
}

func (s Snapshot) WriteTilesheet(path string, scale int) (TilesheetInfo, error) {
	f, err := os.Create(path)
	if err != nil {
		return TilesheetInfo{}, fmt.Errorf("create VRAM tilesheet: %w", err)
	}
	info, renderErr := s.RenderTilesheet(f, scale)
	closeErr := f.Close()
	if renderErr != nil {
		return TilesheetInfo{}, renderErr
	}
	if closeErr != nil {
		return TilesheetInfo{}, fmt.Errorf("close VRAM tilesheet: %w", closeErr)
	}
	return info, nil
}

func cramColor(raw uint16) color.RGBA {
	r := uint8((raw >> 1) & 7)
	g := uint8((raw >> 5) & 7)
	b := uint8((raw >> 9) & 7)
	// Expand 3-bit channels to the full PNG byte range.
	return color.RGBA{R: r * 255 / 7, G: g * 255 / 7, B: b * 255 / 7, A: 255}
}
