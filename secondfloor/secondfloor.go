package secondfloor

import (
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var ErrNotFound = leveldb.ErrNotFound

type DB struct {
	ldb *leveldb.DB
}

func OpenDB(dbPath string) (*DB, error) {
	ldb, err := leveldb.OpenFile(dbPath, &opt.Options{
		ReadOnly:       true,
		ErrorIfMissing: true,
		Comparer:       GreenbaseComparer{},
	})
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, fmt.Errorf("%s is locked (is spotify running?): %w", dbPath, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dbPath, err)
	}
	return &DB{ldb: ldb}, nil
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
