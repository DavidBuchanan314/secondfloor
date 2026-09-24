package secondfloor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Source struct {
	Username   string
	UserDir    string
	StorageDir string
	FileDirs   []string
}

func (s *Source) OpenDB() (*DB, error) {
	return OpenDB(filepath.Join(s.UserDir, "primary.ldb"))
}

func (s *Source) ReadOfflineLists() (*OfflineLists, error) {
	return ReadOfflineLists(filepath.Join(s.UserDir, "offline_lists.bnk"))
}

func (s *Source) ReadOfflineKeys(deviceID string) (map[FileID]ContentKey, error) {
	return ReadOfflineKeys(filepath.Join(s.UserDir, "offline2"), deviceID)
}

func (s *Source) ReadStorageIndex(hmacSecret []byte) (*StorageIndex, error) {
	idx, err := ReadStorageIndex(s.StorageDir, hmacSecret)
	if err != nil {
		return nil, err
	}
	idx.FileDirs = s.FileDirs
	return idx, nil
}

func (s *Source) Validate() error {
	for _, path := range []string{
		filepath.Join(s.UserDir, "primary.ldb"),
		filepath.Join(s.UserDir, "offline_lists.bnk"),
		filepath.Join(s.UserDir, "offline2"),
		filepath.Join(s.StorageDir, "index.dat"),
	} {
		if _, err := os.Stat(path); err != nil {
			return err
		}
	}
	return nil
}

type DesktopInstall struct {
	CacheDir  string
	PrefsPath string
}

func DesktopInstalls() []DesktopInstall {
	var installs []DesktopInstall
	if home, err := os.UserHomeDir(); err == nil {
		installs = append(installs,
			DesktopInstall{
				CacheDir:  filepath.Join(home, ".var", "app", "com.spotify.Client", "cache", "spotify"),
				PrefsPath: filepath.Join(home, ".var", "app", "com.spotify.Client", "config", "spotify", "prefs"),
			},
			DesktopInstall{
				CacheDir:  filepath.Join(home, "snap", "spotify", "common", ".cache", "spotify"),
				PrefsPath: filepath.Join(home, "snap", "spotify", "current", ".config", "spotify", "prefs"),
			},
		)
	}
	cache, cacheErr := os.UserCacheDir()
	config, configErr := os.UserConfigDir()
	if cacheErr == nil && configErr == nil {
		installs = append(installs, DesktopInstall{
			CacheDir:  filepath.Join(cache, "spotify"),
			PrefsPath: filepath.Join(config, "spotify", "prefs"),
		})
	}
	return installs
}

func (in DesktopInstall) Sources() ([]*Source, error) {
	userDirs, err := filepath.Glob(filepath.Join(in.CacheDir, "Users", "*-user"))
	if err != nil {
		return nil, err
	}
	if len(userDirs) == 0 {
		return nil, nil
	}
	storageDir := filepath.Join(in.CacheDir, "Storage")
	if in.PrefsPath != "" {
		prefs, err := ReadPrefs(in.PrefsPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if location := prefs["storage.location"]; location != "" {
			storageDir = location
		}
	}
	sort.Strings(userDirs)
	var sources []*Source
	for _, userDir := range userDirs {
		sources = append(sources, &Source{
			Username:   strings.TrimSuffix(filepath.Base(userDir), "-user"),
			UserDir:    userDir,
			StorageDir: storageDir,
			FileDirs:   []string{storageDir, filepath.Join(in.CacheDir, "Data")},
		})
	}
	return sources, nil
}

func ReadPrefs(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	prefs := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		prefs[key] = value
	}
	return prefs, nil
}

func DiscoverDesktopSource(cacheDir, username string) (*Source, error) {
	installs := DesktopInstalls()
	if cacheDir != "" {
		override := DesktopInstall{CacheDir: cacheDir}
		for _, in := range installs {
			if filepath.Clean(in.CacheDir) == filepath.Clean(cacheDir) {
				override = in
			}
		}
		installs = []DesktopInstall{override}
	}
	var sources []*Source
	var searched []string
	for _, in := range installs {
		found, err := in.Sources()
		if err != nil {
			return nil, err
		}
		sources = append(sources, found...)
		searched = append(searched, in.CacheDir)
	}

	var matches []*Source
	for _, s := range sources {
		if username == "" || s.Username == username {
			matches = append(matches, s)
		}
	}
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(sources) == 0:
		return nil, fmt.Errorf("no spotify accounts found in %s", strings.Join(searched, ", "))
	case len(matches) == 0:
		return nil, fmt.Errorf("account %q not found; available: %s", username, usernames(sources))
	}
	return nil, errors.New("multiple spotify accounts found, choose one with -user: " + usernames(matches))
}

func usernames(sources []*Source) string {
	var names []string
	for _, s := range sources {
		names = append(names, fmt.Sprintf("%s (%s)", s.Username, s.UserDir))
	}
	return strings.Join(names, ", ")
}
