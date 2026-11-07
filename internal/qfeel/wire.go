// Package qfeel decodes the payloads a Shark Matrix (RV2320EDUS-class) robot
// vacuum puts on its local MQTT broker.
//
// The wire format is protobuf, but the schema is reverse-engineered: field
// numbers and value types come off the wire, field names are inferred from
// watching the robot, and anything still unmapped is labelled xN. Nothing here
// is vendor documentation. See the source notes at
// https://github.com/jcpsimmons/shark-vacuum-teardown
package qfeel

import (
	"errors"
	"fmt"
)

// ErrTruncated is returned when a length-delimited field runs past the buffer.
var ErrTruncated = errors.New("qfeel: truncated protobuf")

// Wire is a protobuf wire type.
type Wire uint8

// Protobuf wire types, as they appear in the low three bits of a tag.
const (
	WireVarint  Wire = 0
	WireFixed64 Wire = 1
	WireBytes   Wire = 2
	WireFixed32 Wire = 5
)

// Field is one decoded protobuf field.
type Field struct {
	Num   int
	Wire  Wire
	Uint  uint64
	Float float32
	Bytes []byte
	Msg   *Message
}

// Message is a decoded protobuf message: an ordered list of fields.
//
// The same field number may legally repeat (protobuf packs repeated fields that
// way), so callers look fields up rather than indexing.
type Message struct {
	Fields []Field
}

// Get returns the first field with number num, or nil.
func (m *Message) Get(num int) *Field {
	if m == nil {
		return nil
	}
	for i := range m.Fields {
		if m.Fields[i].Num == num {
			return &m.Fields[i]
		}
	}
	return nil
}

// All returns every field with number num, in wire order.
func (m *Message) All(num int) []*Field {
	if m == nil {
		return nil
	}
	var out []*Field
	for i := range m.Fields {
		if m.Fields[i].Num == num {
			out = append(out, &m.Fields[i])
		}
	}
	return out
}

// Numbers returns the field numbers present, in wire order, without duplicates.
func (m *Message) Numbers() []int {
	if m == nil {
		return nil
	}
	seen := make(map[int]bool, len(m.Fields))
	var out []int
	for _, f := range m.Fields {
		if !seen[f.Num] {
			seen[f.Num] = true
			out = append(out, f.Num)
		}
	}
	return out
}

func readVarint(b []byte) (uint64, int, error) {
	var v uint64
	for i := 0; i < len(b); i++ {
		if i == 10 {
			return 0, 0, errors.New("qfeel: varint too long")
		}
		v |= uint64(b[i]&0x7f) << (7 * i)
		if b[i]&0x80 == 0 {
			return v, i + 1, nil
		}
	}
	return 0, 0, ErrTruncated
}

// Parse walks a protobuf buffer into a Message tree, recursing into any
// length-delimited field that itself parses cleanly. Buffers that do not parse
// cleanly are kept as opaque bytes rather than rejected.
func Parse(b []byte) (*Message, error) {
	m := &Message{}
	for len(b) > 0 {
		tag, n, err := readVarint(b)
		if err != nil {
			return m, err
		}
		b = b[n:]
		f := Field{Num: int(tag >> 3), Wire: Wire(tag & 7)}
		if f.Num <= 0 {
			return m, fmt.Errorf("qfeel: bad field number %d", f.Num)
		}
		switch f.Wire {
		case WireVarint:
			v, n, err := readVarint(b)
			if err != nil {
				return m, err
			}
			f.Uint = v
			b = b[n:]
		case WireFixed64:
			if len(b) < 8 {
				return m, ErrTruncated
			}
			for i := 7; i >= 0; i-- {
				f.Uint = f.Uint<<8 | uint64(b[i])
			}
			b = b[8:]
		case WireFixed32:
			if len(b) < 4 {
				return m, ErrTruncated
			}
			f.Uint = uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
			f.Float = float32frombits(uint32(f.Uint))
			b = b[4:]
		case WireBytes:
			ln, n, err := readVarint(b)
			if err != nil {
				return m, err
			}
			b = b[n:]
			if uint64(len(b)) < ln {
				return m, ErrTruncated
			}
			f.Bytes = b[:ln]
			b = b[ln:]
			if sub, err := Parse(f.Bytes); err == nil && len(sub.Fields) > 0 {
				f.Msg = sub
			}
		default:
			// Groups (wire 3/4) are unused by this device.
			return m, fmt.Errorf("qfeel: unsupported wire type %d on field %d", f.Wire, f.Num)
		}
		m.Fields = append(m.Fields, f)
	}
	return m, nil
}

func float32frombits(v uint32) float32 {
	// Local copy keeps this file dependency-free and testable.
	const sign = 1 << 31
	if v&sign != 0 {
		return -float32frombits(v &^ sign)
	}
	if v == 0 {
		return 0
	}
	exp := int32((v>>23)&0xff) - 127
	mant := float64(v&0x7fffff) / float64(1<<23)
	m := 1 + mant
	if exp > 0 {
		for i := int32(0); i < exp; i++ {
			m *= 2
		}
	} else {
		for i := int32(0); i > exp; i-- {
			m /= 2
		}
	}
	return float32(m)
}
