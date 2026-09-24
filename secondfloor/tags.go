package secondfloor

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/DavidBuchanan314/secondfloor/secondfloor/metadatapb"
	"go.senan.xyz/taglib"
)

func TrackTags(track *metadatapb.Track) map[string][]string {
	tags := make(map[string][]string)
	add := func(key, value string) {
		if value != "" {
			tags[key] = append(tags[key], value)
		}
	}
	album := track.GetAlbum()

	add(taglib.Title, track.GetName())
	for _, a := range track.GetArtist() {
		add(taglib.Artist, a.GetName())
	}
	add(taglib.Album, album.GetName())
	for _, a := range album.GetArtist() {
		add(taglib.AlbumArtist, a.GetName())
	}
	if n := track.GetNumber(); n > 0 {
		add(taglib.TrackNumber, strconv.Itoa(int(n)))
	}
	if n := track.GetDiscNumber(); n > 0 {
		add(taglib.DiscNumber, strconv.Itoa(int(n)))
	}
	add(taglib.Date, formatDate(album.GetDate()))
	add(taglib.Label, album.GetLabel())
	for _, id := range track.GetExternalId() {
		if id.GetType() == "isrc" {
			add(taglib.ISRC, id.GetId())
		}
	}
	add("SPOTIFY_TRACK_ID", Base62ID(track.GetGid()))
	return tags
}

func formatDate(d *metadatapb.Date) string {
	switch {
	case d.GetYear() == 0:
		return ""
	case d.GetMonth() == 0:
		return fmt.Sprintf("%04d", d.GetYear())
	case d.GetDay() == 0:
		return fmt.Sprintf("%04d-%02d", d.GetYear(), d.GetMonth())
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.GetYear(), d.GetMonth(), d.GetDay())
}

func (idx *StorageIndex) CoverImage(album *metadatapb.Album) ([]byte, error) {
	images := append(append([]*metadatapb.Image{}, album.GetCoverGroup().GetImage()...), album.GetCover()...)
	sort.SliceStable(images, func(i, j int) bool {
		return images[i].GetWidth() > images[j].GetWidth()
	})
	for _, img := range images {
		rec, ok := idx.Lookup(img.GetFileId(), RealmImage)
		if !ok {
			continue
		}
		data, err := idx.ReadPlainFile(rec)
		if err != nil {
			return nil, err
		}
		if bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) || bytes.HasPrefix(data, []byte("\x89PNG")) {
			return data, nil
		}
	}
	return nil, nil
}

func writeTags(path string, track *metadatapb.Track, cover []byte) error {
	if err := taglib.WriteTags(path, TrackTags(track), taglib.Clear); err != nil {
		return fmt.Errorf("writing tags: %w", err)
	}
	if cover != nil {
		if err := taglib.WriteImage(path, cover); err != nil {
			return fmt.Errorf("writing cover: %w", err)
		}
	}
	return nil
}
