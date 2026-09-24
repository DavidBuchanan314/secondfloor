package secondfloor

import (
	"fmt"
	"strings"

	"github.com/DavidBuchanan314/secondfloor/secondfloor/contentagnosticpb"
	"github.com/DavidBuchanan314/secondfloor/secondfloor/metadatapb"
	"github.com/DavidBuchanan314/secondfloor/secondfloor/xmetapb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	trackExtension         = []byte{0x2a}
	playbackTraitExtension = []byte{0xe0, 0xd4}
)

func (db *DB) xmetaCache(extension []byte, uri string, m proto.Message) error {
	value, err := db.ldb.Get(GreenbaseKey("!xmeta#cache#", extension, []byte(uri)), nil)
	if err != nil {
		return fmt.Errorf("reading xmeta cache entry: %w", err)
	}
	entry := &xmetapb.CacheEntry{}
	if err := proto.Unmarshal(value, entry); err != nil {
		return fmt.Errorf("decoding xmeta cache entry: %w", err)
	}
	if err := unmarshalAny(entry.GetValue(), m); err != nil {
		return fmt.Errorf("decoding xmeta cache value: %w", err)
	}
	return nil
}

func unmarshalAny(a *anypb.Any, m proto.Message) error {
	name := string(m.ProtoReflect().Descriptor().FullName())
	want := "spotify." + strings.TrimPrefix(name, "secondfloor.")
	url := a.GetTypeUrl()
	if got := url[strings.LastIndexByte(url, '/')+1:]; got != want {
		return fmt.Errorf("any has type %q, want %q", got, want)
	}
	return proto.Unmarshal(a.GetValue(), m)
}

func (db *DB) PlaybackTrait(trackURI string) (*contentagnosticpb.PlaybackTrait, error) {
	trait := &contentagnosticpb.PlaybackTrait{}
	if err := db.xmetaCache(playbackTraitExtension, trackURI, trait); err != nil {
		return nil, fmt.Errorf("playback trait: %w", err)
	}
	return trait, nil
}

func (db *DB) Track(trackURI string) (*metadatapb.Track, error) {
	track := &metadatapb.Track{}
	if err := db.xmetaCache(trackExtension, trackURI, track); err != nil {
		return nil, fmt.Errorf("track metadata: %w", err)
	}
	return track, nil
}

func AudioFiles(trait *contentagnosticpb.PlaybackTrait) []*contentagnosticpb.AudioFile {
	var files []*contentagnosticpb.AudioFile
	for _, entry := range trait.GetAudio() {
		files = append(files, entry.GetValue().GetHosted().GetFiles()...)
	}
	return files
}
