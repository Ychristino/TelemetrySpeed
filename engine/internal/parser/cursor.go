package parser

import (
	"encoding/binary"
	"math"
)

// cursor reads little-endian fields sequentially from a packet slice,
// advancing its position by the size of each read. Field calls mirror the
// order fields appear in the game's UDP spec, so a parse function reads top
// to bottom like the spec table instead of hand-computed byte ranges.
// Use Skip to pass over bytes the parser doesn't need.
type cursor struct {
	b   []byte
	pos int
}

func newCursor(b []byte) *cursor { return &cursor{b: b} }

// Skip advances past n bytes without reading them.
func (c *cursor) Skip(n int) { c.pos += n }

func (c *cursor) U8() uint8 {
	v := c.b[c.pos]
	c.pos++
	return v
}

func (c *cursor) I8() int8 { return int8(c.U8()) }

func (c *cursor) U16() uint16 {
	v := binary.LittleEndian.Uint16(c.b[c.pos : c.pos+2])
	c.pos += 2
	return v
}

func (c *cursor) I16() int16 { return int16(c.U16()) }

func (c *cursor) U32() uint32 {
	v := binary.LittleEndian.Uint32(c.b[c.pos : c.pos+4])
	c.pos += 4
	return v
}

func (c *cursor) F32() float32 { return math.Float32frombits(c.U32()) }

func (c *cursor) F32x4() [4]float32 {
	return [4]float32{c.F32(), c.F32(), c.F32(), c.F32()}
}

func (c *cursor) U8x4() [4]uint8 {
	return [4]uint8{c.U8(), c.U8(), c.U8(), c.U8()}
}

func (c *cursor) U16x4() [4]uint16 {
	return [4]uint16{c.U16(), c.U16(), c.U16(), c.U16()}
}

// Bytes reads the next n bytes as a slice.
func (c *cursor) Bytes(n int) []byte {
	v := c.b[c.pos : c.pos+n]
	c.pos += n
	return v
}

// NullTermStr reads an n-byte fixed field and returns the string up to the
// first NUL byte (or the full field if there is none).
func (c *cursor) NullTermStr(n int) string {
	return nullTermStr(c.Bytes(n))
}
