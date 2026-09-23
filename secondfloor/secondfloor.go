package secondfloor

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func ListKeys(dbPath string) ([][]byte, error) {
	db, err := leveldb.OpenFile(dbPath, &opt.Options{
		ReadOnly:       true,
		ErrorIfMissing: true,
		Comparer:       GreenbaseComparer{},
	})
	if err != nil {
		return nil, err
	}
	defer db.Close()

	iter := db.NewIterator(nil, nil)
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
