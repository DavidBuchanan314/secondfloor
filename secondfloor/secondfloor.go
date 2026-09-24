package secondfloor

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var ErrNotFound = leveldb.ErrNotFound

type DB struct {
	ldb *leveldb.DB
}

// openDBAttempts covers concurrent writers modifying or compacting the
// database while we open it, which can leave us with a torn read or a file
// that was just removed.
const openDBAttempts = 3

func OpenDB(dbPath string) (*DB, error) {
	var err error
	for attempt := range openDBAttempts {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		var ldb *leveldb.DB
		ldb, err = leveldb.Open(&readOnlyStorage{path: dbPath}, &opt.Options{
			ReadOnly:       true,
			ErrorIfMissing: true,
			Comparer:       GreenbaseComparer{},
		})
		if err == nil {
			return &DB{ldb: ldb}, nil
		}
	}
	return nil, fmt.Errorf("%s: %w", dbPath, err)
}

func (db *DB) Close() error {
	return db.ldb.Close()
}

func (db *DB) ListKeys() ([][]byte, error) {
	iter := db.ldb.NewIterator(nil, nil)
	defer iter.Release()

	var keys [][]byte
	for iter.Next() {
		keys = append(keys, append([]byte(nil), iter.Key()...))
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterating keys: %w", err)
	}
	return keys, nil
}

func GreenbaseKey(prefix string, tokens ...[]byte) []byte {
	key := []byte(prefix)
	for _, token := range tokens {
		key = binary.AppendUvarint(key, uint64(len(token)))
		key = append(key, token...)
		key = append(key, '#')
	}
	return key
}
