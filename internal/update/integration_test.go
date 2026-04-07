package update

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFullUpdateCheckFlow(t *testing.T) {
	configDir := t.TempDir()

	// Mock GitHub API.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name": "v2.0.0"}`)
	}))
	defer srv.Close()

	// First run: cache is empty. No notification (cache-then-notify = 1 run delay).
	result := CheckForUpdate("v1.0.2", configDir, srv.URL)
	if result.Notification != "" {
		t.Errorf("first run should not notify (empty cache), got %q", result.Notification)
	}

	// Wait for background refresh goroutine.
	time.Sleep(500 * time.Millisecond)

	// Second run: cache now has v2.0.0. Should notify.
	result = CheckForUpdate("v1.0.2", configDir, srv.URL)
	if result.Notification == "" {
		t.Error("second run should notify after cache refresh")
	}

	// Third run: should be suppressed (same version, within 7 days).
	result = CheckForUpdate("v1.0.2", configDir, srv.URL)
	if result.Notification != "" {
		t.Error("third run should be suppressed")
	}

	// Simulate 8 days passing by manipulating cache.
	cache, _ := readCache(configDir)
	cache.NotifiedAt = time.Now().Add(-8 * 24 * time.Hour)
	writeCache(configDir, cache)

	// Fourth run: should notify again (7 day suppression expired).
	result = CheckForUpdate("v1.0.2", configDir, srv.URL)
	if result.Notification == "" {
		t.Error("fourth run should notify after suppression expires")
	}
}
