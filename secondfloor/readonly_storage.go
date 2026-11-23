package secondfloor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/syndtr/goleveldb/leveldb/journal"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/table"
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
	f, err := openReader(filepath.Join(s.path, fd.String()))
	if errors.Is(err, fs.ErrNotExist) && fd.Type == storage.TypeTable {
		f, err = openReader(filepath.Join(s.path, fmt.Sprintf("%06d.sst", fd.Num)))
	}
	if isLocked(err) && fd.Type == storage.TypeManifest {
		manifest, rebuildErr := s.manifestFromTables()
		if rebuildErr != nil {
			return nil, fmt.Errorf("%w (rebuilding it from the tables failed: %v)", err, rebuildErr)
		}
		slog.Debug("manifest is locked, using one rebuilt from the tables", "dir", s.path)
		return memReader{bytes.NewReader(manifest)}, nil
	}
	return f, err
}

type memReader struct{ *bytes.Reader }

func (memReader) Close() error { return nil }

// leveldb version edit tags
const (
	tagComparator     = 1
	tagLogNumber      = 2
	tagNextFileNumber = 3
	tagLastSequence   = 4
	tagNewFile        = 7

	maxSequence = 1<<56 - 1
)

// all tables go at level 0, where they may overlap and lookups take the newest entry
func (s *readOnlyStorage) manifestFromTables() ([]byte, error) {
	fds, err := s.List(storage.TypeAll)
	if err != nil {
		return nil, err
	}
	var edit []byte
	put := func(xs ...uint64) {
		for _, x := range xs {
			edit = binary.AppendUvarint(edit, x)
		}
	}
	putBytes := func(b []byte) {
		put(uint64(len(b)))
		edit = append(edit, b...)
	}

	put(tagComparator)
	putBytes([]byte(GreenbaseComparer{}.Name()))
	var maxNum int64
	for _, fd := range fds {
		maxNum = max(maxNum, fd.Num)
		if fd.Type != storage.TypeTable {
			continue
		}
		smallest, largest, size, err := s.tableBounds(fd)
		if errors.Is(err, fs.ErrNotExist) {
			continue // compacted away since listing
		}
		if err != nil {
			return nil, fmt.Errorf("table %d: %w", fd.Num, err)
		}
		if smallest == nil {
			continue
		}
		put(tagNewFile, 0, uint64(fd.Num), uint64(size))
		putBytes(smallest)
		putBytes(largest)
	}
	// skip every journal, spotify holds the live one write-only too
	put(tagLogNumber, uint64(maxNum+1), tagNextFileNumber, uint64(maxNum+2), tagLastSequence, maxSequence)

	var buf bytes.Buffer
	jw := journal.NewWriter(&buf)
	w, err := jw.Next()
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(edit); err != nil {
		return nil, err
	}
	if err := jw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *readOnlyStorage) tableBounds(fd storage.FileDesc) (smallest, largest []byte, size int64, err error) {
	r, err := s.Open(fd)
	if err != nil {
		return nil, nil, 0, err
	}
	defer r.Close()
	size, err = r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, nil, 0, err
	}
	tr, err := table.NewReader(r, size, fd, nil, nil, &opt.Options{})
	if err != nil {
		return nil, nil, 0, err
	}
	defer tr.Release()
	it := tr.NewIterator(nil, nil)
	defer it.Release()
	if it.First() {
		smallest = bytes.Clone(it.Key())
	}
	if it.Last() {
		largest = bytes.Clone(it.Key())
	}
	return smallest, largest, size, it.Error()
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
