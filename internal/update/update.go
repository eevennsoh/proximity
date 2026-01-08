package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/hashicorp/go-version"
)

var (
	versionUrl string
)

const (
	requestTimeout = 10 * time.Second
)

// VersionInfo represents the version manifest from the remote server
type VersionInfo struct {
	Version     string    `json:"version"`
	PublishedAt time.Time `json:"published_at"`
}

func NofifyIfNewVersionExists(currentVersion string) error {
	httpClient := &http.Client{
		Timeout: requestTimeout,
	}

	resp, err := httpClient.Get(versionUrl)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// Avoid sending a notification
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var info VersionInfo

	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return err
	}

	if !isNewerVersion(currentVersion, info.Version) {
		return nil
	}

	return beeep.Notify(
		"Proximity",
		fmt.Sprintf("Version %s is available!", info.Version),
		"",
	)
}

func isNewerVersion(currentVersion, latestVersion string) bool {
	current, err := version.NewVersion(currentVersion)
	if err != nil {
		return false
	}

	latestVer, err := version.NewVersion(latestVersion)
	if err != nil {
		return false
	}

	return latestVer.GreaterThan(current)
}
