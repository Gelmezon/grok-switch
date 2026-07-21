package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckFindsReleaseAndUsesCache(t *testing.T) {
	var requests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("ETag", `"release-v1.1.0"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v1.1.0",
			"html_url": server.URL + "/release/v1.1.0",
			"assets": []map[string]any{
				{
					"name":                 "grok-switch-linux-amd64",
					"browser_download_url": server.URL + "/grok-switch-linux-amd64",
					"digest":               "sha256:" + strings.Repeat("a", 64),
					"size":                 1234,
				},
			},
		})
	}))
	defer server.Close()

	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	svc := NewService("v1.0.0", filepath.Join(t.TempDir(), "update-state.json"))
	svc.APIBase = server.URL
	svc.AllowInsecureHTTP = true
	svc.GOOS = "linux"
	svc.GOARCH = "amd64"
	svc.Now = func() time.Time { return now }

	info, err := svc.Check(context.Background(), true)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !info.Available || info.LatestVersion != "v1.1.0" {
		t.Fatalf("Check() info = %+v", info)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}

	now = now.Add(time.Hour)
	info, err = svc.Check(context.Background(), true)
	if err != nil {
		t.Fatalf("cached Check() error = %v", err)
	}
	if !info.Available || requests.Load() != 1 {
		t.Fatalf("cached Check() info = %+v, requests = %d", info, requests.Load())
	}
}

func TestCheckRejectsDevelopmentVersion(t *testing.T) {
	svc := NewService("v1.0.0-2-gabcdef", filepath.Join(t.TempDir(), "state.json"))
	svc.GOOS = "linux"
	svc.GOARCH = "amd64"
	if _, err := svc.Check(context.Background(), true); err == nil {
		t.Fatal("Check() expected an error for development version")
	}
}

func TestApplyAndRollback(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "grok-switch")
	writeExecutable(t, currentPath, "v1.0.0")

	newBinary := scriptForVersion("v1.1.0")
	digest := sha256.Sum256(newBinary)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newBinary)
	}))
	defer server.Close()

	svc := NewService("v1.0.0", filepath.Join(dir, "state.json"))
	svc.GOOS = "linux"
	svc.GOARCH = "amd64"
	svc.ExecutablePath = currentPath
	svc.AllowInsecureHTTP = true
	info := Info{
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v1.1.0",
		Available:      true,
		AssetName:      "grok-switch-linux-amd64",
		DownloadURL:    server.URL + "/grok-switch-linux-amd64",
		Digest:         "sha256:" + hex.EncodeToString(digest[:]),
		Size:           int64(len(newBinary)),
	}

	result, err := svc.Apply(context.Background(), info)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Version != "v1.1.0" {
		t.Fatalf("Apply() version = %q", result.Version)
	}
	assertBinaryVersion(t, currentPath, "v1.1.0")
	assertBinaryVersion(t, currentPath+".previous", "v1.0.0")

	result, err = svc.Rollback(context.Background())
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if result.Version != "v1.0.0" {
		t.Fatalf("Rollback() version = %q", result.Version)
	}
	assertBinaryVersion(t, currentPath, "v1.0.0")
	assertBinaryVersion(t, currentPath+".previous", "v1.1.0")
}

func TestApplyChecksumFailureKeepsCurrentBinary(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "grok-switch")
	writeExecutable(t, currentPath, "v1.0.0")

	newBinary := scriptForVersion("v1.1.0")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newBinary)
	}))
	defer server.Close()

	svc := NewService("v1.0.0", filepath.Join(dir, "state.json"))
	svc.GOOS = "linux"
	svc.GOARCH = "amd64"
	svc.ExecutablePath = currentPath
	svc.AllowInsecureHTTP = true
	_, err := svc.Apply(context.Background(), Info{
		LatestVersion: "v1.1.0",
		Available:     true,
		AssetName:     "grok-switch-linux-amd64",
		DownloadURL:   server.URL,
		Digest:        "sha256:" + strings.Repeat("0", 64),
		Size:          int64(len(newBinary)),
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 校验失败") {
		t.Fatalf("Apply() error = %v, want checksum failure", err)
	}
	assertBinaryVersion(t, currentPath, "v1.0.0")
	if _, err := os.Stat(currentPath + ".previous"); !os.IsNotExist(err) {
		t.Fatalf("unexpected previous backup after failed update: %v", err)
	}
}

func TestApplyFallsBackToSHA256SUMS(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "grok-switch")
	writeExecutable(t, currentPath, "v1.0.0")

	newBinary := scriptForVersion("v1.1.0")
	digest := sha256.Sum256(newBinary)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			_, _ = w.Write([]byte(hex.EncodeToString(digest[:]) + "  grok-switch-linux-amd64\n"))
		default:
			_, _ = w.Write(newBinary)
		}
	}))
	defer server.Close()

	svc := NewService("v1.0.0", filepath.Join(dir, "state.json"))
	svc.GOOS = "linux"
	svc.GOARCH = "amd64"
	svc.ExecutablePath = currentPath
	svc.AllowInsecureHTTP = true
	_, err := svc.Apply(context.Background(), Info{
		LatestVersion: "v1.1.0",
		Available:     true,
		AssetName:     "grok-switch-linux-amd64",
		DownloadURL:   server.URL + "/grok-switch-linux-amd64",
		ChecksumURL:   server.URL + "/SHA256SUMS",
		Size:          int64(len(newBinary)),
	})
	if err != nil {
		t.Fatalf("Apply() with SHA256SUMS error = %v", err)
	}
	assertBinaryVersion(t, currentPath, "v1.1.0")
}

func TestModeFromEnv(t *testing.T) {
	t.Setenv("GROK_SWITCH_NO_UPDATE_CHECK", "")
	t.Setenv("GROK_SWITCH_UPDATE_MODE", "auto")
	if got := ModeFromEnv(); got != ModeAuto {
		t.Fatalf("ModeFromEnv() = %q, want auto", got)
	}
	t.Setenv("GROK_SWITCH_UPDATE_MODE", "off")
	if got := ModeFromEnv(); got != ModeOff {
		t.Fatalf("ModeFromEnv() = %q, want off", got)
	}
	t.Setenv("GROK_SWITCH_UPDATE_MODE", "unexpected")
	if got := ModeFromEnv(); got != ModeNotify {
		t.Fatalf("ModeFromEnv() = %q, want notify", got)
	}
}

func writeExecutable(t *testing.T, path, version string) {
	t.Helper()
	if err := os.WriteFile(path, scriptForVersion(version), 0o755); err != nil {
		t.Fatal(err)
	}
}

func scriptForVersion(version string) []byte {
	return []byte("#!/bin/sh\nif [ \"$1\" = \"version\" ]; then\n  echo " + version + "\nfi\n")
}

func assertBinaryVersion(t *testing.T, path, want string) {
	t.Helper()
	out, err := exec.Command(path, "version").Output()
	if err != nil {
		t.Fatalf("run %s: %v", path, err)
	}
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("%s version = %q, want %q", path, got, want)
	}
}
