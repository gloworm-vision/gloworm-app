package networktables

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	badger "github.com/dgraph-io/badger/v2"
	"github.com/sirupsen/logrus"
)

type ClientConfig struct {
	Addr     string
	Identity string
}

type Client struct {
	Store  Store
	Logger *logrus.Logger
	Config ClientConfig

	conn   net.Conn
	connMu *sync.Mutex
}

func (c *Client) Open(ctx context.Context) error {
	if c.Config.Addr == "" {
		c.Config.Addr = ":1735"
	}

	if c.Config.Identity == "" {
		hostname, err := os.Hostname()
		if err == nil {
			c.Config.Identity = hostname
		} else {
			c.Config.Identity = "networktables-go"
		}
	}

	if c.Store == nil {
		store, err := OpenBadgerDB(badger.DefaultOptions("").WithInMemory(true))
		if err != nil {
			return fmt.Errorf("no store was specified, tried to use badger in memory but got: %w", err)
		}

		c.Store = store
	}

	c.connMu = new(sync.Mutex)

	return nil
}

func (c *Client) Close() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) getConn() (net.Conn, error) {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		conn, err := net.Dial("tcp", c.Config.Addr)
		if err != nil {
			return nil, fmt.Errorf("couldn't dial into server: %w", err)
		}

		c.conn = conn

		fmt.Println("starting handshake")

		c.handshake()

		fmt.Println("handshaked")

		go func() {
			c.listen()
			c.connMu.Lock()
			c.conn = nil
			c.connMu.Unlock()
		}()
	}

	return c.conn, nil
}

func (c *Client) Ping() error {
	conn, err := c.getConn()
	if err != nil {
		return fmt.Errorf("unable to get connection to server: %w", err)
	}

	_, err = (&ntMessageType{Type: keepAliveMessageType}).Encode(conn)
	if err != nil {
		return fmt.Errorf("unable to encode ping to server: %w", err)
	}

	return err
}

const newEntryID = 0xFFFF

func (c *Client) Put(entry Entry) error {
	id, seq, err := c.Store.GetIDSeq(entry.Name)
	if err == nil { // todo: actually check for not found
		update := ntEntryUpdate{
			ID:             uint16(id),
			SequenceNumber: uint16(seq) + 1,
			EntryValue:     ntFromEntryValue(entry.Value),
		}

		if err := c.Store.UpdateSeq(id, seq+1); err != nil {
			return fmt.Errorf("couldn't update sequence number: %w", err)
		}

		_ = update

		// c.requests <- request{messageType: entryUpdateMessageType, data: &update}

		return nil
	}

	// assignment := assignmentFromEntry(newEntryID, entry)
	// c.requests <- request{messageType: entryAssignmentMessageType, data: &assignment}

	return nil
}

func (c *Client) Get(name string) (Entry, error) {
	value, err := c.Store.GetByName(name)
	if err != nil {
		return Entry{}, fmt.Errorf("couldn't get entry by name: %w", err)
	}

	return value, nil
}

func (c *Client) Delete(name string) error {
	id, err := c.Store.DeleteByName(name)
	if err != nil {
		return fmt.Errorf("couldn't delete entry: %w", err)
	}
	_ = id

	// c.requests <- request{messageType: entryDeleteMessageType, data: &ntEntryDelete{ID: uint16(id)}}

	return nil
}

const protocolVersion = 0x0300

