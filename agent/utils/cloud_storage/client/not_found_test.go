package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	webdavHelper "github.com/1Panel-dev/1Panel/agent/utils/cloud_storage/client/helper/webdav"
	"github.com/minio/minio-go/v7"
	qiniuClient "github.com/qiniu/go-sdk/v7/client"
)

func TestOneDriveExistUsesStructuredNotFoundResponse(t *testing.T) {
	const sensitiveMarker = "provider-detail-must-not-leak"
	tests := []struct {
		name      string
		status    int
		body      string
		wantExist bool
		wantErr   bool
	}{
		{
			name:      "object exists",
			status:    http.StatusOK,
			body:      `{"id":"drive-item-id"}`,
			wantExist: true,
		},
		{
			name:   "item not found",
			status: http.StatusNotFound,
			body:   `{"error":{"code":"itemNotFound","message":"missing"}}`,
		},
		{
			name:    "different 404 code remains an error",
			status:  http.StatusNotFound,
			body:    `{"error":{"code":"resourceNotFound","message":"` + sensitiveMarker + `"}}`,
			wantErr: true,
		},
		{
			name:    "permission failure remains an error",
			status:  http.StatusForbidden,
			body:    `{"error":{"code":"accessDenied","message":"` + sensitiveMarker + `"}}`,
			wantErr: true,
		},
		{
			name:    "malformed not found response remains an error",
			status:  http.StatusNotFound,
			body:    `{"error":`,
			wantErr: true,
		},
		{
			name:    "malformed success response remains an error",
			status:  http.StatusOK,
			body:    `{"id":`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			baseURL, err := url.Parse(server.URL + "/v1.0/")
			if err != nil {
				t.Fatalf("parse test URL: %v", err)
			}
			client := oneDriveClient{client: server.Client(), baseURL: baseURL, token: "synthetic-access-token"}

			exists, err := client.Exist("backup/file.tar.gz")
			if exists != tt.wantExist || (err != nil) != tt.wantErr {
				t.Fatalf("Exist() = (%v, %v), want (%v, error=%v)", exists, err, tt.wantExist, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), sensitiveMarker) {
				t.Fatalf("Exist() leaked provider response detail: %v", err)
			}
		})
	}
}

func TestOneDriveDownloadRejectsNonSuccessWithoutLeakingBody(t *testing.T) {
	const sensitiveMarker = "download-provider-detail-must-not-leak"
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/me/drive/root:/backup/file.tar.gz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id":"item-id","@microsoft.graph.downloadUrl":%q}`, server.URL+"/download")
		case "/download":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(sensitiveMarker))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL + "/v1.0/")
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	client := oneDriveClient{client: server.Client(), baseURL: baseURL, token: "synthetic-access-token"}
	_, err = client.Download("backup/file.tar.gz", filepath.Join(t.TempDir(), "download.tar.gz"))
	if err == nil {
		t.Fatal("Download() accepted a non-success response")
	}
	if strings.Contains(err.Error(), sensitiveMarker) {
		t.Fatalf("Download() leaked provider response detail: %v", err)
	}
}

func TestOneDriveUploadRejectsNonSuccessWithoutLeakingBody(t *testing.T) {
	const sensitiveMarker = "upload-provider-detail-must-not-leak"
	var sawContentLength atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"folder-id"}`))
		case http.MethodPut:
			sawContentLength.Store(r.ContentLength == int64(len("payload")) && r.Header.Get("Content-Length") == fmt.Sprint(len("payload")))
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"accessDenied","message":"` + sensitiveMarker + `"}}`))
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL + "/v1.0/")
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	source := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(source, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write upload source: %v", err)
	}
	client := oneDriveClient{client: server.Client(), baseURL: baseURL, token: "synthetic-access-token"}
	_, err = client.Upload(t.Context(), source, "/backup/payload.bin")
	if err == nil {
		t.Fatal("Upload() accepted a non-success response")
	}
	if strings.Contains(err.Error(), sensitiveMarker) {
		t.Fatalf("Upload() leaked provider response detail: %v", err)
	}
	if !sawContentLength.Load() {
		t.Fatal("Upload() did not send the exact simple-upload Content-Length")
	}
}

func TestEscapeDrivePathEscapesEachSegment(t *testing.T) {
	got := escapeDrivePath("/folder name/report#1?.tar")
	want := "/folder%20name/report%231%3F.tar"
	if got != want {
		t.Fatalf("escapeDrivePath() = %q, want %q", got, want)
	}
}

