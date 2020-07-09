package networktables

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

type ntEntryType int

const (
	booleanEntryType                       ntEntryType = 0x00
	doubleEntryType                        ntEntryType = 0x01
	stringEntryType                        ntEntryType = 0x02
	rawDataEntryType                       ntEntryType = 0x03
	booleanArrayEntryType                  ntEntryType = 0x10
	doubleArrayEntryType                   ntEntryType = 0x11
	stringArrayEntryType                   ntEntryType = 0x12
	remoteProcedureCallDefinitionEntryType ntEntryType = 0x20
)

type ntEntryFlags struct {
	Persist bool
}

const (
	persistMask byte = 0x00000001
)

func (ef *ntEntryFlags) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 1)
	n, err := io.ReadFull(rd, buf)
	if err != nil {
		return n, fmt.Errorf("can't read entry flag from reader: %w", err)
	}

	ef.Persist = buf[0]&persistMask == 0x01

	return n, nil
}

func (ef *ntEntryFlags) Encode(w io.Writer) (int, error) {
	var v byte

	if ef.Persist {
		v |= persistMask
	}

	return w.Write([]byte{v})
}

type ntBoolean struct {
	V bool
}

func (boolean *ntBoolean) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 1)
	n, err := io.ReadFull(rd, buf)
	if err != nil {
		return n, fmt.Errorf("can't read byte from reader: %w", err)
	}

	var v bool
	if buf[0] == 0x01 {
		v = true
	} else if buf[0] != 0x00 {
		return n, fmt.Errorf("boolean entry value must be 0x01 or 0x00, not %x", buf[0])
	}

	boolean.V = v

	return n, nil
}

func (boolean *ntBoolean) Encode(w io.Writer) (int, error) {
	val := byte(0x00)
	if boolean.V {
		val = 0x01
	}

	return w.Write([]byte{val})
}

type ntDouble struct {
	V float64
}

func (d *ntDouble) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 8)
	n, err := io.ReadFull(rd, buf)
	if err != nil {
		return n, fmt.Errorf("couldn't read 8 bytes from reader: %w", err)
	}

	bits := binary.BigEndian.Uint64(buf)
	f := math.Float64frombits(bits)

	d.V = f

	return n, nil
}

func (d *ntDouble) Encode(w io.Writer) (int, error) {
	bits := math.Float64bits(d.V)

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, bits)

	return w.Write(buf)
}

type uleb128 struct {
	V uint64
}

func (ul *uleb128) Encode(w io.Writer) (int, error) {
	buf := make([]byte, 0)

	for {
		c := uint8(ul.V & 0x7f)
		ul.V >>= 7
		if ul.V != 0 {
			c |= 0x80
		}
		buf = append(buf, c)
		if c&0x80 == 0 {
			break
		}
	}

	return w.Write(buf)
}

func (ul *uleb128) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 1)
	total := 0

	var x uint64
	var s, i uint
	for {
		n, err := io.ReadFull(rd, buf)
		total += n
		if err != nil {
			return total, fmt.Errorf("couldn't read byte: %w", err)
		}
		b := buf[0]

		x |= (uint64(0x7F & b)) << s
		if b&0x80 == 0 {
			break
		}

		s += 7
		i++
	}

	ul.V = x

	return total, nil
}

type ntString struct {
	V string
}

func (str *ntString) Decode(rd io.Reader) (int, error) {
	raw := ntRawData{}

	n, err := raw.Decode(rd)
	if err != nil {
		return n, fmt.Errorf("couldn't read string as raw data: %w", err)
	}

	str.V = string(raw.V)

	return n, nil
}

func (str *ntString) Encode(w io.Writer) (int, error) {
	raw := ntRawData{V: []byte(str.V)}

	n, err := raw.Encode(w)
	if err != nil {
		return n, fmt.Errorf("couldn't write string as raw data: %w", err)
	}

	return n, nil
}

type ntRawData struct {
	V []byte
}

