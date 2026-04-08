package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIsNewer verifies semver comparison logic.
func TestIsNewer(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"v1.0.2", "v1.0.3", true},
		{"v1.0.3", "v1.0.3", false},
		{"v1.0.3", "v1.0.2", false},
		{"v1.0.2", "v1.0.10", true},
		{"dev", "v1.0.3", false},
		{"v1.0.3", "dev", false},
		{"v1.0.0", "v2.0.0", true},
		{"v2.0.0", "v1.0.0", false},
		// Without "v" prefix — IsNewer should add it.
		{"1.0.2", "1.0.3", true},
		{"1.0.3", "1.0.2", false},
		// Empty strings.
		{"", "v1.0.0", false},
		{"v1.0.0", "", false},
	}

	for _, tt := range tests {
		got := IsNewer(tt.current, tt.latest)
		if got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}

// TestCacheReadWrite verifies a round-trip through writeCache/readCache.
func TestCacheReadWrite(t *testing.T) {
	dir := t.TempDir()
	original := &Cache{
		LatestVersion:   "v1.2.3",
		CheckedAt:       time.Now().UTC().Truncate(time.Second),
		NotifiedAt:      time.Now().UTC().Truncate(time.Second),
		NotifiedVersion: "v1.2.3",
	}

	if err := writeCache(dir, original); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	got, err := readCache(dir)
	if err != nil {
		t.Fatalf("readCache: %v", err)
	}

	if got.LatestVersion != original.LatestVersion {
		t.Errorf("LatestVersion: got %q, want %q", got.LatestVersion, original.LatestVersion)
	}
	if !got.CheckedAt.Equal(original.CheckedAt) {
		t.Errorf("CheckedAt: got %v, want %v", got.CheckedAt, original.CheckedAt)
	}
	if !got.NotifiedAt.Equal(original.NotifiedAt) {
		t.Errorf("NotifiedAt: got %v, want %v", got.NotifiedAt, original.NotifiedAt)
	}
	if got.NotifiedVersion != original.NotifiedVersion {
		t.Errorf("NotifiedVersion: got %q, want %q", got.NotifiedVersion, original.NotifiedVersion)
	}
}

// TestCacheReadMissing verifies that a missing cache file returns an empty Cache without error.
func TestCacheReadMissing(t *testing.T) {
	dir := t.TempDir()
	c, err := readCache(dir)
	if err != nil {
		t.Fatalf("readCache on missing file should not error, got: %v", err)
	}
	if c == nil {
		t.Fatal("readCache returned nil cache")
	}
	if c.LatestVersion != "" {
		t.Errorf("empty cache should have empty LatestVersion, got %q", c.LatestVersion)
	}
}

// TestFetchLatestVersion verifies that fetchLatestVersion parses tag_name correctly.
func TestFetchLatestVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v1.5.0"})
	}))
	defer srv.Close()

	version, err := fetchLatestVersion(srv.URL)
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if version != "v1.5.0" {
		t.Errorf("got %q, want %q", version, "v1.5.0")
	}
}

// TestFetchLatestVersionTimeout verifies that a slow server causes an error.
func TestFetchLatestVersionTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the 2s httpTimeout.
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := fetchLatestVersion(srv.URL)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// TestCheckForUpdate_CacheFresh_NewVersion verifies that a pre-populated cache with a
// newer version produces a notification.
func TestCheckForUpdate_CacheFresh_NewVersion(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{
		LatestVersion: "v1.5.0",
		CheckedAt:     time.Now().UTC(),
	}
	if err := writeCache(dir, c); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	result := CheckForUpdate("v1.0.0", dir, "")
	if !result.ShouldNotify {
		t.Error("expected ShouldNotify=true")
	}
	if result.LatestVersion != "v1.5.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "v1.5.0")
	}
}

// TestCheckForUpdate_CacheFresh_SameVersion verifies no notification when versions match.
func TestCheckForUpdate_CacheFresh_SameVersion(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{
		LatestVersion: "v1.0.0",
		CheckedAt:     time.Now().UTC(),
	}
	if err := writeCache(dir, c); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	result := CheckForUpdate("v1.0.0", dir, "")
	if result.ShouldNotify {
		t.Error("expected ShouldNotify=false")
	}
}

// TestCheckForUpdate_NotificationSuppressed verifies that the same version notified
// within 7 days does not produce a second notification.
func TestCheckForUpdate_NotificationSuppressed(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{
		LatestVersion:   "v1.5.0",
		CheckedAt:       time.Now().UTC(),
		NotifiedAt:      time.Now().UTC().Add(-3 * 24 * time.Hour), // 3 days ago
		NotifiedVersion: "v1.5.0",
	}
	if err := writeCache(dir, c); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	result := CheckForUpdate("v1.0.0", dir, "")
	if result.ShouldNotify {
		t.Error("expected ShouldNotify=false (suppressed)")
	}
}

// TestCheckForUpdate_NotificationResurfaces verifies that the same version notified
// more than 7 days ago produces a new notification.
func TestCheckForUpdate_NotificationResurfaces(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{
		LatestVersion:   "v1.5.0",
		CheckedAt:       time.Now().UTC(),
		NotifiedAt:      time.Now().UTC().Add(-8 * 24 * time.Hour), // 8 days ago
		NotifiedVersion: "v1.5.0",
	}
	if err := writeCache(dir, c); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	result := CheckForUpdate("v1.0.0", dir, "")
	if !result.ShouldNotify {
		t.Error("expected ShouldNotify=true after 7-day suppression expires")
	}
}

// TestCheckForUpdate_NewerThanNotified verifies that a newly discovered version that
// is newer than the previously notified version always triggers a notification.
func TestCheckForUpdate_NewerThanNotified(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{
		LatestVersion:   "v1.6.0", // newer than what was notified
		CheckedAt:       time.Now().UTC(),
		NotifiedAt:      time.Now().UTC().Add(-1 * 24 * time.Hour), // 1 day ago (within 7 days)
		NotifiedVersion: "v1.5.0",
	}
	if err := writeCache(dir, c); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	result := CheckForUpdate("v1.0.0", dir, "")
	if !result.ShouldNotify {
		t.Error("expected ShouldNotify=true for version newer than previously notified")
	}
}

// TestCheckForUpdate_DevVersion verifies that dev builds produce no notifications.
func TestCheckForUpdate_DevVersion(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{
		LatestVersion: "v1.5.0",
		CheckedAt:     time.Now().UTC(),
	}
	if err := writeCache(dir, c); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	result := CheckForUpdate("dev", dir, "")
	if result.ShouldNotify {
		t.Error("dev build should not notify")
	}

	// Also test empty version.
	result = CheckForUpdate("", dir, "")
	if result.ShouldNotify {
		t.Error("empty version should not notify")
	}
}

// TestCheckForUpdate_NotificationFields verifies the structured notification fields.
func TestCheckForUpdate_NotificationFields(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{
		LatestVersion: "v1.5.0",
		CheckedAt:     time.Now().UTC(),
	}
	if err := writeCache(dir, c); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	result := CheckForUpdate("v1.0.0", dir, "")
	if !result.ShouldNotify {
		t.Fatal("expected ShouldNotify=true")
	}
	if result.LatestVersion != "v1.5.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "v1.5.0")
	}
}

// TestCacheFilePath verifies the cache is written to the expected location.
func TestCacheFilePath(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{LatestVersion: "v1.0.0", CheckedAt: time.Now().UTC()}
	if err := writeCache(dir, c); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	expectedPath := filepath.Join(dir, "contextception", "update-check.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("cache file not found at expected path %q", expectedPath)
	}
}
