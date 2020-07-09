package networktables

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/sirupsen/logrus"
)

// Client is a networktables 3 client. It's zero value is usable for communicating with a local
// networktables server at port 1735 with an in-memory store and logging disabled.
type Client struct {
	Store    Store
	Logger   *logrus.Logger
	Addr     string
	Identity string

	memoryStore *badgerDB
	storeMu     sync.Mutex

	conn   net.Conn
	connMu sync.Mutex
}

// Ping sends a keep alive to the server. If you need to keep the connection alive you
// should call this function no more than once every 100ms.
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

// UpdateValue updates the entry value for an existing entry with the given name, and
// issues an entry value update to the server.
func (c *Client) UpdateValue(name string, value EntryValue) error {
	store, err := c.getStore()
	if err != nil {
		return fmt.Errorf("couldn't get underlying store: %w", err)
	}

	id, seq, err := store.GetIDSeq(name)
	if err != nil { // todo: actually check for not found
		return fmt.Errorf("unable to get existing entry (perhaps it hasn't been created yet): %w", err)
	}

	if err := store.UpdateValue(id, seq+1, value); err != nil {
		return fmt.Errorf("couldn't update value: %w", err)
	}

	conn, err := c.getConn()
	if err != nil {
		return fmt.Errorf("unable to get connection to server: %w", err)
	}

	if err := writeEntryUpdate(conn, id, seq+1, value); err != nil {
		return fmt.Errorf("unable to write entry value update to server: %w", err)
	}

	return nil
}

// UpdateOptions updates the entry options for an existing entry with the given name, and
// issues an entry options update to the server.
func (c *Client) UpdateOptions(name string, opt EntryOptions) error {
	store, err := c.getStore()
	if err != nil {
		return fmt.Errorf("couldn't get underlying store: %w", err)
	}

	id, _, err := store.GetIDSeq(name)
	if err != nil { // todo: actually check for not found
		return fmt.Errorf("unable to get existing entry (perhaps it hasn't been created yet): %w", err)
	}

	if err := store.UpdateOptions(id, opt); err != nil {
		return fmt.Errorf("couldn't update options: %w", err)
	}

	conn, err := c.getConn()
	if err != nil {
		return fmt.Errorf("unable to get connection to server: %w", err)
	}

	if err := writeEntryFlagsUpdate(conn, id, opt); err != nil {
		return fmt.Errorf("unable to write entry options update to server: %w", err)
	}

	return nil
}

// Create tells the server to issue an entry assignment to all clients (including us)
// for the given entry. It does not immediately create an entry in the underlying store,
// and for this reason it's not guaranteed that the value will exist after this function
// returns, so successive Puts may fail. It is only guaranteed that the create request
// has been written to the server. This is unfortunately due to how the networktables
// protocol works, because there is no way for us to know which entry assignment from the
// server corresponds to our entry assignment.
func (c *Client) Create(entry Entry) error {
	conn, err := c.getConn()
	if err != nil {
		return fmt.Errorf("unable to get connection to server: %w", err)
	}

	if err := writeEntryAssignment(conn, entry); err != nil {
		return fmt.Errorf("unable to write entry assignment to server: %w", err)
	}

	return nil
}

// Get returns an entry from the underlying store for the given name.
func (c *Client) Get(name string) (Entry, error) {
	store, err := c.getStore()
	if err != nil {
		return Entry{}, fmt.Errorf("couldn't get underlying store: %w", err)
	}

	entry, err := store.GetByName(name)
	if err != nil {
		return entry, fmt.Errorf("couldn't get entry by name: %w", err)
	}

	return entry, nil
}

// Delete deletes an entry from the underlying store and issues a delete request to the
// server.
func (c *Client) Delete(name string) error {
	store, err := c.getStore()
	if err != nil {
		return fmt.Errorf("couldn't get underlying store: %w", err)
	}

	id, err := store.DeleteByName(name)
	if err != nil {
		return fmt.Errorf("couldn't delete entry: %w", err)
	}

	conn, err := c.getConn()
	if err != nil {
		return fmt.Errorf("unable to get connection to server: %w", err)
	}

	if err := writeDelete(conn, id); err != nil {
		return fmt.Errorf("unable to write delete request to server: %w", err)
	}

	return nil
}

// Close closes the underlying connection if one exists.
func (c *Client) Close() error {
	c.storeMu.Lock()
	defer c.storeMu.Unlock()
	if c.memoryStore != nil {
		_ = c.memoryStore.db.Close()
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()

	var err error
	if c.conn != nil {
		err = c.conn.Close()
	}
	c.conn = nil
	return err
}

func (c *Client) getStore() (Store, error) {
	if c.Store != nil {
		return c.Store, nil
	}

	c.storeMu.Lock()
	defer c.storeMu.Unlock()

	if c.memoryStore == nil {
		db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true).WithLogger(nil))
		if err != nil {
			return nil, fmt.Errorf("no store was specified, tried to use badger in memory but got: %w", err)
		}

		c.memoryStore = &badgerDB{db: db}
	}

	return c.memoryStore, nil
}

