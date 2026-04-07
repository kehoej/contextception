package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	githubReleasesURL = "https://api.github.com/repos/kehoej/contextception/releases/latest"
	checkInterval     = 24 * time.Hour
	notifyInterval    = 7 * 24 * time.Hour
	httpTimeout       = 2 * time.Second
)

// Cache holds the persisted update-check state between CLI runs.
type Cache struct {
	LatestVersion   string    `json:"latest_version"`
	CheckedAt       time.Time `json:"checked_at"`
	NotifiedAt      time.Time `json:"notified_at,omitempty"`
	NotifiedVersion string    `json:"notified_version,omitempty"`
}

// CheckResult holds the outcome of CheckForUpdate.
type CheckResult struct {
	// Notification is non-empty when the user should be told about a new version.
	Notification string
	// RefreshDone is closed when the background cache refresh completes.
	// Nil if no refresh was needed. Callers can select on this to wait.
	RefreshDone <-chan struct{}
}

// cacheFilePath returns the path of the update-check cache file for the given
// user config directory (e.g. os.UserConfigDir()).
func cacheFilePath(configDir string) string {
	return filepath.Join(configDir, "contextception", "update-check.json")
}

// readCache reads the cache file. Returns an empty Cache (not an error) when
// the file is missing or corrupt, so callers never need to handle a nil return.
func readCache(configDir string) (*Cache, error) {
	path := cacheFilePath(configDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Cache{}, nil
		}
		// Treat any read error as "no cache".
		return &Cache{}, nil //nolint:nilerr
	}

	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		// Corrupt file — treat as empty.
		return &Cache{}, nil //nolint:nilerr
	}
	return &c, nil
}

// writeCache persists c to disk, creating intermediate directories as needed.
func writeCache(configDir string, c *Cache) error {
	path := cacheFilePath(configDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}
	return nil
}

// ensureVPrefix returns s with a "v" prefix, if it does not already have one.
func ensureVPrefix(s string) string {
	if strings.HasPrefix(s, "v") {
		return s
	}
	return "v" + s
}

// IsNewer reports whether latest is a higher semver than current.
// Returns false when either value is not valid semver (e.g. "dev").
func IsNewer(current, latest string) bool {
	c := ensureVPrefix(current)
	l := ensureVPrefix(latest)
	if !semver.IsValid(c) || !semver.IsValid(l) {
		return false
	}
	return semver.Compare(l, c) > 0
}

// fetchLatestVersion sends a GET request to url and parses the GitHub
// releases API response to extract the tag_name field.
func fetchLatestVersion(url string) (string, error) {
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "contextception-update-check")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("empty tag_name in response")
	}
	return payload.TagName, nil
}

// FetchLatest is an exported wrapper around fetchLatestVersion. If apiURL is
// empty it uses the default GitHub releases URL.
func FetchLatest(apiURL string) (string, error) {
	if apiURL == "" {
		apiURL = githubReleasesURL
	}
	return fetchLatestVersion(apiURL)
}

// refreshCache fetches the latest release version and updates the cache.
// Errors are silently discarded — this runs in a background goroutine and
// should never surface to the user.
func refreshCache(configDir, apiURL string) {
	if apiURL == "" {
		apiURL = githubReleasesURL
	}
	version, err := fetchLatestVersion(apiURL)
	if err != nil {
		return
	}

	// Read existing cache so we preserve NotifiedAt/NotifiedVersion.
	c, _ := readCache(configDir)
	if c == nil {
		c = &Cache{}
	}
	c.LatestVersion = version
	c.CheckedAt = time.Now().UTC()
	_ = writeCache(configDir, c)
}

// CheckForUpdate implements the cache-then-notify pattern:
//  1. Read cache synchronously (fast path).
//  2. If cache is stale, spawn a background goroutine to refresh it for the NEXT run.
//  3. Compare cached latest vs currentVersion.
//  4. Apply notification-suppression rules.
//  5. If notifying, update the cache and return a non-empty CheckResult.Notification.
//
// If the cache is stale, RefreshDone will be a non-nil channel that closes when
// the background refresh completes. Callers can optionally wait on it to ensure
// the cache is populated before the process exits.
func CheckForUpdate(currentVersion, configDir, apiURL string) CheckResult {
	// Dev and empty builds are never notified.
	if currentVersion == "" || currentVersion == "dev" {
		return CheckResult{}
	}

	c, _ := readCache(configDir)
	if c == nil {
		c = &Cache{}
	}

	var result CheckResult

	// Refresh cache in background when stale.
	if time.Since(c.CheckedAt) > checkInterval {
		done := make(chan struct{})
		result.RefreshDone = done
		go func() {
			defer close(done)
			refreshCache(configDir, apiURL)
		}()
	}

	// Nothing to compare yet.
	if c.LatestVersion == "" {
		return result
	}

	// Is the cached version actually newer?
	if !IsNewer(currentVersion, c.LatestVersion) {
		return result
	}

	// Notification suppression: same version notified within 7 days.
	if c.NotifiedVersion == c.LatestVersion && time.Since(c.NotifiedAt) < notifyInterval {
		return result
	}

	// We will notify. Persist the notification record.
	c.NotifiedAt = time.Now().UTC()
	c.NotifiedVersion = c.LatestVersion
	_ = writeCache(configDir, c)

	result.Notification = fmt.Sprintf(
		"A new version of contextception is available (%s). Run 'contextception update' to upgrade.",
		c.LatestVersion,
	)
	return result
}
