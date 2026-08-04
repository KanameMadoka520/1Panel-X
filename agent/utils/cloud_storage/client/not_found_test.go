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
	"testing"

	webdavHelper "github.com/1Panel-dev/1Panel/agent/utils/cloud_storage/client/helper/webdav"
	odsdk "github.com/goh-chunlin/go-onedrive/onedrive"
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			sdkClient := odsdk.NewClient(server.Client())
			baseURL, err := url.Parse(server.URL + "/v1.0/")
			if err != nil {
				t.Fatalf("parse test URL: %v", err)
			}
			sdkClient.BaseURL = baseURL
			client := oneDriveClient{client: *sdkClient, httpClient: server.Client()}

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
