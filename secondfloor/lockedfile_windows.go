package secondfloor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/syndtr/goleveldb/leveldb/storage"
	"golang.org/x/sys/windows"
)

const (
	holderExe          = "Spotify.exe"
	handleQueryTimeout = 250 * time.Millisecond
	fileReadData       = 0x1
)

// SYSTEM_HANDLE_TABLE_ENTRY_INFO_EX
type systemHandleEntry struct {
	Object     uintptr
	PID        uintptr
	Value      uintptr
	Access     uint32
	BackTrace  uint16
	TypeIndex  uint16
	Attributes uint32
	_          uint32
}

func isLocked(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if !isLocked(err) {
		return data, err
	}
	data, borrowErr := readViaHolder(path)
	if borrowErr != nil {
		return nil, fmt.Errorf("%s is locked and reading it through %s failed (%v), close spotify and try again: %w", path, holderExe, borrowErr, err)
	}
	return data, nil
}

func openReader(path string) (storage.Reader, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	// so spotify can still delete tables we hold open
	h, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// spotify holds index.dat open unshared, so borrow its handle
func readViaHolder(path string) ([]byte, error) {
	target, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	pids, err := processIDs(holderExe)
	if err != nil {
		return nil, err
	}
	if len(pids) == 0 {
		return nil, fmt.Errorf("%s is not running", holderExe)
	}
	handles, err := systemHandles()
	if err != nil {
		return nil, fmt.Errorf("listing handles: %w", err)
	}

	self := windows.CurrentProcess()
	procs := make(map[uintptr]windows.Handle)
	defer func() {
		for _, p := range procs {
			windows.CloseHandle(p)
		}
	}()
	var lastErr error
	for _, e := range handles {
		if !pids[e.PID] {
			continue
		}
		proc, ok := procs[e.PID]
		if !ok {
			proc, err = windows.OpenProcess(windows.PROCESS_DUP_HANDLE, false, uint32(e.PID))
			if err != nil {
				lastErr = fmt.Errorf("opening pid %d: %w", e.PID, err)
				continue
			}
			procs[e.PID] = proc
		}
		var dup windows.Handle
		if err := windows.DuplicateHandle(proc, windows.Handle(e.Value), self, &dup, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
			continue
		}
		name, ok := diskFileName(dup)
		if !ok {
			// leaked, the hung query still uses it
			if name != hungName {
				windows.CloseHandle(dup)
			}
			continue
		}
		if !strings.EqualFold(name, target) {
			windows.CloseHandle(dup)
			continue
		}
		if e.Access&fileReadData == 0 {
			windows.CloseHandle(dup)
			lastErr = errors.New("the handle is write-only")
			continue
		}
		data, err := readMapped(dup)
		windows.CloseHandle(dup)
		if err != nil {
			lastErr = err
			continue
		}
		return data, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no %s handle open on the file", holderExe)
}

func processIDs(exe string) (map[uintptr]bool, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	pids := make(map[uintptr]bool)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err := windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), exe) {
			pids[uintptr(entry.ProcessID)] = true
		}
	}
	return pids, nil
}

func systemHandles() ([]systemHandleEntry, error) {
	buf := make([]byte, 4<<20)
	for {
		var need uint32
		err := windows.NtQuerySystemInformation(int32(windows.SystemExtendedHandleInformation), unsafe.Pointer(&buf[0]), uint32(len(buf)), &need)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.STATUS_INFO_LENGTH_MISMATCH) {
			return nil, err
		}
		// the table can grow between calls
		buf = make([]byte, max(int(need), len(buf))*2)
	}
	const header = 2 * unsafe.Sizeof(uintptr(0))
	count := *(*uintptr)(unsafe.Pointer(&buf[0]))
	if header+count*unsafe.Sizeof(systemHandleEntry{}) > uintptr(len(buf)) {
		return nil, errors.New("handle table is larger than its buffer")
	}
	return unsafe.Slice((*systemHandleEntry)(unsafe.Add(unsafe.Pointer(&buf[0]), header)), count), nil
}

const hungName = "\x00hung"

// name queries can hang on some pipes
func diskFileName(h windows.Handle) (name string, ok bool) {
	type result struct {
		name string
		ok   bool
	}
	done := make(chan result, 1)
	go func() {
		if t, _ := windows.GetFileType(h); t != windows.FILE_TYPE_DISK {
			done <- result{}
			return
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil || int(n) > len(buf) {
			done <- result{}
			return
		}
		done <- result{strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`), true}
	}()
	select {
	case r := <-done:
		return r.name, r.ok
	case <-time.After(handleQueryTimeout):
		return hungName, false
	}
}

func readMapped(h windows.Handle) ([]byte, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return nil, fmt.Errorf("sizing file: %w", err)
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size == 0 {
		return nil, nil
	}
	// a mapping leaves spotify's file position alone
	mapping, err := windows.CreateFileMapping(h, nil, windows.PAGE_READONLY, 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("mapping file: %w", err)
	}
	defer windows.CloseHandle(mapping)
	addr, err := windows.MapViewOfFile(mapping, windows.FILE_MAP_READ, 0, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("mapping view: %w", err)
	}
	defer windows.UnmapViewOfFile(addr)
	// addr is outside the go heap
	view := unsafe.Slice(*(**byte)(unsafe.Pointer(&addr)), size)
	return append([]byte(nil), view...), nil
}
