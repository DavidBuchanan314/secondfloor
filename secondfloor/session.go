package secondfloor

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"

	"github.com/DavidBuchanan314/secondfloor/secondfloor/contentagnosticpb"
	"github.com/DavidBuchanan314/secondfloor/secondfloor/metadatapb"
)

type Session struct {
	Source *Source
	DB     *DB
	Index  *StorageIndex
	Lists  *OfflineLists
	Keys   map[FileID]ContentKey
	Logger *slog.Logger
}

type DownloadedTrack struct {
	URI    string
	Track  *metadatapb.Track
	File   *contentagnosticpb.AudioFile
	Record *StorageRecord
	Key    ContentKey
	HasKey bool
}

func (s *Source) Open(hmacSecret []byte) (*Session, error) {
	index, err := s.ReadStorageIndex(hmacSecret)
	if err != nil {
		return nil, err
	}
	lists, err := s.ReadOfflineLists()
	if err != nil {
		return nil, err
	}
	keys, err := s.ReadOfflineKeys(lists.DeviceID)
	if err != nil {
		return nil, err
	}
	db, err := s.OpenDB()
	if err != nil {
		return nil, err
	}
	sess := &Session{Source: s, DB: db, Index: index, Lists: lists, Keys: keys, Logger: slog.Default()}
	sess.Logger.Debug("opened source",
		"user", s.Username,
		"user_dir", s.UserDir,
		"storage_dir", s.StorageDir,
		"index_records", len(index.Records),
		"content_keys", len(keys),
		"contexts", len(lists.Contexts),
	)
	return sess, nil
}

func (sess *Session) Close() error {
	return sess.DB.Close()
}

func (sess *Session) DownloadedTracks() ([]*DownloadedTrack, error) {
	var tracks []*DownloadedTrack
	seen := make(map[string]bool)
	for _, ctx := range sess.Lists.Contexts {
		for _, gid := range ctx.TrackGIDs {
			uri := TrackURI(gid)
			if seen[uri] {
				continue
			}
			seen[uri] = true

			trait, err := sess.DB.PlaybackTrait(uri)
			if errors.Is(err, ErrNotFound) {
				sess.Logger.Debug("skipping track without playback trait", "uri", uri, "context", ctx.URI)
				continue
			}
			if err != nil {
				return nil, err
			}
			t := &DownloadedTrack{URI: uri}
			for _, file := range AudioFiles(trait) {
				if rec, ok := sess.Index.Lookup(file.GetFileId()); ok {
					t.File, t.Record = file, rec
					break
				}
			}
			if t.Record == nil {
				sess.Logger.Debug("skipping track with no stored audio file", "uri", uri, "context", ctx.URI)
				continue
			}
			t.Key, t.HasKey = sess.Keys[t.Record.ID]

			t.Track, err = sess.DB.Track(uri)
			if errors.Is(err, ErrNotFound) {
				t.Track = nil
			} else if err != nil {
				return nil, err
			}
			sess.Logger.Debug("found downloaded track",
				"uri", uri,
				"format", t.File.GetFormat(),
				"file_id", fmt.Sprintf("%x", t.Record.ID),
				"path", sess.Index.FilePath(t.Record),
				"has_key", t.HasKey,
				"has_metadata", t.Track != nil,
			)
			tracks = append(tracks, t)
		}
	}
	return tracks, nil
}

var ErrExists = errors.New("output file already exists")

func (sess *Session) Export(t *DownloadedTrack, outDir string, overwrite bool) (string, error) {
	if t.Track == nil {
		return "", errors.New("no track metadata")
	}
	if !t.HasKey {
		return "", errors.New("no content key")
	}
	ext, ok := FormatExtension(t.File.GetFormat())
	if !ok {
		return "", errors.New("unsupported format " + t.File.GetFormat().String())
	}
	dst := TrackOutputPath(outDir, t.Track, ext)
	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return dst, ErrExists
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	cover, err := sess.Index.CoverImage(t.Track.GetAlbum())
	if err != nil {
		return "", err
	}
	if cover == nil {
		sess.Logger.Debug("no cached cover", "uri", t.URI)
	} else {
		sess.Logger.Debug("using cover", "uri", t.URI, "bytes", len(cover))
	}
	err = writeFileAtomic(dst, func(w io.Writer) error {
		return sess.Index.DecryptAudio(t.Record, t.File.GetFormat(), t.Key, w)
	}, func(path string) error {
		return writeTags(path, t.Track, cover)
	})
	if err != nil {
		return "", err
	}
	return dst, nil
}
