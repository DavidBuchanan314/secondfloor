package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/DavidBuchanan314/secondfloor/secondfloor"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s <leveldb-path> <offline_lists.bnk>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}

	db, err := secondfloor.OpenDB(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	contexts, err := secondfloor.OfflineContexts(flag.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, ctx := range contexts {
		fmt.Printf("[%s] %s\n", ctx.Kind, ctx.URI)
		for _, gid := range ctx.TrackGIDs {
			uri := secondfloor.TrackURI(gid)
			desc := "(not in collection)"
			track, err := db.CollectionTrack(uri)
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
					fmt.Printf("\t\t%x %s\n", file.GetFileId(), file.GetFormat())
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
