package secondfloor

import (
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	coverBaseURL      = "https://i.scdn.co/image/"
	maxCoverSize      = 16 << 20
	coverFetchTimeout = 30 * time.Second
)

var coverClient = &http.Client{Timeout: coverFetchTimeout}

func DownloadCover(fileID []byte) ([]byte, error) {
	resp, err := coverClient.Get(coverBaseURL + hex.EncodeToString(fileID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover %x: http %s", fileID, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverSize))
	if err != nil {
		return nil, err
	}
	if !isCoverImage(data) {
		return nil, fmt.Errorf("cover %x is not a jpeg or png", fileID)
	}
	return data, nil
}