func (c *Client) handshake() error {
	// handshake callers should have a connMu lock acquired

	conn := c.conn

	if c.Logger != nil {
		c.Logger.Infof("identifying as %q to server at %q", c.Config.Identity, conn.RemoteAddr().String())
	}
	if err := writeClientHello(conn, protocolVersion, c.Config.Identity); err != nil {
		return fmt.Errorf("couldn't send client hello to server: %w", err)
	}

	seen, identity, err := readServerHello(conn)
	if err != nil {
		return fmt.Errorf("couldn't read server hello: %w", err)
	}

	if c.Logger != nil {
		c.Logger.Infof("connected to server %q (seen = %t)", identity, seen)
	}

	// now we need to load all entry assignments from the server, and collect
	// a list of entry names so we can send the server any entry assignments
	// they're missing

	var messageType ntMessageType
	var assignment ntEntryAssignment
	serverNames := make(map[string]struct{})

	for {
		if _, err := messageType.Decode(conn); err != nil {
			return fmt.Errorf("couldn't decode server message type: %w", err)
		}

		if messageType.Type == serverHelloCompleteMessageType {
			break
		} else if messageType.Type != entryAssignmentMessageType {
			return fmt.Errorf("server responded with unexpected message type %x instead of %x", messageType.Type, entryAssignmentMessageType)
		}

		if _, err := assignment.Decode(conn); err != nil {
			return fmt.Errorf("couldn't decode assignment: %w", err)
		}

		if err := c.Store.Create(entryFromAssignment(assignment)); err != nil {
			return fmt.Errorf("couldn't create server assignment %q: %w", assignment.ID, err)
		}

		serverNames[assignment.Name] = struct{}{}
	}

	if c.Logger != nil {
		c.Logger.Infof("saved %d entry assignments from server", len(serverNames))
	}

	clientNames, err := c.Store.GetNames()
	if err != nil {
		return fmt.Errorf("couldn't get existing entry names from store: %w", err)
	}

	var clientCreateCount int
	for _, name := range clientNames {
		if _, ok := serverNames[name]; ok {
			continue
		}

		entry, err := c.Store.GetByName(name)
		if err != nil {
			return fmt.Errorf("couldn't get client entry %q: %w", name, err)
		}

		if err := writeEntryAssignment(conn, entry); err != nil {
			return fmt.Errorf("couldn't write entry assignment: %w", err)
		}

		clientCreateCount++
	}

	if c.Logger != nil {
		c.Logger.Infof("client sent server %d missing entry assignments", clientCreateCount)
	}

	if _, err := (&ntMessageType{Type: clientHelloCompleteMessageType}).Encode(conn); err != nil {
		return fmt.Errorf("couldn't write client hello message: %w", err)
	}

	if c.Logger != nil {
		c.Logger.Infof("completed handshake with server %q", identity)
	}

	// we might have entry assignments to process now, those will be handled by the
	// request handler

	return nil
}

func (c *Client) listen() {
	for {
		select {
		default:
			err := c.handleResponse()
			if errors.Is(err, io.EOF) {
				if c.Logger != nil {
					c.Logger.Errorf("server closed connection")
				}

				return
			} else if err != nil {
				if c.Logger != nil {
					c.Logger.Errorf("couldn't handle response: %s", err)
				}
			}
		}
	}
}

const clearAllEntriesMagic = 0xD06CB27A

