package secondfloor

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type BnkType byte

const (
	BnkVarint BnkType = 0
	BnkBytes  BnkType = 1
	BnkStruct BnkType = 4
	BnkList   BnkType = 5
)

const bnkStructEnd = 0x78

type BnkValue struct {
	Type   BnkType
	Uint   uint64
	Bytes  []byte
	Fields []BnkField
	Items  []BnkValue
}

type BnkField struct {
	ID    int
	Tag   byte
	Value BnkValue
}

func ParseBnk(data []byte) (BnkValue, error) {
	p := &bnkParser{data: data}
	fields, err := p.fields(false)
	if err != nil {
		return BnkValue{}, fmt.Errorf("bnk offset %d: %w", p.pos, err)
	}
	return BnkValue{Type: BnkStruct, Fields: fields}, nil
}

type bnkParser struct {
	data []byte
	pos  int
}

func (p *bnkParser) byte() (byte, error) {
	if p.pos >= len(p.data) {
		return 0, errors.New("unexpected end of data")
	}
	b := p.data[p.pos]
	p.pos++
	return b, nil
}

func (p *bnkParser) varint() (uint64, error) {
	v, n := binary.Uvarint(p.data[p.pos:])
	if n <= 0 {
		return 0, errors.New("bad varint")
	}
	p.pos += n
	return v, nil
}

func (p *bnkParser) fields(terminated bool) ([]BnkField, error) {
	var fields []BnkField
	id := 0
	for {
		if !terminated && p.pos == len(p.data) {
			return fields, nil
		}
		tag, err := p.byte()
		if err != nil {
			return nil, err
		}
		if tag == bnkStructEnd {
			if !terminated {
				return nil, errors.New("unexpected struct end")
			}
			return fields, nil
		}
		v, err := p.value(BnkType(tag & 7))
		if err != nil {
			return nil, err
		}
		id += int(tag >> 3)
		fields = append(fields, BnkField{ID: id, Tag: tag, Value: v})
	}
}

func (p *bnkParser) value(t BnkType) (BnkValue, error) {
	v := BnkValue{Type: t}
	switch t {
	case BnkVarint:
		n, err := p.varint()
		if err != nil {
			return v, err
		}
		v.Uint = n
	case BnkBytes:
		n, err := p.varint()
		if err != nil {
			return v, err
		}
		if n > uint64(len(p.data)-p.pos) {
			return v, errors.New("bytes length out of range")
		}
		v.Bytes = p.data[p.pos : p.pos+int(n)]
		p.pos += int(n)
	case BnkStruct:
		fields, err := p.fields(true)
		if err != nil {
			return v, err
		}
		v.Fields = fields
	case BnkList:
		header, err := p.varint()
		if err != nil {
			return v, err
		}
		for range header >> 3 {
			item, err := p.value(BnkType(header & 7))
			if err != nil {
				return v, err
			}
			v.Items = append(v.Items, item)
		}
	default:
		return v, fmt.Errorf("unknown value type %d", t)
	}
	return v, nil
}

func (v BnkValue) Field(id int) (BnkValue, bool) {
	for _, f := range v.Fields {
		if f.ID == id {
			return f.Value, true
		}
	}
	return BnkValue{}, false
}

func (v BnkValue) Repeated(id int) []BnkValue {
	f, ok := v.Field(id)
	switch {
	case !ok:
		return nil
	case f.Type == BnkList:
		return f.Items
	}
	return []BnkValue{f}
}
