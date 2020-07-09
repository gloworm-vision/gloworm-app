package networktables

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	keepAliveMessageType                   uint8 = 0x00
	clientHelloMessageType                 uint8 = 0x01
	protocolVersionUnsupportedMessageType  uint8 = 0x02
	serverHelloCompleteMessageType         uint8 = 0x03
	serverHelloMessageType                 uint8 = 0x04
	clientHelloCompleteMessageType         uint8 = 0x05
	entryAssignmentMessageType             uint8 = 0x10
	entryUpdateMessageType                 uint8 = 0x11
	entryFlagsUpdateMessageType            uint8 = 0x12
	entryDeleteMessageType                 uint8 = 0x13
	clearAllEntriesMessageType             uint8 = 0x14
	remoteProcedureCallExecuteMessageType  uint8 = 0x20
	remoteProcedureCallResponseMessageType uint8 = 0x21
)

type ntMessageType struct {
	Type uint8
}

func (m *ntMessageType) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 1)
	n, err := io.ReadFull(rd, buf)
	if err != nil {
		return n, fmt.Errorf("couldn't read message type: %w", err)
	}

	m.Type = uint8(buf[0])

	return n, nil
}

func (m *ntMessageType) Encode(w io.Writer) (int, error) {
	buf := []byte{byte(m.Type)}
	n, err := w.Write(buf)
	if err != nil {
		return n, fmt.Errorf("couldn't write message type: %w", err)
	}

	return n, nil
}

type clientHello struct {
	ClientProtocolRevision uint16
	Identity               string
}

func (c *clientHello) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 2)
	revN, err := io.ReadFull(rd, buf)
	if err != nil {
		return revN, fmt.Errorf("unable to read protocol revision: %w", err)
	}
	c.ClientProtocolRevision = binary.BigEndian.Uint16(buf)

	identity := ntString{}
	identityN, err := identity.Decode(rd)
	if err != nil {
		return revN, fmt.Errorf("unable to read identity: %w", err)
	}

	c.Identity = identity.V

	return revN + identityN, nil
}

func (c *clientHello) Encode(w io.Writer) (int, error) {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, c.ClientProtocolRevision)
	revN, err := w.Write(buf)
	if err != nil {
		return revN, fmt.Errorf("unable to write protocol revision: %w", err)
	}

	identity := ntString{V: c.Identity}
	identityN, err := identity.Encode(w)
	if err != nil {
		return revN, fmt.Errorf("unable to write identity: %w", err)
	}

	return revN + identityN, nil
}

type ntProtocolVersionUnsupported struct {
	ServerSupportedProtocolRevision uint16
}

func (p *ntProtocolVersionUnsupported) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 2)
	n, err := io.ReadFull(rd, buf)
	if err != nil {
		return n, fmt.Errorf("unable to read protocol revision: %w", err)
	}
	p.ServerSupportedProtocolRevision = binary.BigEndian.Uint16(buf)

	return n, nil
}

func (p *ntProtocolVersionUnsupported) Encode(w io.Writer) (int, error) {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, p.ServerSupportedProtocolRevision)
	n, err := w.Write(buf)
	if err != nil {
		return n, fmt.Errorf("unable to write protocol revision: %w", err)
	}

	return n, nil
}

const (
	clientSeenMask byte = 0x00000001
)

type ntServerFlag struct {
	ClientSeen bool
}

func (sf *ntServerFlag) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 1)
	n, err := io.ReadFull(rd, buf)
	if err != nil {
		return n, fmt.Errorf("can't read entry flag from reader: %w", err)
	}

	sf.ClientSeen = buf[0]&clientSeenMask == 0x01

	return n, nil
}

func (sf *ntServerFlag) Encode(w io.Writer) (int, error) {
	var v byte

	if sf.ClientSeen {
		v |= clientSeenMask
	}

	return w.Write([]byte{v})
}

type ntServerHello struct {
	Flags          ntServerFlag
	ServerIdentity string
}