func (c *Client) handleResponse() error {
	var messageType ntMessageType
	if _, err := messageType.Decode(c.conn); err != nil {
		return fmt.Errorf("couldn't decode message type: %w", err)
	}

	switch messageType.Type {
	case keepAliveMessageType:
	case entryAssignmentMessageType:
		var assignment ntEntryAssignment
		if _, err := assignment.Decode(c.conn); err != nil {
			return fmt.Errorf("couldn't decode entry assignment: %w", err)
		}

		entry := entryFromAssignment(assignment)
		if err := c.Store.Create(entry); err != nil {
			return fmt.Errorf("couldn't create entry assignment: %w", err)
		}

		if c.Logger != nil {
			c.Logger.WithField("name", entry.Name).Info("created entry")
		}
	case entryUpdateMessageType:
		var entryUpdate ntEntryUpdate
		if _, err := entryUpdate.Decode(c.conn); err != nil {
			return fmt.Errorf("couldn't decode entry update: %w", err)
		}

		err := c.Store.UpdateValue(int(entryUpdate.ID), entryValueFromNt(entryUpdate.EntryValue, entryUpdate.SequenceNumber))
		if err != nil {
			return fmt.Errorf("couldn't update entry: %w", err)
		}

		if c.Logger != nil {
			c.Logger.WithField("id", entryUpdate.ID).Info("updated entry")
		}
	case entryFlagsUpdateMessageType:
		var flagsUpdate ntEntryFlagsUpdate
		if _, err := flagsUpdate.Decode(c.conn); err != nil {
			return fmt.Errorf("couldn't decode entry flags update: %w", err)
		}

		err := c.Store.UpdateOptions(int(flagsUpdate.ID), entryOptionsFromNt(flagsUpdate.EntryFlags))
		if err != nil {
			return fmt.Errorf("couldn't update options: %q", err)
		}

		if c.Logger != nil {
			c.Logger.WithField("id", flagsUpdate.ID).Info("updated entry flags")
		}
	case entryDeleteMessageType:
		var delete ntEntryDelete
		if _, err := delete.Decode(c.conn); err != nil {
			return fmt.Errorf("couldn't decode entry flags update: %w", err)
		}

		if err := c.Store.Delete(int(delete.ID)); err != nil {
			return fmt.Errorf("couldn't delete entry: %w", err)
		}

		if c.Logger != nil {
			c.Logger.WithField("id", delete.ID).Info("deleted entry")
		}
	case clearAllEntriesMessageType:
		var clear ntClearAllEntries
		if _, err := clear.Decode(c.conn); err != nil {
			return fmt.Errorf("couldn't decode clear all entries: %w", err)
		}

		if clear.Magic == clearAllEntriesMagic {
			if err := c.Store.Clear(); err != nil {
				return fmt.Errorf("unable to clear store: %w", err)
			}
		}

		if c.Logger != nil {
			c.Logger.Info("cleared all entries")
		}
	default:
		return fmt.Errorf("got unknown message type: %d", messageType.Type)
	}

	return nil
}

// type request struct {
// 	messageType uint8
// 	data        encoder
// }

// type encoder interface {
// 	Encode(w io.Writer) (int, error)
// }

// func (c *Client) requestHandler(ctx context.Context) {
// 	keepAlive := time.NewTicker(time.Millisecond * 1000)
// 	defer keepAlive.Stop()

// 	for {
// 		select {
// 		case request := <-c.requests:
// 			t := ntMessageType{Type: request.messageType}
// 			if _, err := t.Encode(c.conn); err != nil {
// 				if c.Logger != nil {
// 					c.Logger.Error("unable to encode message type: %w", err)
// 					continue
// 				}
// 			}

// 			if _, err := request.data.Encode(c.conn); err != nil {
// 				if c.Logger != nil {
// 					c.Logger.Error("unable to encode request: %w", err)
// 				}
// 			}
// 		case <-keepAlive.C:
// 			t := ntMessageType{Type: keepAliveMessageType}
// 			if _, err := t.Encode(c.conn); err != nil {
// 				if c.Logger != nil {
// 					c.Logger.Error("unable to encode keep alive: %w", err)
// 					continue
// 				}
// 			}
// 		case <-ctx.Done():
// 			return
// 		}
// 	}
// }

// these translation functions are pretty annoying, but I
// think it's important to decouple networktables entries
// from our native entries

func entryFromAssignment(nt ntEntryAssignment) Entry {
	return Entry{
		ID:             int(nt.ID),
		SequenceNumber: int(nt.SequenceNumber),
		Name:           nt.Name,
		Options:        entryOptionsFromNt(nt.EntryFlags),
		Value:          entryValueFromNt(nt.EntryValue, nt.SequenceNumber),
	}
}

func assignmentFromEntry(id int, entry Entry) ntEntryAssignment {
	return ntEntryAssignment{
		Name:           entry.Name,
		SequenceNumber: uint16(entry.SequenceNumber),
		ID:             uint16(id),
		EntryFlags: ntEntryFlags{
			Persist: entry.Options.Persist,
		},
		EntryValue: ntFromEntryValue(entry.Value),
	}
}

