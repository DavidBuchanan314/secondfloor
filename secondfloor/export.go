package secondfloor

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/DavidBuchanan314/secondfloor/secondfloor/collectionpb"
)

func SanitizePathComponent(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case strings.ContainsRune(`<>:"/\|?*`, r), unicode.IsControl(r):
			return -1
		case unicode.IsSpace(r):
			return '_'
		}
		return r
	}, s)
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	return s
}

func TrackOutputPath(outDir string, track *collectionpb.CollectionTrackEntry) string {
	artist := ""
	if names := track.GetArtistName(); len(names) > 0 {
		artist = names[0]
	}
	name := fmt.Sprintf("%02d_%s.flac", track.GetTrackNumber(), track.GetTrackName())
	return filepath.Join(
		outDir,
		SanitizePathComponent(artist),
		SanitizePathComponent(track.GetAlbumName()),
		SanitizePathComponent(name),
	)
}

func (idx *StorageIndex) DecryptFile(rec *StorageRecord, contentKey ContentKey, audioIV []byte, dstPath string) error {
	src, err := os.Open(idx.FilePath(rec))
	if err != nil {
		return err
	}
	defer src.Close()

	storageBlock, err := aes.NewCipher(idx.Key[:])
	if err != nil {
		return err
	}
	contentBlock, err := aes.NewCipher(contentKey[:])
	if err != nil {
		return err
	}
	var r io.Reader = io.LimitReader(src, int64(rec.ContentLength))
	r = cipher.StreamReader{S: cipher.NewCTR(storageBlock, rec.StorageIV()), R: r}
	r = cipher.StreamReader{S: cipher.NewCTR(contentBlock, audioIV), R: r}

	return writeFileAtomic(dstPath, func(w io.Writer) error {
		n, err := io.Copy(w, r)
		if err != nil {
			return err
		}
		if n != int64(rec.ContentLength) {
			return fmt.Errorf("%s: read %d of %d content bytes", idx.FilePath(rec), n, rec.ContentLength)
		}
		return nil
	})
}

func writeFileAtomic(dstPath string, write func(io.Writer) error) (err error) {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dstPath), ".secondfloor-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if err = write(tmp); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dstPath)
}