func (raw *ntRawData) Decode(rd io.Reader) (int, error) {
	var size uleb128
	sizeN, err := size.Decode(rd)
	if err != nil {
		return sizeN, fmt.Errorf("couldn't read raw data size: %w", err)
	}

	buf := make([]byte, size.V)
	dataN, err := io.ReadFull(rd, buf)
	if err != nil {
		return sizeN + dataN, fmt.Errorf("couldn't read raw data: %w", err)
	}

	raw.V = buf

	return sizeN + dataN, nil
}

func (raw *ntRawData) Encode(w io.Writer) (int, error) {
	size := uleb128{V: uint64(len(raw.V))}
	sizeN, err := size.Encode(w)
	if err != nil {
		return sizeN, fmt.Errorf("couldn't write string size: %w", err)
	}

	dataN, err := w.Write(raw.V)
	if err != nil {
		return sizeN + dataN, fmt.Errorf("couldn't write raw data: %w", err)
	}

	return sizeN + dataN, nil
}

type ntBooleanArray struct {
	V []bool
}

func (ba *ntBooleanArray) Decode(rd io.Reader) (int, error) {
	size := make([]byte, 1)
	sizeN, err := rd.Read(size)
	if err != nil {
		return sizeN, fmt.Errorf("couldn't read boolean array size: %w", err)
	}

	totalRead := sizeN

	boolean := ntBoolean{}
	arrayLen := uint8(size[0])
	ba.V = make([]bool, arrayLen)

	for i := 0; i < int(arrayLen); i++ {
		n, err := boolean.Decode(rd)
		totalRead += n
		if err != nil {
			return totalRead, fmt.Errorf("couldn't read boolean array index %d: %w", i, err)
		}

		ba.V[i] = boolean.V
	}

	return totalRead, nil
}

func (ba *ntBooleanArray) Encode(w io.Writer) (int, error) {
	size := []byte{uint8(len(ba.V))}
	sizeN, err := w.Write(size)
	if err != nil {
		return sizeN, fmt.Errorf("couldn't write boolean array size: %w", err)
	}

	totalWritten := sizeN

	boolean := ntBoolean{}
	for i, b := range ba.V {
		boolean.V = b
		n, err := boolean.Encode(w)
		totalWritten += n
		if err != nil {
			return totalWritten, fmt.Errorf("couldn't write boolean array index %d: %w", i, err)
		}
	}

	return totalWritten, nil
}

type ntDoubleArray struct {
	V []float64
}

func (ba *ntDoubleArray) Decode(rd io.Reader) (int, error) {
	size := make([]byte, 1)
	sizeN, err := rd.Read(size)
	if err != nil {
		return sizeN, fmt.Errorf("couldn't read double array size: %w", err)
	}

	totalRead := sizeN

	double := ntDouble{}
	arrayLen := uint8(size[0])
	ba.V = make([]float64, arrayLen)

	for i := 0; i < int(arrayLen); i++ {
		n, err := double.Decode(rd)
		totalRead += n
		if err != nil {
			return totalRead, fmt.Errorf("couldn't read double array index %d: %w", i, err)
		}

		ba.V[i] = double.V
	}

	return totalRead, nil
}

func (ba *ntDoubleArray) Encode(w io.Writer) (int, error) {
	size := []byte{uint8(len(ba.V))}
	sizeN, err := w.Write(size)
	if err != nil {
		return sizeN, fmt.Errorf("couldn't write double array size: %w", err)
	}

	totalWritten := sizeN

	double := ntDouble{}
	for i, b := range ba.V {
		double.V = b
		n, err := double.Encode(w)
		totalWritten += n
		if err != nil {
			return totalWritten, fmt.Errorf("couldn't write double array index %d: %w", i, err)
		}
	}

	return totalWritten, nil
}

type ntStringArray struct {
	V []string
}