func entryOptionsFromNt(nt ntEntryFlags) EntryOptions {
	return EntryOptions{
		Persist: nt.Persist,
	}
}

func entryValueFromNt(nt ntEntryValue, seq uint16) EntryValue {
	return EntryValue{
		EntryType:    entryTypeFromNt(nt.Type),
		Boolean:      nt.BooleanValue,
		Double:       nt.DoubleValue,
		RawData:      nt.RawDataValue,
		String:       nt.StringValue,
		BooleanArray: nt.BooleanArrayValue,
		DoubleArray:  nt.DoubleArrayValue,
		StringArray:  nt.StringArrayValue,
	}
}

func ntFromEntryValue(v EntryValue) ntEntryValue {
	return ntEntryValue{
		Type:              ntFromEntryType(v.EntryType),
		BooleanValue:      v.Boolean,
		DoubleValue:       v.Double,
		RawDataValue:      v.RawData,
		StringValue:       v.String,
		BooleanArrayValue: v.BooleanArray,
		DoubleArrayValue:  v.DoubleArray,
		StringArrayValue:  v.StringArray,
	}
}

func entryTypeFromNt(nt ntEntryType) EntryType {
	switch nt {
	case booleanEntryType:
		return Boolean
	case doubleEntryType:
		return Double
	case rawDataEntryType:
		return RawData
	case stringEntryType:
		return String
	case booleanArrayEntryType:
		return BooleanArray
	case doubleArrayEntryType:
		return DoubleArray
	case stringArrayEntryType:
		return StringArray
	}

	return EntryType(-1)
}

func ntFromEntryType(t EntryType) ntEntryType {
	switch t {
	case Boolean:
		return booleanEntryType
	case Double:
		return doubleEntryType
	case RawData:
		return rawDataEntryType
	case String:
		return stringEntryType
	case BooleanArray:
		return booleanArrayEntryType
	case DoubleArray:
		return doubleArrayEntryType
	case StringArray:
		return stringArrayEntryType
	}

	return ntEntryType(-1)
}

func writeClientHello(w io.Writer, protocolRevision uint16, identity string) error {
	if _, err := (&ntMessageType{Type: clientHelloMessageType}).Encode(w); err != nil {
		return fmt.Errorf("couldn't encode client hello message type: %w", err)
	}

	hello := clientHello{ClientProtocolRevision: protocolVersion, Identity: identity}
	if _, err := hello.Encode(w); err != nil {
		return fmt.Errorf("couldn't encode client hello message: %w", err)
	}

	return nil
}

func readServerHello(rd io.Reader) (bool, string, error) {
	var messageType ntMessageType
	if _, err := messageType.Decode(rd); err != nil {
		return false, "", fmt.Errorf("couldn't decode message type: %w", err)
	}

	if messageType.Type != serverHelloMessageType {
		return false, "", fmt.Errorf("server responded with incorrect message type %x instead of %x", messageType.Type, serverHelloMessageType)
	}

	var serverHello ntServerHello
	if _, err := serverHello.Decode(rd); err != nil {
		return false, "", fmt.Errorf("couldn't decode server hello: %w", err)
	}

	return serverHello.Flags.ClientSeen, serverHello.ServerIdentity, nil
}

func writeEntryAssignment(w io.Writer, entry Entry) error {
	if _, err := (&ntMessageType{Type: entryAssignmentMessageType}).Encode(w); err != nil {
		return fmt.Errorf("couldn't encode entry assignment message type: %w", err)
	}

	assignment := assignmentFromEntry(int(createId), entry)

	if _, err := assignment.Encode(w); err != nil {
		return fmt.Errorf("couldn't encode entry assignment: %w", err)
	}

	return nil
}
