package networktables

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"strconv"

	badger "github.com/dgraph-io/badger/v2"
)

// Store defines a minimal interface for a generic networktables store.
type Store interface {
	GetValue(id int) (e EntryValue, err error)
	GetIDSeq(name string) (id int, seq int, err error)
	GetNames() (names []string, err error)
	GetByName(name string) (e Entry, err error)
	Create(e Entry) error
	UpdateValue(id int, seq int, ev EntryValue) error
	UpdateOptions(id int, opt EntryOptions) error
	Delete(id int) error
	DeleteByName(name string) (id int, err error)
	Clear() error
}

// EntryType defines a networktables entry type.
type EntryType int

const (
	// Boolean represents a boolean (true or false) entry type.
	Boolean EntryType = iota
	// Double represents a double (float64) entry type.
	Double
	// RawData represents a raw data (byte slice) entry type.
	RawData
	// String represents a string entry type.
	String
	// BooleanArray represents a boolean array (boolean slice) entry type.
	BooleanArray
	// DoubleArray represents a double array (float64 slice) entry type.
	DoubleArray
	// StringArray represents a string array entry type.
	StringArray
)

// EntryOptions is the options (or flags) that an entry can be annotated with.
type EntryOptions struct {
	Persist bool
}

// Entry is an all-encompassing networktables entry.
type Entry struct {
	ID             int
	SequenceNumber int
	Name           string
	Options        EntryOptions
	Value          EntryValue
}

// EntryValue represents a single networktables entry value. It only ever makes
// sense for the entry types corresponding type to be set.
type EntryValue struct {
	EntryType EntryType

	Boolean      bool
	Double       float64
	RawData      []byte
	String       string
	BooleanArray []bool
	DoubleArray  []float64
	StringArray  []string
}

type badgerDB struct {
	db *badger.DB
}

// OpenBadgerDB opens a badger DB with the given options as a networktables store.
func OpenBadgerDB(options badger.Options) (Store, error) {
	db, err := badger.Open(options)
	if err != nil {
		return nil, fmt.Errorf("unable to open badger db: %w", err)
	}

	return &badgerDB{db: db}, nil
}

const (
	badgerValueSuffix = "/value"
	badgerOptSuffix   = "/opt"
	badgerSeqSuffix   = "/seq"
	badgerNamePrefix  = "names/"
	badgerIDPrefix    = "ids/"
)

func (b *badgerDB) GetByName(name string) (Entry, error) {
	entry := Entry{Name: name}

	err := b.db.View(func(tx *badger.Txn) error {
		var err error
		entry.ID, err = getID(name, tx)
		if err != nil {
			return fmt.Errorf("couldn't get id for entry: %w", err)
		}

		entry.SequenceNumber, err = getSequenceNumber(entry.ID, tx)
		if err != nil {
			return fmt.Errorf("couldn't get entry sequence number: %w", err)
		}

		entry.Value, err = getValue(entry.ID, tx)
		if err != nil {
			return fmt.Errorf("couldn't get entry value: %w", err)
		}

		entry.Options, err = getOptions(entry.ID, tx)
		if err != nil {
			return fmt.Errorf("couldn't get entry options: %w", err)
		}

		return nil
	})
	if err != nil {
		return entry, fmt.Errorf("couldn't get entry by name: %w", err)
	}

	return entry, nil
}

func getValue(id int, tx *badger.Txn) (EntryValue, error) {
	var ev EntryValue

	item, err := tx.Get([]byte(strconv.Itoa(id) + badgerValueSuffix))
	if err != nil {
		return ev, fmt.Errorf("couldn't get raw entry value: %w", err)
	}

	err = item.Value(func(val []byte) error {
		if err := gob.NewDecoder(bytes.NewReader(val)).Decode(&ev); err != nil {
			return fmt.Errorf("couldn't decode entry value with gob: %w", err)
		}

		return nil
	})
	if err != nil {
		return ev, fmt.Errorf("couldn't get entry value: %w", err)
	}

	return ev, nil
}

func (b *badgerDB) GetValue(id int) (EntryValue, error) {
	var ev EntryValue

	err := b.db.View(func(tx *badger.Txn) error {
		var err error
		ev, err = getValue(id, tx)
		if err != nil {
			return fmt.Errorf("couldn't get entry value: %w", err)
		}

		return nil
	})
	if err != nil {
		return ev, fmt.Errorf("couldn't get value for id: %w", err)
	}

	return ev, nil
}

