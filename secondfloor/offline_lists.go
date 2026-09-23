package secondfloor

import (
	"fmt"
	"os"
)

const (
	offlineContextsTag = 0x05
	offlineURITag      = 0x01
	offlineKindTag     = 0x09
	offlineTracksTag   = 0x0d
	offlineGIDTag      = 0x01
)

type OfflineContext struct {
	URI       string
	Kind      string
	TrackGIDs [][]byte
}

func OfflineContexts(bnkPath string) ([]OfflineContext, error) {
	data, err := os.ReadFile(bnkPath)
	if err != nil {
		return nil, err
	}
	root, err := ParseBnk(data)
	if err != nil {
		return nil, err
	}
	contexts, ok := root.Field(offlineContextsTag)
	if !ok || contexts.Type != BnkList {
		return nil, fmt.Errorf("no context list in %s", bnkPath)
	}
	var out []OfflineContext
	for _, ctx := range contexts.Items {
		var oc OfflineContext
		if uri, ok := ctx.Field(offlineURITag); ok && uri.Type == BnkBytes {
			oc.URI = string(uri.Bytes)
		}
		if kind, ok := ctx.Field(offlineKindTag); ok && kind.Type == BnkBytes {
			oc.Kind = string(kind.Bytes)
		}
		if tracks, ok := ctx.Field(offlineTracksTag); ok {
			if tracks.Type != BnkList {
				return nil, fmt.Errorf("context %s track list has type %d", oc.URI, tracks.Type)
			}
			for _, track := range tracks.Items {
				gid, ok := track.Field(offlineGIDTag)
				if !ok || gid.Type != BnkBytes || len(gid.Bytes) != 16 {
					return nil, fmt.Errorf("context %s has a track entry without a 16-byte gid", oc.URI)
				}
				oc.TrackGIDs = append(oc.TrackGIDs, gid.Bytes)
			}
		}
		out = append(out, oc)
	}
	return out, nil
}
