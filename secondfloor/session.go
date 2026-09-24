package secondfloor

import (
	"errors"

	"github.com/DavidBuchanan314/secondfloor/secondfloor/collectionpb"
	"github.com/DavidBuchanan314/secondfloor/secondfloor/contentagnosticpb"
)

type Session struct {
	Source *Source
	DB     *DB
	Index  *StorageIndex
	Lists  *OfflineLists
	Keys   map[FileID]ContentKey
}

type DownloadedTrack struct {
	URI    string
	Track  *collectionpb.CollectionTrackEntry
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
	return &Session{Source: s, DB: db, Index: index, Lists: lists, Keys: keys}, nil
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
				continue
			}
			t.Key, t.HasKey = sess.Keys[t.Record.ID]

			t.Track, err = sess.DB.CollectionTrack(uri)
			if errors.Is(err, ErrNotFound) {
				t.Track = nil
			} else if err != nil {
				return nil, err
			}
			tracks = append(tracks, t)
		}
	}
	return tracks, nil
}

func (sess *Session) Export(t *DownloadedTrack, outDir string) (string, error) {
	if t.Track == nil {
		return "", errors.New("no collection metadata")
	}
	if !t.HasKey {
		return "", errors.New("no content key")
	}
	ext, ok := FormatExtension(t.File.GetFormat())
	if !ok {
		return "", errors.New("unsupported format " + t.File.GetFormat().String())
	}
	dst := TrackOutputPath(outDir, t.Track, ext)
	if err := sess.Index.DecryptFile(t.Record, t.File.GetFormat(), t.Key, dst); err != nil {
		return "", err
	}
	return dst, nil
}
