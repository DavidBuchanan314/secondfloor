package secondfloor

import (
	"fmt"
	"strings"

	"github.com/DavidBuchanan314/secondfloor/secondfloor/contentagnosticpb"
	"github.com/DavidBuchanan314/secondfloor/secondfloor/xmetapb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var playbackTraitExtension = []byte{0xe0, 0xd4}

func (db *DB) xmetaCache(extension []byte, uri string, m proto.Message) error {
	value, err := db.ldb.Get(GreenbaseKey("!xmeta#cache#", extension, []byte(uri)), nil)
	if err != nil {
		return err
	}
	entry := &xmetapb.CacheEntry{}
	if err := proto.Unmarshal(value, entry); err != nil {
		return err
	}
	return unmarshalAny(entry.GetValue(), m)
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
		return nil, err
	}
	return trait, nil
}

func AudioFiles(trait *contentagnosticpb.PlaybackTrait) []*contentagnosticpb.AudioFile {
	var files []*contentagnosticpb.AudioFile
	for _, entry := range trait.GetAudio() {
		files = append(files, entry.GetValue().GetHosted().GetFiles()...)
	}
	return files
}
