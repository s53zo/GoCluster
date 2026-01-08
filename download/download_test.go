package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadUpdated(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	dest := filepath.Join(dir, "cty.plist")

	res, err := Download(testContext(t), Request{
		URL:         server.URL,
		Destination: dest,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if res.Status != StatusUpdated {
		t.Fatalf("expected updated status, got %s", res.Status)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected content: %q", string(data))
	}
	meta, _ := ReadMetadata(MetadataPath(dest))
	if meta == nil || meta.ETag != `"v1"` || meta.SHA256 == "" {
		t.Fatalf("metadata missing expected fields: %+v", meta)
	}
}

func TestDownloadNotModified(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("same"))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	dest := filepath.Join(dir, "scp.txt")

	first, err := Download(testContext(t), Request{
		URL:         server.URL,
		Destination: dest,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("initial download: %v", err)
	}
	if first.Status != StatusUpdated {
		t.Fatalf("expected updated status, got %s", first.Status)
	}
	metaBefore, _ := ReadMetadata(MetadataPath(dest))
	if metaBefore == nil {
		t.Fatalf("missing metadata after first download")
	}

	time.Sleep(10 * time.Millisecond)
	second, err := Download(testContext(t), Request{
		URL:         server.URL,
		Destination: dest,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("second download: %v", err)
	}
	if second.Status != StatusNotModified {
		t.Fatalf("expected not modified status, got %s", second.Status)
	}
	metaAfter, _ := ReadMetadata(MetadataPath(dest))
	if metaAfter == nil || metaAfter.DownloadedAt != metaBefore.DownloadedAt {
		t.Fatalf("expected DownloadedAt to remain unchanged")
	}
}

func TestDownloadSameContent(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v2"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("repeat"))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	dest := filepath.Join(dir, "ipinfo.gz")

	first, err := Download(testContext(t), Request{
		URL:         server.URL,
		Destination: dest,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("initial download: %v", err)
	}
	if first.Status != StatusUpdated {
		t.Fatalf("expected updated status, got %s", first.Status)
	}
	metaBefore, _ := ReadMetadata(MetadataPath(dest))
	if metaBefore == nil {
		t.Fatalf("missing metadata after first download")
	}

	time.Sleep(10 * time.Millisecond)
	second, err := Download(testContext(t), Request{
		URL:         server.URL,
		Destination: dest,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("second download: %v", err)
	}
	if second.Status != StatusSameContent {
		t.Fatalf("expected same content status, got %s", second.Status)
	}
	metaAfter, _ := ReadMetadata(MetadataPath(dest))
	if metaAfter == nil || metaAfter.DownloadedAt != metaBefore.DownloadedAt {
		t.Fatalf("expected DownloadedAt to remain unchanged")
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