func (ba *ntStringArray) Decode(rd io.Reader) (int, error) {
	size := make([]byte, 1)
	sizeN, err := rd.Read(size)
	if err != nil {
		return sizeN, fmt.Errorf("couldn't read string array size: %w", err)
	}

	totalRead := sizeN

	str := ntString{}
	arrayLen := uint8(size[0])
	ba.V = make([]string, arrayLen)

	for i := 0; i < int(arrayLen); i++ {
		n, err := str.Decode(rd)
		totalRead += n
		if err != nil {
			return totalRead, fmt.Errorf("couldn't read string array index %d: %w", i, err)
		}

		ba.V[i] = str.V
	}

	return totalRead, nil
}

func (ba *ntStringArray) Encode(w io.Writer) (int, error) {
	size := []byte{uint8(len(ba.V))}
	sizeN, err := w.Write(size)
	if err != nil {
		return sizeN, fmt.Errorf("couldn't write string array size: %w", err)
	}

	totalWritten := sizeN

	str := ntString{}
	for i, b := range ba.V {
		str.V = b
		n, err := str.Encode(w)
		totalWritten += n
		if err != nil {
			return totalWritten, fmt.Errorf("couldn't write string array index %d: %w", i, err)
		}
	}

	return totalWritten, nil
}

type ntEntryValue struct {
	Type ntEntryType

	BooleanValue      bool
	DoubleValue       float64
	StringValue       string
	RawDataValue      []byte
	BooleanArrayValue []bool
	DoubleArrayValue  []float64
	StringArrayValue  []string
}

func (ev *ntEntryValue) Decode(rd io.Reader) (int, error) {
	var entryN int
	var err error

	switch ev.Type {
	case booleanEntryType:
		entry := ntBoolean{}
		entryN, err = entry.Decode(rd)
		ev.BooleanValue = entry.V
	case doubleEntryType:
		entry := ntDouble{}
		entryN, err = entry.Decode(rd)
		ev.DoubleValue = entry.V
	case stringEntryType:
		entry := ntString{}
		entryN, err = entry.Decode(rd)
		ev.StringValue = entry.V
	case rawDataEntryType:
		entry := ntRawData{}
		entryN, err = entry.Decode(rd)
		ev.RawDataValue = entry.V
	case booleanArrayEntryType:
		entry := ntBooleanArray{}
		entryN, err = entry.Decode(rd)
		ev.BooleanArrayValue = entry.V
	case doubleArrayEntryType:
		entry := ntDoubleArray{}
		entryN, err = entry.Decode(rd)
		ev.DoubleArrayValue = entry.V
	case stringArrayEntryType:
		entry := ntStringArray{}
		entryN, err = entry.Decode(rd)
		ev.StringArrayValue = entry.V
	default:
		err = fmt.Errorf("unknown entry type %x", ev.Type)
	}

	if err != nil {
		return entryN, fmt.Errorf("unable to read entry (expected type %x): %w", ev.Type, err)
	}

	return entryN, nil
}

func (ev *ntEntryValue) Encode(w io.Writer) (int, error) {
	var entryN int
	var err error

	switch ev.Type {
	case booleanEntryType:
		entry := ntBoolean{V: ev.BooleanValue}
		entryN, err = entry.Encode(w)
	case doubleEntryType:
		entry := ntDouble{V: ev.DoubleValue}
		entryN, err = entry.Encode(w)
	case stringEntryType:
		entry := ntString{V: ev.StringValue}
		entryN, err = entry.Encode(w)
	case rawDataEntryType:
		entry := ntRawData{V: ev.RawDataValue}
		entryN, err = entry.Encode(w)
	case booleanArrayEntryType:
		entry := ntBooleanArray{V: ev.BooleanArrayValue}
		entryN, err = entry.Encode(w)
	case doubleArrayEntryType:
		entry := ntDoubleArray{V: ev.DoubleArrayValue}
		entryN, err = entry.Encode(w)
	case stringArrayEntryType:
		entry := ntStringArray{V: ev.StringArrayValue}
		entryN, err = entry.Encode(w)
	default:
		err = fmt.Errorf("unknown entry type %x", ev.Type)
	}

	if err != nil {
		return entryN, fmt.Errorf("unable to read entry (expected type %x): %w", ev.Type, err)
	}

	return entryN, nil
}