func (c *Client) getConn() (net.Conn, error) {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		addr := c.Addr
		if addr == "" {
			addr = ":1735"
		}

		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("couldn't dial into server: %w", err)
		}

		c.conn = conn

		c.handshake()

		go func() {
			c.listen()
			c.connMu.Lock()
			c.conn = nil
			c.connMu.Unlock()
		}()
	}

	return c.conn, nil
}

const protocolVersion = 0x0300

// handshake callers should have a connMu lock acquired before calling handshake
func (c *Client) handshake() error {
	store, err := c.getStore()
	if err != nil {
		return fmt.Errorf("couldn't get underlying store: %w", err)
	}

	conn := c.conn

	identity := c.Identity
	if identity == "" {
		hostname, err := os.Hostname()
		if err == nil {
			identity = hostname
		} else {
			identity = "networktables-go"
		}
	}

	if c.Logger != nil {
		c.Logger.Infof("identifying as %q to server at %q", identity, conn.RemoteAddr().String())
	}
	if err := writeClientHello(conn, protocolVersion, identity); err != nil {
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

		var assignment ntEntryAssignment
		if _, err := assignment.Decode(conn); err != nil {
			return fmt.Errorf("couldn't decode assignment: %w", err)
		}

		if err := store.Create(entryFromAssignment(assignment)); err != nil {
			return fmt.Errorf("couldn't create server assignment %q: %w", assignment.ID, err)
		}

		serverNames[assignment.Name] = struct{}{}
	}

	if c.Logger != nil {
		c.Logger.Infof("saved %d entry assignments from server", len(serverNames))
	}

	clientNames, err := store.GetNames()
	if err != nil {
		return fmt.Errorf("couldn't get existing entry names from store: %w", err)
	}

	var clientCreateCount int
	for _, name := range clientNames {
		if _, ok := serverNames[name]; ok {
			continue
		}

		entry, err := store.GetByName(name)
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
			if c.conn == nil {
				return
			}

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

	store, err := c.getStore()
	if err != nil {
		return fmt.Errorf("couldn't get underlying store: %w", err)
	}

	switch messageType.Type {
	case keepAliveMessageType:
	case entryAssignmentMessageType:
		var assignment ntEntryAssignment
		if _, err := assignment.Decode(c.conn); err != nil {
			return fmt.Errorf("couldn't decode entry assignment: %w", err)
		}

		entry := entryFromAssignment(assignment)
		if err := store.Create(entry); err != nil {
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

		err := store.UpdateValue(int(entryUpdate.ID), int(entryUpdate.SequenceNumber), entryValueFromNt(entryUpdate.EntryValue))
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

		err := store.UpdateOptions(int(flagsUpdate.ID), entryOptionsFromNt(flagsUpdate.EntryFlags))
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

		if err := store.Delete(int(delete.ID)); err != nil {
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
			if err := store.Clear(); err != nil {
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

// these translation functions are pretty annoying, but I
// think it's important to decouple networktables entries
// from our native entries

func entryFromAssignment(nt ntEntryAssignment) Entry {
	return Entry{
		ID:             int(nt.ID),
		SequenceNumber: int(nt.SequenceNumber),
		Name:           nt.Name,
		Options:        entryOptionsFromNt(nt.EntryFlags),
		Value:          entryValueFromNt(nt.EntryValue),
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

func ntFromEntryOptions(nt EntryOptions) ntEntryFlags {
	return ntEntryFlags{
		Persist: nt.Persist,
	}
}

func entryValueFromNt(nt ntEntryValue) EntryValue {
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

	assignment := assignmentFromEntry(int(createID), entry)

	if _, err := assignment.Encode(w); err != nil {
		return fmt.Errorf("couldn't encode entry assignment: %w", err)
	}

	return nil
}

func writeEntryUpdate(w io.Writer, id int, seq int, value EntryValue) error {
	if _, err := (&ntMessageType{Type: entryUpdateMessageType}).Encode(w); err != nil {
		return fmt.Errorf("couldn't encode entry update message type: %w", err)
	}

	update := ntEntryUpdate{
		ID:             uint16(id),
		SequenceNumber: uint16(seq),
		EntryValue:     ntFromEntryValue(value),
	}

	if _, err := update.Encode(w); err != nil {
		return fmt.Errorf("couldn't encode entry value update: %w", err)
	}

	return nil
}

func writeDelete(w io.Writer, id int) error {
	if _, err := (&ntMessageType{Type: entryDeleteMessageType}).Encode(w); err != nil {
		return fmt.Errorf("couldn't encode entry delete message type: %w", err)
	}

	delete := ntEntryDelete{
		ID: uint16(id),
	}

	if _, err := delete.Encode(w); err != nil {
		return fmt.Errorf("couldn't encode delete: %w", err)
	}

	return nil
}

func writeEntryFlagsUpdate(w io.Writer, id int, opt EntryOptions) error {
	if _, err := (&ntMessageType{Type: entryFlagsUpdateMessageType}).Encode(w); err != nil {
		return fmt.Errorf("couldn't encode entry update message type: %w", err)
	}

	update := ntEntryFlagsUpdate{
		ID:         uint16(id),
		EntryFlags: ntFromEntryOptions(opt),
	}

	if _, err := update.Encode(w); err != nil {
		return fmt.Errorf("couldn't encode entry flags update: %w", err)
	}

	return nil
}
