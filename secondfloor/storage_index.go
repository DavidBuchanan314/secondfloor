package secondfloor

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/adler32"
	"os"
	"path/filepath"
	"time"
)

const (
	storageHeaderSize = 0x200
	storageRecordSize = 0x40
)

var (
	storageMagic    = []byte{0x11, 0x6f, 0x34, 0xd4, 0xa6, 0xd0, 0x35, 0x8b, 0x3c, 0xf7, 0x69, 0xa2, 0x59, 0xa9, 0xeb, 0xcc}
	storageMagicOld = []byte{0x11, 0x6f, 0x34, 0xd4, 0xa6, 0xd0, 0x35, 0x8b, 0x3c, 0xf7, 0x69, 0xa2, 0x59, 0xa9, 0xeb, 0xcb}
	storageTag      = []byte{0xde, 0xad, 0xbe, 0xef}
)

type FileID [20]byte

type StorageIndex struct {
	CacheID      [16]byte
	Salt         [4]byte
	Version      uint32
	CreateTime   time.Time
	UserHash     [20]byte
	LastSaveTime time.Time
	Key          [16]byte
	NamePrefix   [20]byte
	Records      []*StorageRecord
	ByID         map[FileID]*StorageRecord
	FileDirs     []string
}

type StorageRecord struct {
	Slot           int
	ID             FileID
	Realm          byte
	Salt           uint16
	Version        uint32
	ContentLength  uint32
	CreateTime     time.Time
	Size           uint32
	State          uint32
	LastAccessTime time.Time
}

func ReadStorageIndex(storageDir string, hmacSecret []byte) (*StorageIndex, error) {
	data, err := os.ReadFile(filepath.Join(storageDir, "index.dat"))
	if err != nil {
		return nil, err
	}
	if len(data) < storageHeaderSize || (len(data)-storageHeaderSize)%storageRecordSize != 0 {
		return nil, fmt.Errorf("index.dat has invalid size %d", len(data))
	}
	magic := data[0x00:0x10]
	if !bytes.Equal(magic, storageMagic) && !bytes.Equal(magic, storageMagicOld) {
		return nil, fmt.Errorf("index.dat has bad magic %x", magic)
	}

	idx := &StorageIndex{
		Version:      binary.BigEndian.Uint32(data[0x28:]),
		CreateTime:   beTime(data[0x100:]),
		LastSaveTime: beTime(data[0x118:]),
		ByID:         make(map[FileID]*StorageRecord),
		FileDirs:     []string{storageDir},
	}
	copy(idx.CacheID[:], data[0x10:0x20])
	copy(idx.Salt[:], data[0x20:0x24])
	copy(idx.UserHash[:], data[0x104:0x118])
	if idx.Version != 3 {
		return nil, fmt.Errorf("index.dat has unsupported version %d", idx.Version)
	}

	mac := hmac.New(sha1.New, hmacSecret)
	mac.Write(idx.CacheID[:])
	mac.Write(idx.Salt[:])
	k := sha1.Sum(mac.Sum(nil)[:16])
	copy(idx.Key[:], k[:16])
	idx.NamePrefix = sha1.Sum(append([]byte{0x01}, k[:]...))

	block, err := aes.NewCipher(idx.Key[:])
	if err != nil {
		return nil, err
	}
	for slot := 0; storageHeaderSize+slot*storageRecordSize < len(data); slot++ {
		off := storageHeaderSize + slot*storageRecordSize
		rec, err := decryptStorageRecord(block, slot, data[off:off+storageRecordSize])
		if err != nil {
			return nil, fmt.Errorf("index.dat record %d: %w", slot, err)
		}
		if _, dup := idx.ByID[rec.ID]; dup {
			return nil, fmt.Errorf("index.dat record %d: duplicate id %x", slot, rec.ID)
		}
		idx.Records = append(idx.Records, rec)
		idx.ByID[rec.ID] = rec
	}
	return idx, nil
}

func decryptStorageRecord(block cipher.Block, slot int, ciphertext []byte) (*StorageRecord, error) {
	var slotBytes [4]byte
	binary.BigEndian.PutUint32(slotBytes[:], uint32(slot))
	iv := sha1.Sum(slotBytes[:])
	plain := make([]byte, storageRecordSize)
	cipher.NewCBCDecrypter(block, iv[:aes.BlockSize]).CryptBlocks(plain, ciphertext)

	if !bytes.Equal(plain[0x04:0x08], storageTag) {
		return nil, errors.New("bad tag")
	}
	if sum := binary.BigEndian.Uint32(plain[0x00:]); sum != adler32.Checksum(plain[0x04:]) {
		return nil, errors.New("bad checksum")
	}

	rec := &StorageRecord{
		Slot:           slot,
		Realm:          plain[0x1c],
		Salt:           binary.BigEndian.Uint16(plain[0x1e:]),
		Version:        binary.BigEndian.Uint32(plain[0x20:]),
		ContentLength:  binary.BigEndian.Uint32(plain[0x24:]),
		CreateTime:     beTime(plain[0x28:]),
		Size:           binary.BigEndian.Uint32(plain[0x2c:]),
		State:          binary.BigEndian.Uint32(plain[0x30:]),
		LastAccessTime: beTime(plain[0x34:]),
	}
	copy(rec.ID[:], plain[0x08:0x1c])
	return rec, nil
}

func (idx *StorageIndex) Lookup(fileID []byte) (*StorageRecord, bool) {
	if len(fileID) != len(FileID{}) {
		return nil, false
	}
	rec, ok := idx.ByID[FileID(fileID)]
	return rec, ok
}

func (rec *StorageRecord) StorageIV() []byte {
	iv := make([]byte, 16)
	copy(iv, rec.ID[:16])
	iv[0] ^= rec.Realm
	iv[2] ^= byte(rec.Salt >> 8)
	iv[3] ^= byte(rec.Salt)
	return iv
}

func (idx *StorageIndex) FileName(rec *StorageRecord) string {
	h := sha1.New()
	h.Write(idx.NamePrefix[:])
	h.Write(rec.ID[:])
	h.Write([]byte{rec.Realm, 0x00, byte(rec.Salt), byte(rec.Salt >> 8)})
	return hex.EncodeToString(h.Sum(nil))
}

func (idx *StorageIndex) FilePath(rec *StorageRecord) string {
	name := idx.FileName(rec)
	rel := filepath.Join(name[:2], name+".file")
	for _, dir := range idx.FileDirs {
		path := filepath.Join(dir, rel)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return filepath.Join(idx.FileDirs[0], rel)
}

func beTime(b []byte) time.Time {
	t := binary.BigEndian.Uint32(b)
	if t == 0 {
		return time.Time{}
	}
	return time.Unix(int64(t), 0)
}
