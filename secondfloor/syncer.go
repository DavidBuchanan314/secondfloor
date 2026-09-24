package secondfloor

import (
	"errors"
	"log/slog"
)

type Syncer struct {
	Source       *Source
	HMACSecret   []byte
	OutDir       string
	Overwrite    bool
	FetchArtwork bool
	Logger       *slog.Logger

	synced bool
	done   map[string]FileID
	warned map[string]bool
}

type SyncResult struct {
	Exported int
	Failed   int
}

func (s *Syncer) Sync() (SyncResult, error) {
	var result SyncResult
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if s.done == nil {
		s.done = make(map[string]FileID)
		s.warned = make(map[string]bool)
	}
	overwrite := s.Overwrite && !s.synced

	sess, err := s.Source.Open(s.HMACSecret)
	if err != nil {
		return result, err
	}
	defer sess.Close()
	sess.Logger = logger
	sess.FetchArtwork = s.FetchArtwork

	seen := make(map[string]struct{})
	for _, ctx := range sess.Lists.Contexts {
		for _, gid := range ctx.TrackGIDs {
			key := string(gid)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			if fileID, ok := s.done[key]; ok && !overwrite {
				if _, stillDownloaded := sess.Keys[fileID]; stillDownloaded {
					continue
				}
			}

			t, err := sess.LookupTrack(gid, ctx.URI)
			if err != nil {
				return result, err
			}
			if t == nil {
				continue
			}
			dst, err := sess.Export(t, s.OutDir, overwrite)
			switch {
			case errors.Is(err, ErrExists):
				logger.Debug("already exported", "uri", t.URI, "path", dst)
			case err != nil:
				result.Failed++
				if s.warned[key] {
					logger.Debug("skipping track", "uri", t.URI, "err", err)
				} else {
					logger.Warn("skipping track", "uri", t.URI, "err", err)
					s.warned[key] = true
				}
				continue
			default:
				result.Exported++
				logger.Info("exported", "uri", t.URI, "path", dst)
			}
			s.done[key] = t.Record.ID
			delete(s.warned, key)
		}
	}
	s.synced = true
	return result, nil
}
