package secondfloor

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"os"

	"github.com/DavidBuchanan314/secondfloor/secondfloor/contentagnosticpb"
	"github.com/DavidBuchanan314/secondfloor/secondfloor/metadatapb"
)

type Session struct {
	Source       *Source
	Index        *StorageIndex
	Lists        *OfflineLists
	Keys         map[FileID]ContentKey
	Logger       *slog.Logger
	FetchArtwork bool
	db           *DB

	lastCoverID string
	lastCover   []byte
}

type DownloadedTrack struct {
	GID    []byte
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
	sess := &Session{Source: s, Index: index, Lists: lists, Keys: keys, Logger: slog.Default()}
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

func (sess *Session) DB() (*DB, error) {
	if sess.db == nil {
		db, err := sess.Source.OpenDB()
		if err != nil {
			return nil, err
		}
		sess.db = db
	}
	return sess.db, nil
}

func (sess *Session) Close() error {
	if sess.db == nil {
		return nil
	}
	return sess.db.Close()
}

func (sess *Session) DownloadedTracks() iter.Seq2[*DownloadedTrack, error] {
	return func(yield func(*DownloadedTrack, error) bool) {
		seen := make(map[string]struct{})
		for _, ctx := range sess.Lists.Contexts {
			for _, gid := range ctx.TrackGIDs {
				if _, dup := seen[string(gid)]; dup {
					continue
				}
				seen[string(gid)] = struct{}{}
				t, err := sess.LookupTrack(gid, ctx.URI)
				if err != nil {
					yield(nil, err)
					return
				}
				if t != nil && !yield(t, nil) {
					return
				}
			}
		}
	}
}

func (sess *Session) LookupTrack(gid []byte, contextURI string) (*DownloadedTrack, error) {
	db, err := sess.DB()
	if err != nil {
		return nil, err
	}
	uri := TrackURI(gid)
	trait, err := db.PlaybackTrait(uri)
	if errors.Is(err, ErrNotFound) {
		sess.Logger.Debug("skipping track without playback trait", "uri", uri, "context", contextURI)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t := &DownloadedTrack{GID: gid, URI: uri}
	for _, file := range AudioFiles(trait) {
		if rec, ok := sess.Index.Lookup(file.GetFileId(), RealmAudio); ok {
			t.File, t.Record = file, rec
			break
		}
	}
	if t.Record == nil {
		sess.Logger.Debug("skipping track with no stored audio file", "uri", uri, "context", contextURI)
		return nil, nil
	}
	t.Key, t.HasKey = sess.Keys[t.Record.ID]

	t.Track, err = db.Track(uri)
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
	return t, nil
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
	cover, err := sess.cover(t.Track.GetAlbum())
	if err != nil {
		return "", err
	}
	if cover == nil {
		sess.Logger.Debug("no cover", "uri", t.URI)
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

func (sess *Session) cover(album *metadatapb.Album) ([]byte, error) {
	cached, err := sess.Index.CachedCover(album)
	if err != nil || cached != nil {
		return cached, err
	}
	images := coverImages(album)
	if !sess.FetchArtwork || len(images) == 0 {
		return nil, nil
	}
	id := images[0].GetFileId()
	if string(id) == sess.lastCoverID {
		return sess.lastCover, nil
	}
	data, err := DownloadCover(id)
	if err != nil {
		sess.Logger.Warn("could not download cover", "album", album.GetName(), "err", err)
		data = nil
	} else {
		sess.Logger.Debug("downloaded cover", "album", album.GetName(), "image", fmt.Sprintf("%x", id), "bytes", len(data))
	}
	sess.lastCoverID, sess.lastCover = string(id), data
	return data, nil
}
