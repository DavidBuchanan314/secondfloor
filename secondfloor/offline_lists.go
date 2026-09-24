package secondfloor

import (
	"encoding/hex"
	"fmt"
	"os"
)

const (
	offlineContextsField = 0
	offlineURIField      = 0
	offlineKindField     = 1
	offlineTracksField   = 2
	offlineGIDField      = 0
	offlineStateField    = 3
	offlineDeviceField   = 0
	offlineDeviceIDField = 4
)

type OfflineLists struct {
	Contexts []OfflineContext
	DeviceID string
}

type OfflineContext struct {
	URI       string
	Kind      string
	TrackGIDs [][]byte
}

func ReadOfflineLists(bnkPath string) (*OfflineLists, error) {
	data, err := os.ReadFile(bnkPath)
	if err != nil {
		return nil, err
	}
	root, err := ParseBnk(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", bnkPath, err)
	}
	out := &OfflineLists{}
	for _, ctx := range root.Repeated(offlineContextsField) {
		if ctx.Type != BnkStruct {
			return nil, fmt.Errorf("%s: offline context has type %d", bnkPath, ctx.Type)
		}
		var oc OfflineContext
		if uri, ok := ctx.Field(offlineURIField); ok && uri.Type == BnkBytes {
			oc.URI = string(uri.Bytes)
		}
		if kind, ok := ctx.Field(offlineKindField); ok && kind.Type == BnkBytes {
			oc.Kind = string(kind.Bytes)
		}
		for _, track := range ctx.Repeated(offlineTracksField) {
			gid, ok := track.Field(offlineGIDField)
			if !ok || gid.Type != BnkBytes || len(gid.Bytes) != 16 {
				return nil, fmt.Errorf("%s: context %s has a track entry without a 16-byte gid", bnkPath, oc.URI)
			}
			oc.TrackGIDs = append(oc.TrackGIDs, gid.Bytes)
		}
		out.Contexts = append(out.Contexts, oc)
	}
	if state, ok := root.Field(offlineStateField); ok {
		device, _ := state.Field(offlineDeviceField)
		if id, ok := device.Field(offlineDeviceIDField); ok && id.Type == BnkBytes {
			decoded, err := hex.DecodeString(string(id.Bytes))
			if err != nil {
				return nil, fmt.Errorf("%s: device id: %w", bnkPath, err)
			}
			out.DeviceID = string(decoded)
		}
	}
	return out, nil
}
