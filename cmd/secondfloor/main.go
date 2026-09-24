package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidBuchanan314/secondfloor/secondfloor"
	"github.com/joho/godotenv"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s <user-dir> <out-dir> [storage-dir]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 2 || flag.NArg() > 3 {
		flag.Usage()
		os.Exit(2)
	}

	userDir := flag.Arg(0)
	outDir := flag.Arg(1)
	storageDir := filepath.Join(userDir, "..", "..", "Storage")
	if flag.NArg() == 3 {
		storageDir = flag.Arg(2)
	}

	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	hmacSecret := os.Getenv("SECONDFLOOR_HMAC_SECRET")
	if hmacSecret == "" {
		fmt.Fprintf(os.Stderr, "error: SECONDFLOOR_HMAC_SECRET is not set\n")
		os.Exit(1)
	}

	audioIV, err := hex.DecodeString(os.Getenv("SECONDFLOOR_AUDIO_IV"))
	if err != nil || len(audioIV) != 16 {
		fmt.Fprintf(os.Stderr, "error: SECONDFLOOR_AUDIO_IV must be set to 16 hex-encoded bytes\n")
		os.Exit(1)
	}

	storage, err := secondfloor.ReadStorageIndex(storageDir, []byte(hmacSecret))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	db, err := secondfloor.OpenDB(filepath.Join(userDir, "primary.ldb"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	lists, err := secondfloor.ReadOfflineLists(filepath.Join(userDir, "offline_lists.bnk"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	keys, err := secondfloor.ReadOfflineKeys(filepath.Join(userDir, "offline2"), lists.DeviceID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("device_id %s\n", lists.DeviceID)
	exported := make(map[string]bool)
	for _, ctx := range lists.Contexts {
		fmt.Printf("[%s] %s\n", ctx.Kind, ctx.URI)
		for _, gid := range ctx.TrackGIDs {
			uri := secondfloor.TrackURI(gid)
			desc := "(not in collection)"
			track, err := db.CollectionTrack(uri)
			if err != nil {
				track = nil
			}
			switch {
			case err == nil:
				desc = fmt.Sprintf("%s - %s - %s", strings.Join(track.GetArtistName(), ", "), track.GetAlbumName(), track.GetTrackName())
			case !errors.Is(err, secondfloor.ErrNotFound):
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\t%x %s %s\n", gid, uri, desc)

			trait, err := db.PlaybackTrait(uri)
			switch {
			case err == nil:
				for _, file := range secondfloor.AudioFiles(trait) {
					rec, ok := storage.Lookup(file.GetFileId())
					if !ok {
						continue
					}
					fmt.Printf("\t\t%x %s\n", file.GetFileId(), file.GetFormat())
					if track == nil || exported[uri] {
						continue
					}
					key, ok := keys[rec.ID]
					if !ok {
						fmt.Printf("\t\t(no content key)\n")
						continue
					}
					dst := secondfloor.TrackOutputPath(outDir, track)
					if err := storage.DecryptFile(rec, key, audioIV, dst); err != nil {
						fmt.Fprintf(os.Stderr, "error: %v\n", err)
						os.Exit(1)
					}
					exported[uri] = true
					fmt.Printf("\t\t-> %s\n", dst)
				}
			case errors.Is(err, secondfloor.ErrNotFound):
				fmt.Printf("\t\t(no playback trait)\n")
			default:
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
	}
}