func (s *ntServerHello) Decode(rd io.Reader) (int, error) {
	flagN, err := s.Flags.Decode(rd)
	if err != nil {
		return flagN, fmt.Errorf("unable to read flags: %w", err)
	}

	identity := ntString{}
	identityN, err := identity.Decode(rd)
	if err != nil {
		return flagN, fmt.Errorf("unable to read identity: %w", err)
	}

	s.ServerIdentity = identity.V

	return flagN + identityN, nil
}

func (s *ntServerHello) Encode(w io.Writer) (int, error) {
	flagN, err := s.Flags.Encode(w)
	if err != nil {
		return flagN, fmt.Errorf("unable to write flags: %w", err)
	}

	identity := ntString{V: s.ServerIdentity}
	identityN, err := identity.Encode(w)
	if err != nil {
		return flagN, fmt.Errorf("unable to write identity: %w", err)
	}

	return flagN + identityN, nil
}

type ntEntryAssignment struct {
	Name           string
	ID             uint16
	SequenceNumber uint16

	EntryValue ntEntryValue
	EntryFlags ntEntryFlags
}

const (
	createID uint16 = 0xFFFF
)

func (ea *ntEntryAssignment) Decode(rd io.Reader) (int, error) {
	totalRead := 0

	name := ntString{}
	nameN, err := name.Decode(rd)
	totalRead += nameN
	if err != nil {
		return totalRead, fmt.Errorf("unable to read name: %w", err)
	}

	ea.Name = name.V

	buf := make([]byte, 5)
	bufN, err := io.ReadFull(rd, buf)
	totalRead += bufN
	if err != nil {
		return totalRead, fmt.Errorf("unable to read entry assignment buffer: %w", err)
	}

	ea.EntryValue.Type = ntEntryType(buf[0])
	ea.ID = binary.BigEndian.Uint16(buf[1:3])
	ea.SequenceNumber = binary.BigEndian.Uint16(buf[3:5])

	flagN, err := ea.EntryFlags.Decode(rd)
	totalRead += flagN
	if err != nil {
		return totalRead, fmt.Errorf("unable to read entry assignment flags: %w", err)
	}

	valueN, err := ea.EntryValue.Decode(rd)
	totalRead += valueN
	if err != nil {
		return totalRead, fmt.Errorf("unable to read entry type: %w", err)
	}

	return totalRead, nil
}

func (ea *ntEntryAssignment) Encode(w io.Writer) (int, error) {
	totalWritten := 0

	name := ntString{ea.Name}
	nameN, err := name.Encode(w)
	totalWritten += nameN
	if err != nil {
		return totalWritten, fmt.Errorf("unable to write name: %w", err)
	}

	buf := make([]byte, 5)
	buf[0] = byte(ea.EntryValue.Type)
	binary.BigEndian.PutUint16(buf[1:3], ea.ID)
	binary.BigEndian.PutUint16(buf[3:5], ea.SequenceNumber)
	bufN, err := w.Write(buf)
	totalWritten += bufN
	if err != nil {
		return totalWritten, fmt.Errorf("unable to write entry assignment buffer: %w", err)
	}

	flagN, err := ea.EntryFlags.Encode(w)
	totalWritten += flagN
	if err != nil {
		return totalWritten, fmt.Errorf("unable to write entry assignment flags: %w", err)
	}

	valueN, err := ea.EntryValue.Encode(w)
	totalWritten += valueN
	if err != nil {
		return totalWritten, fmt.Errorf("unable to read entry type: %w", err)
	}

	return totalWritten, nil
}

type ntEntryUpdate struct {
	ID             uint16
	SequenceNumber uint16

	EntryValue ntEntryValue
}

