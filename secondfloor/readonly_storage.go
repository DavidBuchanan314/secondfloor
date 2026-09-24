package secondfloor

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/syndtr/goleveldb/leveldb/storage"
)

// readOnlyStorage is a leveldb storage that never takes the LOCK file, so the
// database can be read alongside concurrent writers.
type readOnlyStorage struct {
	path string
}

var errReadOnlyStorage = errors.New("leveldb storage is read-only")

type noopLocker struct{}

func (noopLocker) Unlock() {}

func (s *readOnlyStorage) Lock() (storage.Locker, error) {
	return noopLocker{}, nil
}

func (s *readOnlyStorage) Log(str string) {
	slog.Debug("leveldb", "msg", str)
}

func (s *readOnlyStorage) GetMeta() (storage.FileDesc, error) {
	current, err := os.ReadFile(filepath.Join(s.path, "CURRENT"))
	if err != nil {
		return storage.FileDesc{}, err
	}
	name, ok := strings.CutSuffix(string(current), "\n")
	if fd, valid := parseLevelDBName(name); ok && valid && fd.Type == storage.TypeManifest {
		return fd, nil
	}
	return storage.FileDesc{}, &storage.ErrCorrupted{Err: fmt.Errorf("CURRENT has bad content %q", current)}
}

func (s *readOnlyStorage) List(ft storage.FileType) ([]storage.FileDesc, error) {
	entries, err := os.ReadDir(s.path)
	if err != nil {
		return nil, err
	}
	var fds []storage.FileDesc
	for _, e := range entries {
		if fd, ok := parseLevelDBName(e.Name()); ok && fd.Type&ft != 0 {
			fds = append(fds, fd)
		}
	}
	return fds, nil
}

func (s *readOnlyStorage) Open(fd storage.FileDesc) (storage.Reader, error) {
	if !storage.FileDescOk(fd) {
		return nil, storage.ErrInvalidFile
	}
	f, err := os.Open(filepath.Join(s.path, fd.String()))
	if errors.Is(err, fs.ErrNotExist) && fd.Type == storage.TypeTable {
		f, err = os.Open(filepath.Join(s.path, fmt.Sprintf("%06d.sst", fd.Num)))
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *readOnlyStorage) SetMeta(storage.FileDesc) error {
	return errReadOnlyStorage
}

func (s *readOnlyStorage) Create(storage.FileDesc) (storage.Writer, error) {
	return nil, errReadOnlyStorage
}

func (s *readOnlyStorage) Remove(storage.FileDesc) error {
	return errReadOnlyStorage
}

func (s *readOnlyStorage) Rename(storage.FileDesc, storage.FileDesc) error {
	return errReadOnlyStorage
}

func (s *readOnlyStorage) Close() error {
	return nil
}

func parseLevelDBName(name string) (storage.FileDesc, bool) {
	var fd storage.FileDesc
	var tail string
	if _, err := fmt.Sscanf(name, "%d.%s", &fd.Num, &tail); err == nil {
		switch tail {
		case "log":
			fd.Type = storage.TypeJournal
		case "ldb", "sst":
			fd.Type = storage.TypeTable
		default:
			return fd, false
		}
		return fd, true
	}
	if n, _ := fmt.Sscanf(name, "MANIFEST-%d%s", &fd.Num, &tail); n == 1 {
		fd.Type = storage.TypeManifest
		return fd, true
	}
	return fd, false
}
