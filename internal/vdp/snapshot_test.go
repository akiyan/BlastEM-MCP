package vdp

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestParseAndRenderTilesheet(t *testing.T) {
	data := make([]byte, 24+32+64*2+2*2+4)
	copy(data, Magic)
	binary.LittleEndian.PutUint32(data[8:12], 32)
	binary.LittleEndian.PutUint32(data[12:16], 64)
	binary.LittleEndian.PutUint32(data[16:20], 2)
	binary.LittleEndian.PutUint32(data[20:24], 4)
	for i := 24; i < 56; i++ {
		data[i] = 0x12
	}
	cram := 56
	binary.LittleEndian.PutUint16(data[cram+2:cram+4], 0x000e)
	binary.LittleEndian.PutUint16(data[cram+4:cram+6], 0x00e0)

	snapshot, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	info, err := snapshot.RenderTilesheet(&output, 2)
	if err != nil {
		t.Fatal(err)
	}
	if info.TileCount != 1 || info.Width != 2048 || info.Height != 16 {
		t.Fatalf("unexpected tilesheet info: %+v", info)
	}
	img, err := png.Decode(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.At(0, 0); got == nil {
		t.Fatal("missing rendered pixel")
	}
}

func TestParseRejectsTruncatedSnapshot(t *testing.T) {
	data := make([]byte, 24)
	copy(data, Magic)
	binary.LittleEndian.PutUint32(data[8:12], 32)
	binary.LittleEndian.PutUint32(data[12:16], 64)
	if _, err := Parse(data); err == nil {
		t.Fatal("Parse accepted a truncated snapshot")
	}
}
