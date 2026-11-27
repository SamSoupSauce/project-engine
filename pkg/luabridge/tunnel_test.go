package luabridge

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTicket21ParseTryCloudflareURLAndUpstream(t *testing.T) {
	logLine := "2026-11-27T11:30:00Z INF |  https://gentle-falcon-4242.trycloudflare.com  |"
	got := ParseTryCloudflareURL(logLine)
	if got != "https://gentle-falcon-4242.trycloudflare.com" {
		t.Fatalf("unexpected extracted URL: %q", got)
	}
	if ParseTryCloudflareURL("no tunnel url here") != "" {
		t.Fatalf("expected empty URL when none present")
	}

	if u := NormalizeTunnelUpstreamURL("0.0.0.0:4242"); u != "http://127.0.0.1:4242" {
		t.Fatalf("expected http://127.0.0.1:4242, got %q", u)
	}
	if u := NormalizeTunnelUpstreamURL(":9000"); u != "http://127.0.0.1:9000" {
		t.Fatalf("expected http://127.0.0.1:9000, got %q", u)
	}
}

func TestTicket21EnsureCloudflaredDownloadAndCache(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("fake-cloudflared-binary"))
	}))
	defer srv.Close()

	cacheDir := filepath.Join(t.TempDir(), "bin")
	tm := NewTunnelManager(nil, TunnelConfig{
		CacheDir:    cacheDir,
		DownloadURL: srv.URL + "/cloudflared",
		BinaryPath:  filepath.Join(cacheDir, "nonexistent_force_download"),
	})

	// Isolate PATH so a real cloudflared on host doesn't bypass download test.
	t.Setenv("PATH", t.TempDir())

	path1, err := tm.EnsureCloudflared(t.Context())
	if err != nil {
		t.Fatalf("EnsureCloudflared failed: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 download hit, got %d", hits)
	}
	raw, err := os.ReadFile(path1)
	if err != nil || string(raw) != "fake-cloudflared-binary" {
		t.Fatalf("unexpected cached binary contents: %q err=%v", string(raw), err)
	}

	// Second call reuses cache without downloading again.
	path2, err := tm.EnsureCloudflared(t.Context())
	if err != nil {
		t.Fatalf("second EnsureCloudflared failed: %v", err)
	}
	if path2 != path1 || hits != 1 {
		t.Fatalf("expected cached path %q with 1 hit, got %q hits=%d", path1, path2, hits)
	}
}

func TestTicket21TunnelManagerURLDetectionAndConsoleOutput(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()
	console := NewConsole(bridge)

	gotURLCh := make(chan string, 1)
	tm := NewTunnelManager(console, TunnelConfig{
		TargetAddress: "127.0.0.1:4242",
		OnURL: func(u string) {
			select {
			case gotURLCh <- u:
			default:
			}
		},
	})

	var bin string
	var args []string
	if runtime.GOOS == "windows" {
		bin = "cmd"
		args = []string{"/C", "echo INF +------------------------------------------------------------+ 1>&2 & echo INF ^| https://alpha-test-99.trycloudflare.com ^| 1>&2"}
	} else {
		bin = "sh"
		args = []string{"-c", "echo 'INF | https://alpha-test-99.trycloudflare.com |' >&2"}
	}

	if err := tm.StartWithCommand(t.Context(), bin, args...); err != nil {
		t.Fatalf("StartWithCommand failed: %v", err)
	}
	defer tm.Stop()

	select {
	case u := <-gotURLCh:
		if u != "https://alpha-test-99.trycloudflare.com" {
			t.Fatalf("unexpected tunnel URL: %q", u)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for tunnel URL")
	}

	if tm.GetURL() != "https://alpha-test-99.trycloudflare.com" {
		t.Fatalf("GetURL mismatch: %q", tm.GetURL())
	}

	lines := console.GetOutputLines()
	found := false
	for _, l := range lines {
		if strings.Contains(l.Text, "https://alpha-test-99.trycloudflare.com") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected console output to contain assigned tunnel URL, got %+v", lines)
	}
	_ = fmt.Sprintf("verified tunnel URL: %s", tm.GetURL())
}