func TestOneDriveDataURLRequiresHTTPS(t *testing.T) {
	for _, rawURL := range []string{
		"http://storage.example/upload",
		"https://user:password@storage.example/upload",
		"//storage.example/upload",
		"not a URL",
	} {
		if err := validateOneDriveDataURL(rawURL); err == nil {
			t.Fatalf("validateOneDriveDataURL(%q) accepted an unsafe URL", rawURL)
		}
	}
	if err := validateOneDriveDataURL("https://storage.example/upload?opaque=token"); err != nil {
		t.Fatalf("validateOneDriveDataURL() rejected HTTPS upload URL: %v", err)
	}
}

func TestOneDriveRedirectPolicyRejectsAllRedirects(t *testing.T) {
	from, err := http.NewRequest(http.MethodGet, "https://storage.example/download", nil)
	if err != nil {
		t.Fatalf("create source request: %v", err)
	}
	toHTTP, err := http.NewRequest(http.MethodGet, "http://storage.example/download", nil)
	if err != nil {
		t.Fatalf("create downgrade request: %v", err)
	}
	if err := oneDriveRedirectPolicy(toHTTP, []*http.Request{from}); err == nil {
		t.Fatal("oneDriveRedirectPolicy() accepted an HTTPS-to-HTTP redirect")
	}
	toHTTPS, err := http.NewRequest(http.MethodGet, "https://cdn.example/download", nil)
	if err != nil {
		t.Fatalf("create HTTPS redirect request: %v", err)
	}
	if err := oneDriveRedirectPolicy(toHTTPS, []*http.Request{from}); err == nil {
		t.Fatal("oneDriveRedirectPolicy() accepted an HTTPS redirect")
	}
}

func TestOneDriveUploadChunkRequiresFinalCompletionStatus(t *testing.T) {
	const sensitiveMarker = "upload-session-detail-must-not-leak"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"nextExpectedRanges":["1-"],"detail":"` + sensitiveMarker + `"}`))
	}))
	defer server.Close()
	client := oneDriveClient{client: server.Client()}

	if err := client.uploadChunk(t.Context(), server.URL, 0, 2, []byte("a")); err != nil {
		t.Fatalf("intermediate 202 upload chunk failed: %v", err)
	}
	err := client.uploadChunk(t.Context(), server.URL, 0, 1, []byte("a"))
	if err == nil {
		t.Fatal("final upload chunk accepted 202 without completion")
	}
	if strings.Contains(err.Error(), sensitiveMarker) {
		t.Fatalf("final upload chunk leaked provider response detail: %v", err)
	}
}

func TestProviderNotFoundClassifiers(t *testing.T) {
	tests := []struct {
		name    string
		missing error
		other   error
		check   func(error) bool
	}{
		{
			name:    "sftp",
			missing: fmt.Errorf("wrapped: %w", os.ErrNotExist),
			other:   os.ErrPermission,
			check:   isSFTPObjectNotFound,
		},
		{
			name:    "webdav",
			missing: webdavHelper.NewPathError("stat", "/missing", http.StatusNotFound),
			other:   webdavHelper.NewPathError("stat", "/denied", http.StatusForbidden),
			check:   isWebDAVObjectNotFound,
		},
		{
			name:    "minio",
			missing: minio.ErrorResponse{Code: minio.NoSuchKey},
			other:   minio.ErrorResponse{Code: "AccessDenied"},
			check:   isMinIOObjectNotFound,
		},
		{
			name:    "kodo",
			missing: fmt.Errorf("wrapped: %w", &qiniuClient.ErrorInfo{Code: 612}),
			other:   &qiniuClient.ErrorInfo{Code: http.StatusForbidden},
			check:   isKodoObjectNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check(tt.missing) {
				t.Fatalf("missing error %T was not recognized", tt.missing)
			}
			if tt.check(tt.other) {
				t.Fatalf("non-missing error %T was misclassified", tt.other)
			}
		})
	}
}

func TestLocalExistTreatsOnlyMissingPathAsAbsent(t *testing.T) {
	client := localClient{}
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	if err := os.WriteFile(existing, []byte("data"), 0o600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	exists, err := client.Exist(existing)
	if err != nil || !exists {
		t.Fatalf("existing file Exist() = (%v, %v)", exists, err)
	}
	exists, err = client.Exist(filepath.Join(root, "missing"))
	if err != nil || exists {
		t.Fatalf("missing file Exist() = (%v, %v)", exists, err)
	}
}

func TestCloudStorageNotFoundSentinelSupportsWrapping(t *testing.T) {
	wrapped := fmt.Errorf("lookup failed: %w", errCloudStorageObjectNotFound)
	if !errors.Is(wrapped, errCloudStorageObjectNotFound) {
		t.Fatal("cloud storage not-found sentinel did not survive wrapping")
	}
}
