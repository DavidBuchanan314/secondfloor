package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DavidBuchanan314/secondfloor/secondfloor"
	"github.com/fsnotify/fsnotify"
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

func (f sourceFlags) source() *secondfloor.Source {
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
	return source
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
		fatal(fmt.Errorf("loading .env: %w", err))
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

	sess, err := src.source().Open([]byte(requireEnv("SECONDFLOOR_HMAC_SECRET")))
	if err != nil {
		fatal(err)
	}
	defer sess.Close()
	for t, err := range sess.DownloadedTracks() {
		if err != nil {
			fatal(err)
		}
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
	fetchArtwork := flags.Bool("fetch-artwork", false, "download cover art from spotify's image CDN when none is cached locally")
	watch := flags.Bool("watch", false, "after syncing, keep running and sync again whenever spotify finishes a download")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: %s sync [flags] <out-dir>\n", os.Args[0])
		flags.PrintDefaults()
	}
	flags.Parse(args)
	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(2)
	}

	syncer := &secondfloor.Syncer{
		Source:       src.source(),
		HMACSecret:   []byte(requireEnv("SECONDFLOOR_HMAC_SECRET")),
		OutDir:       flags.Arg(0),
		Overwrite:    *overwrite,
		FetchArtwork: *fetchArtwork,
	}
	if !*watch {
		if _, err := syncer.Sync(); err != nil {
			fatal(err)
		}
		return
	}
	if err := watchAndSync(syncer); err != nil {
		fatal(err)
	}
}

const watchSettle = time.Second

func watchAndSync(syncer *secondfloor.Syncer) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}
	defer watcher.Close()
	if err := watcher.Add(syncer.Source.UserDir); err != nil {
		return fmt.Errorf("watching %s: %w", syncer.Source.UserDir, err)
	}

	runSync := func() {
		result, err := syncer.Sync()
		switch {
		case err != nil:
			slog.Warn("sync failed", "err", err)
		case result.Exported > 0 || result.Failed > 0:
			slog.Info("sync finished", "exported", result.Exported, "failed", result.Failed)
		default:
			slog.Debug("sync finished, nothing new")
		}
	}

	runSync()
	slog.Info("watching for downloads", "dir", syncer.Source.UserDir)
	settle := time.NewTimer(watchSettle)
	settle.Stop()
	for {
		select {
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if filepath.Base(ev.Name) == "offline2" && ev.Has(fsnotify.Create) {
				slog.Debug("offline2 updated")
				settle.Reset(watchSettle)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			return fmt.Errorf("watching %s: %w", syncer.Source.UserDir, err)
		case <-settle.C:
			runSync()
		}
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
