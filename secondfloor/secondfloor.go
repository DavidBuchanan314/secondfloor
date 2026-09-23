package secondfloor

import (
	"encoding/binary"

	"github.com/DavidBuchanan314/secondfloor/secondfloor/collectionpb"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"google.golang.org/protobuf/proto"
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
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return keys, nil
}

func (db *DB) CollectionTrack(trackURI string) (*collectionpb.CollectionTrackEntry, error) {
	value, err := db.ldb.Get(GreenbaseKey("!col#col.albtrk#", []byte(trackURI)), nil)
	if err != nil {
		return nil, err
	}
	entry := &collectionpb.CollectionTrackEntry{}
	if err := proto.Unmarshal(value, entry); err != nil {
		return nil, err
	}
	return entry, nil
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
