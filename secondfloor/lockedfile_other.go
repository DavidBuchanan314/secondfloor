//go:build !windows

package secondfloor

import (
	"os"

	"github.com/syndtr/goleveldb/leveldb/storage"
)

func isLocked(error) bool {
	return false
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func openReader(path string) (storage.Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}
