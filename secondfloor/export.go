package secondfloor

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DavidBuchanan314/secondfloor/secondfloor/contentagnosticpb"
	"github.com/DavidBuchanan314/secondfloor/secondfloor/metadatapb"
)

// room for the temp prefix and extension under 255
const maxComponentBytes = 200

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
	for len(s) > maxComponentBytes {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "_"
	}
	if isWindowsDeviceName(s) {
		return "_" + s
	}
	return s
}

func isWindowsDeviceName(s string) bool {
	stem, _, _ := strings.Cut(s, ".")
	stem = strings.ToUpper(stem)
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(stem) == 4 && (stem[:3] == "COM" || stem[:3] == "LPT") && stem[3] >= '1' && stem[3] <= '9'
}

func FormatExtension(format contentagnosticpb.Format) (string, bool) {
	name := format.String()
	switch {
	case strings.HasPrefix(name, "OGG_VORBIS_"):
		return ".ogg", true
	case strings.HasPrefix(name, "FLAC_FLAC"):
		return ".flac", true
	}
	return "", false
}

func TrackOutputPath(outDir string, track *metadatapb.Track, ext string) string {
	artists := track.GetAlbum().GetArtist()
	if len(artists) == 0 {
		artists = track.GetArtist()
	}
	var names []string
	for _, a := range artists {
		names = append(names, a.GetName())
	}
	number := fmt.Sprintf("%02d", track.GetNumber())
	if disc := track.GetDiscNumber(); disc > 1 {
		number = fmt.Sprintf("%d-%s", disc, number)
	}
	return filepath.Join(
		outDir,
		SanitizePathComponent(strings.Join(names, ", ")),
		SanitizePathComponent(track.GetAlbum().GetName()),
		SanitizePathComponent(number+"_"+track.GetName())+ext,
	)
}

var audioIV = []byte{0x72, 0xe0, 0x67, 0xfb, 0xdd, 0xcb, 0xcf, 0x77, 0xeb, 0xe8, 0xbc, 0x64, 0x3f, 0x63, 0x0d, 0x93}

func (idx *StorageIndex) DecryptAudio(rec *StorageRecord, format contentagnosticpb.Format, contentKey ContentKey, w io.Writer) error {
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
	limited := &io.LimitedReader{R: src, N: int64(rec.ContentLength)}
	var r io.Reader = cipher.StreamReader{S: cipher.NewCTR(storageBlock, rec.StorageIV()), R: limited}
	r = cipher.StreamReader{S: cipher.NewCTR(contentBlock, audioIV), R: r}

	if ext, _ := FormatExtension(format); ext == ".ogg" {
		br := bufio.NewReader(r)
		if err := skipSpotifyOggPage(br); err != nil {
			return fmt.Errorf("%s: %w", idx.FilePath(rec), err)
		}
		r = br
	}

	if _, err := io.Copy(w, r); err != nil {
		return fmt.Errorf("decrypting %s: %w", idx.FilePath(rec), err)
	}
	if limited.N != 0 {
		return fmt.Errorf("%s: missing %d of %d content bytes", idx.FilePath(rec), limited.N, rec.ContentLength)
	}
	return nil
}

func (idx *StorageIndex) ReadPlainFile(rec *StorageRecord) ([]byte, error) {
	f, err := os.Open(idx.FilePath(rec))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data := make([]byte, rec.ContentLength)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, fmt.Errorf("%s: %w", idx.FilePath(rec), err)
	}
	return data, nil
}

func skipSpotifyOggPage(r *bufio.Reader) error {
	const headerSize = 27
	header, err := r.Peek(headerSize)
	if err != nil {
		return fmt.Errorf("reading first ogg page header: %w", err)
	}
	if string(header[:4]) != "OggS" || header[5]&0x06 != 0x06 {
		return errors.New("first ogg page is not a spotify header page")
	}
	segments := int(header[26])
	full, err := r.Peek(headerSize + segments)
	if err != nil {
		return fmt.Errorf("reading first ogg page segment table: %w", err)
	}
	pageSize := headerSize + segments
	for _, n := range full[headerSize:] {
		pageSize += int(n)
	}
	if _, err := r.Discard(pageSize); err != nil {
		return fmt.Errorf("skipping spotify header page: %w", err)
	}
	if next, err := r.Peek(4); err != nil || string(next) != "OggS" {
		return errors.New("no ogg page after spotify header page")
	}
	return nil
}

func writeFileAtomic(dstPath string, write func(io.Writer) error, finalize func(path string) error) (err error) {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	tmp, err := createTemp(filepath.Dir(dstPath), filepath.Ext(dstPath))
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
	if err = finalize(tmp.Name()); err != nil {
		return fmt.Errorf("finalizing %s: %w", tmp.Name(), err)
	}
	return os.Rename(tmp.Name(), dstPath)
}

const tempPrefix = ".secondfloor-"

func createTemp(dir, ext string) (*os.File, error) {
	for {
		name := filepath.Join(dir, tempPrefix+rand.Text()+ext)
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, err
	}
}

func removeTemps(outDir string, logger *slog.Logger) {
	filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() || !strings.HasPrefix(d.Name(), tempPrefix) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			logger.Warn("removing partial export", "path", path, "err", err)
			return nil
		}
		logger.Info("removed partial export", "path", path)
		return nil
	})
}
