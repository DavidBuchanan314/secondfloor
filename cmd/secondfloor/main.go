package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/DavidBuchanan314/secondfloor/secondfloor"
	"github.com/joho/godotenv"
)

type sourceFlags struct {
	cacheDir   *string
	username   *string
	storageDir *string
	verbose    *bool
}

func addSourceFlags(flags *flag.FlagSet) sourceFlags {
	return sourceFlags{
		cacheDir:   flags.String("spotify-dir", "", "spotify cache directory (default: auto-detect)"),
		username:   flags.String("user", "", "spotify account (required if there are several)"),
		storageDir: flags.String("storage", "", "storage directory containing index.dat (default: from spotify prefs)"),
		verbose:    flags.Bool("v", false, "verbose (debug) logging"),
	}
}

func (f sourceFlags) open() *secondfloor.Session {
	if *f.verbose {
		logLevel.Set(slog.LevelDebug)
	}
	source, err := secondfloor.DiscoverDesktopSource(*f.cacheDir, *f.username)
	if err != nil {
		fatal(err)
	}
	if *f.storageDir != "" {
		source.StorageDir = *f.storageDir
		source.FileDirs = append([]string{*f.storageDir}, source.FileDirs...)
	}
	if err := source.Validate(); err != nil {
		fatal(err)
	}
	sess, err := source.Open([]byte(requireEnv("SECONDFLOOR_HMAC_SECRET")))
	if err != nil {
		fatal(err)
	}
	return sess
}

var logLevel = new(slog.LevelVar)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))

	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		fatal(err)
	}

	commands := map[string]func([]string){
		"list": runList,
		"sync": runSync,
	}
	if len(os.Args) < 2 || commands[os.Args[1]] == nil {
		fmt.Fprintf(os.Stderr, "usage: %s <command> [flags]\n\ncommands:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  list    list tracks downloaded in spotify\n")
		fmt.Fprintf(os.Stderr, "  sync    decrypt downloaded tracks into an output directory\n")
		os.Exit(2)
	}
	commands[os.Args[1]](os.Args[2:])
}

func runList(args []string) {
	flags := flag.NewFlagSet("list", flag.ExitOnError)
	src := addSourceFlags(flags)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: %s list [flags]\n", os.Args[0])
		flags.PrintDefaults()
	}
	flags.Parse(args)
	if flags.NArg() != 0 {
		flags.Usage()
		os.Exit(2)
	}

	sess := src.open()
	defer sess.Close()
	tracks, err := sess.DownloadedTracks()
	if err != nil {
		fatal(err)
	}
	for _, t := range tracks {
		desc := "(no metadata)"
		if t.Track != nil {
			var artists []string
			for _, a := range t.Track.GetArtist() {
				artists = append(artists, a.GetName())
			}
			desc = fmt.Sprintf("%s - %s - %02d %s", strings.Join(artists, ", "), t.Track.GetAlbum().GetName(), t.Track.GetNumber(), t.Track.GetName())
		}
		note := ""
		if !t.HasKey {
			note = " (no content key)"
		}
		fmt.Printf("%s %s [%s]%s\n", strings.TrimPrefix(t.URI, "spotify:track:"), desc, t.File.GetFormat(), note)
	}
}

func runSync(args []string) {
	flags := flag.NewFlagSet("sync", flag.ExitOnError)
	src := addSourceFlags(flags)
	overwrite := flags.Bool("overwrite", false, "re-export tracks whose output files already exist")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: %s sync [flags] <out-dir>\n", os.Args[0])
		flags.PrintDefaults()
	}
	flags.Parse(args)
	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(2)
	}
	outDir := flags.Arg(0)

	sess := src.open()
	defer sess.Close()
	tracks, err := sess.DownloadedTracks()
	if err != nil {
		fatal(err)
	}
	for _, t := range tracks {
		dst, err := sess.Export(t, outDir, *overwrite)
		if errors.Is(err, secondfloor.ErrExists) {
			slog.Debug("already exported", "uri", t.URI, "path", dst)
			continue
		}
		if err != nil {
			slog.Warn("skipping track", "uri", t.URI, "err", err)
			continue
		}
		slog.Info("exported", "uri", t.URI, "path", dst)
	}
}

func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		fatal(fmt.Errorf("%s is not set", name))
	}
	return value
}

func fatal(err error) {
	slog.Error(err.Error())
	os.Exit(1)
}