func (eu *ntEntryUpdate) Decode(rd io.Reader) (int, error) {
	totalRead := 0

	buf := make([]byte, 5)
	bufN, err := io.ReadFull(rd, buf)
	totalRead += bufN
	if err != nil {
		return totalRead, fmt.Errorf("unable to read entry update buffer: %w", err)
	}

	eu.ID = binary.BigEndian.Uint16(buf[0:2])
	eu.SequenceNumber = binary.BigEndian.Uint16(buf[2:4])
	eu.EntryValue.Type = ntEntryType(buf[4])

	valueN, err := eu.EntryValue.Decode(rd)
	totalRead += valueN
	if err != nil {
		return totalRead, fmt.Errorf("unable to read entry type: %w", err)
	}

	return totalRead, nil
}

func (eu *ntEntryUpdate) Encode(w io.Writer) (int, error) {
	totalWritten := 0

	buf := make([]byte, 5)
	binary.BigEndian.PutUint16(buf[0:2], eu.ID)
	binary.BigEndian.PutUint16(buf[2:4], eu.SequenceNumber)
	buf[4] = byte(eu.EntryValue.Type)
	bufN, err := w.Write(buf)
	totalWritten += bufN
	if err != nil {
		return totalWritten, fmt.Errorf("unable to write entry update buffer: %w", err)
	}

	valueN, err := eu.EntryValue.Encode(w)
	totalWritten += valueN
	if err != nil {
		return totalWritten, fmt.Errorf("unable to read entry type: %w", err)
	}

	return totalWritten, nil
}

type ntEntryFlagsUpdate struct {
	ID uint16

	EntryFlags ntEntryFlags
}

func (efu *ntEntryFlagsUpdate) Decode(rd io.Reader) (int, error) {
	totalRead := 0

	buf := make([]byte, 2)
	bufN, err := io.ReadFull(rd, buf)
	totalRead += bufN
	if err != nil {
		return totalRead, fmt.Errorf("unable to read entry id: %w", err)
	}
	efu.ID = binary.BigEndian.Uint16(buf)

	flagN, err := efu.EntryFlags.Decode(rd)
	totalRead += flagN
	if err != nil {
		return totalRead, fmt.Errorf("unable to read entry assignment flags: %w", err)
	}

	return totalRead, nil
}

func (efu *ntEntryFlagsUpdate) Encode(w io.Writer) (int, error) {
	totalWritten := 0

	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, efu.ID)
	bufN, err := w.Write(buf)
	totalWritten += bufN
	if err != nil {
		return totalWritten, fmt.Errorf("unable to write entry id: %w", err)
	}

	flagN, err := efu.EntryFlags.Encode(w)
	totalWritten += flagN
	if err != nil {
		return totalWritten, fmt.Errorf("unable to write entry assignment flags: %w", err)
	}

	return totalWritten, nil
}

type ntEntryDelete struct {
	ID uint16
}

func (ed *ntEntryDelete) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 2)
	n, err := io.ReadFull(rd, buf)
	if err != nil {
		return n, fmt.Errorf("unable to read entry id: %w", err)
	}
	ed.ID = binary.BigEndian.Uint16(buf)

	return n, nil
}

func (ed *ntEntryDelete) Encode(w io.Writer) (int, error) {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, ed.ID)
	n, err := w.Write(buf)
	if err != nil {
		return n, fmt.Errorf("unable to write entry id: %w", err)
	}

	return n, nil
}

type ntClearAllEntries struct {
	ID    uint16
	Magic uint64
}

func (ce *ntClearAllEntries) Decode(rd io.Reader) (int, error) {
	buf := make([]byte, 6)
	n, err := io.ReadFull(rd, buf)
	if err != nil {
		return n, fmt.Errorf("unable to read clear all entries buf: %w", err)
	}
	ce.ID = binary.BigEndian.Uint16(buf[0:2])
	ce.Magic = binary.BigEndian.Uint64(buf[2:6])

	return n, nil
}

func (ce *ntClearAllEntries) Encode(w io.Writer) (int, error) {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf[0:2], ce.ID)
	binary.BigEndian.PutUint64(buf[2:6], ce.Magic)
	n, err := w.Write(buf)
	if err != nil {
		return n, fmt.Errorf("unable to write clear all entries buf: %w", err)
	}

	return n, nil
}