func getOptions(id int, tx *badger.Txn) (EntryOptions, error) {
	var opt EntryOptions

	item, err := tx.Get([]byte(strconv.Itoa(id) + badgerOptSuffix))
	if err != nil {
		return opt, fmt.Errorf("couldn't get raw entry options: %w", err)
	}

	err = item.Value(func(val []byte) error {
		if err := gob.NewDecoder(bytes.NewReader(val)).Decode(&opt); err != nil {
			return fmt.Errorf("couldn't decode entry options with gob: %w", err)
		}

		return nil
	})
	if err != nil {
		return opt, fmt.Errorf("couldn't get entry options: %w", err)
	}

	return opt, nil
}

func (b *badgerDB) GetOptions(id int) (EntryOptions, error) {
	var opt EntryOptions

	err := b.db.View(func(tx *badger.Txn) error {
		var err error
		opt, err = getOptions(id, tx)
		if err != nil {
			return fmt.Errorf("couldn't get entry options: %w", err)
		}

		return nil
	})
	if err != nil {
		return opt, fmt.Errorf("couldn't get options for id: %w", err)
	}

	return opt, nil
}

func getID(name string, tx *badger.Txn) (int, error) {
	var id int

	item, err := tx.Get([]byte(badgerNamePrefix + name))
	if err != nil {
		return 0, fmt.Errorf("couldn't get id: %w", err)
	}

	err = item.Value(func(val []byte) error {
		id, err = strconv.Atoi(string(val))
		if err != nil {
			return fmt.Errorf("couldn't parse id: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("couldn't get id value: %w", err)
	}

	return id, nil
}

func getSequenceNumber(id int, tx *badger.Txn) (int, error) {
	var seq int

	item, err := tx.Get([]byte(strconv.Itoa(id) + badgerSeqSuffix))
	if err != nil {
		return 0, fmt.Errorf("couldn't get sequence number: %w", err)
	}

	err = item.Value(func(val []byte) error {
		seq, err = strconv.Atoi(string(val))
		if err != nil {
			return fmt.Errorf("couldn't parse sequence number: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("couldn't get sequence number value: %w", err)
	}

	return seq, nil
}

func (b *badgerDB) GetID(name string) (int, error) {
	var id int

	err := b.db.View(func(tx *badger.Txn) error {
		var err error
		id, err = getID(name, tx)
		if err != nil {
			return fmt.Errorf("couldn't get id for name: %w", err)
		}
		return nil
	})
	if err != nil {
		return id, fmt.Errorf("couldn't get id for name: %w", err)
	}

	return id, nil
}

func (b *badgerDB) GetIDSeq(name string) (int, int, error) {
	var id, seq int

	err := b.db.View(func(tx *badger.Txn) error {
		var err error
		id, err = getID(name, tx)
		if err != nil {
			return fmt.Errorf("couldn't get id for name: %w", err)
		}

		seq, err = getSequenceNumber(id, tx)
		if err != nil {
			return fmt.Errorf("couldn't get sequence number for name: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("couldn't get id and sequence number for name: %w", err)
	}

	return id, seq, nil
}

func (b *badgerDB) GetNames() ([]string, error) {
	var names []string

	err := b.db.View(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := tx.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(badgerNamePrefix)); it.ValidForPrefix([]byte(badgerNamePrefix)); it.Next() {
			key := it.Item().Key()

			names = append(names, string(key[len(badgerNamePrefix):]))
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't walk names: %w", err)
	}

	return names, nil
}

func (b *badgerDB) Create(entry Entry) error {
	valueBuf := new(bytes.Buffer)
	if err := gob.NewEncoder(valueBuf).Encode(entry.Value); err != nil {
		return fmt.Errorf("couldn't encode value to buffer with gob: %w", err)
	}

	optBuf := new(bytes.Buffer)
	if err := gob.NewEncoder(optBuf).Encode(entry.Options); err != nil {
		return fmt.Errorf("couldn't encode value to buffer with gob: %w", err)
	}

	err := b.db.Update(func(tx *badger.Txn) error {
		// first we need to remove any entry with the same name

		// TODO: actually check for not found
		id, _ := getID(entry.Name, tx)
		_ = deleteEntry(id, entry.Name, tx)

		// now create the new entry

		if err := tx.Set([]byte(strconv.Itoa(entry.ID)+badgerValueSuffix), valueBuf.Bytes()); err != nil {
			return fmt.Errorf("couldn't set entry value: %w", err)
		}

		if err := tx.Set([]byte(strconv.Itoa(entry.ID)+badgerOptSuffix), optBuf.Bytes()); err != nil {
			return fmt.Errorf("couldn't set entry options: %w", err)
		}

		if err := tx.Set([]byte(strconv.Itoa(entry.ID)+badgerSeqSuffix), []byte(strconv.Itoa(entry.SequenceNumber))); err != nil {
			return fmt.Errorf("couldn't set entry sequence number: %w", err)
		}

		if err := tx.Set([]byte(badgerNamePrefix+entry.Name), []byte(strconv.Itoa(entry.ID))); err != nil {
			return fmt.Errorf("couldn't set name to id mapping: %w", err)
		}

		if err := tx.Set([]byte(badgerIDPrefix+strconv.Itoa(entry.ID)), []byte(entry.Name)); err != nil {
			return fmt.Errorf("couldn't set id to name mapping: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("couldn't create entry: %w", err)
	}

	return nil
}

func (b *badgerDB) UpdateValue(id int, seq int, ev EntryValue) error {
	valueBuf := new(bytes.Buffer)
	if err := gob.NewEncoder(valueBuf).Encode(ev); err != nil {
		return fmt.Errorf("couldn't encode value to buffer with gob: %w", err)
	}

	err := b.db.Update(func(tx *badger.Txn) error {
		if err := tx.Set([]byte(strconv.Itoa(id)+badgerValueSuffix), valueBuf.Bytes()); err != nil {
			return fmt.Errorf("couldn't set entry value: %w", err)
		}

		if err := tx.Set([]byte(strconv.Itoa(id)+badgerSeqSuffix), []byte(strconv.Itoa(seq))); err != nil {
			return fmt.Errorf("couldn't set entry sequence number: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("couldn't update entry value: %w", err)
	}

	return nil
}

func (b *badgerDB) UpdateOptions(id int, opt EntryOptions) error {
	optBuf := new(bytes.Buffer)
	if err := gob.NewEncoder(optBuf).Encode(opt); err != nil {
		return fmt.Errorf("couldn't encode value to buffer with gob: %w", err)
	}

	err := b.db.Update(func(tx *badger.Txn) error {
		if err := tx.Set([]byte(strconv.Itoa(id)+badgerOptSuffix), optBuf.Bytes()); err != nil {
			return fmt.Errorf("couldn't set entry options: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("couldn't update entry options: %w", err)
	}

	return nil
}

func (b *badgerDB) UpdateSeq(id int, seq int) error {
	err := b.db.Update(func(tx *badger.Txn) error {
		if err := tx.Set([]byte(strconv.Itoa(id)+badgerSeqSuffix), []byte(strconv.Itoa(seq))); err != nil {
			return fmt.Errorf("couldn't set entry options: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("couldn't update entry options: %w", err)
	}

	return nil
}

func getName(id int, tx *badger.Txn) (string, error) {
	item, err := tx.Get([]byte(badgerIDPrefix + strconv.Itoa(id)))
	if err != nil {
		return "", fmt.Errorf("couldn't get id to name mapping: %w", err)
	}

	var name string
	err = item.Value(func(val []byte) error {
		name = string(val)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("couldn't get id to name mapping value: %w", err)
	}

	return name, nil
}

func deleteEntry(id int, name string, tx *badger.Txn) error {
	if err := tx.Delete([]byte(strconv.Itoa(id) + badgerValueSuffix)); err != nil {
		return fmt.Errorf("couldn't delete entry value: %w", err)
	}

	if err := tx.Delete([]byte(strconv.Itoa(id) + badgerOptSuffix)); err != nil {
		return fmt.Errorf("couldn't delete entry options: %w", err)
	}

	if err := tx.Delete([]byte(strconv.Itoa(id) + badgerSeqSuffix)); err != nil {
		return fmt.Errorf("couldn't delete entry sequence number: %w", err)
	}

	if err := tx.Delete([]byte(badgerNamePrefix + name)); err != nil {
		return fmt.Errorf("couldn't delete name to id mapping: %w", err)
	}

	if err := tx.Delete([]byte(badgerIDPrefix + strconv.Itoa(id))); err != nil {
		return fmt.Errorf("couldn't delete id to name mapping: %w", err)
	}

	return nil
}

func (b *badgerDB) Delete(id int) error {
	err := b.db.Update(func(tx *badger.Txn) error {
		name, err := getName(id, tx)
		if err != nil {
			return fmt.Errorf("couldn't get entry name: %w", err)
		}

		if err := deleteEntry(id, name, tx); err != nil {
			return fmt.Errorf("couldn't delete entry: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("couldn't delete entry: %w", err)
	}

	return nil
}

func (b *badgerDB) DeleteByName(name string) (int, error) {
	var id int

	err := b.db.Update(func(tx *badger.Txn) error {
		var err error
		id, err = getID(name, tx)
		if err != nil {
			return fmt.Errorf("couldn't get entry id: %w", err)
		}

		if err := deleteEntry(id, name, tx); err != nil {
			return fmt.Errorf("couldn't delete entry: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("couldn't delete entry: %w", err)
	}

	return id, nil
}

func (b *badgerDB) Clear() error {
	err := b.db.Update(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := tx.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()

			if err := tx.Delete(key); err != nil {
				return fmt.Errorf("couldn't delete key %q: %w", key, err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("couldn't delete all keys: %w", err)
	}

	return nil
}
