package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	fileutil "github.com/1Panel-dev/1Panel/agent/utils/files"
)

type oneDriveRoundTripFunc func(*http.Request) (*http.Response, error)

func (f oneDriveRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOneDriveSmallUploadUsesDestinationNameAndFailConflict(t *testing.T) {
	var sawUpload atomic.Bool
	var expectedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"id":"folder-id"}`)
		case http.MethodPut:
			if request.URL.Path != "/v1.0/me/drive/items/folder-id:/destination.tar.gz:/content" {
				t.Errorf("unexpected upload path: %s", request.URL.Path)
			}
			if got := request.URL.Query().Get("@microsoft.graph.conflictBehavior"); got != "fail" {
				t.Errorf("conflict behavior = %q, want fail", got)
			}
			if request.ContentLength != int64(len("payload")) {
				t.Errorf("content length = %d", request.ContentLength)
			}
			if got := request.Header.Get("Content-Type"); got == "" || got != expectedContentType {
				t.Errorf("content type = %q, want %q from the local source", got, expectedContentType)
			}
			sawUpload.Store(true)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	source := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(source, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	expectedContentType = fileutil.GetMimeType(source)
	if expectedContentType == "" {
		t.Fatal("test source MIME type is empty")
	}
	client := newOneDriveTestClient(t, server.URL+"/v1.0/", server.Client())
	if ok, err := client.Upload(t.Context(), source, "/backup/destination.tar.gz"); err != nil || !ok {
		t.Fatalf("Upload() = (%v, %v)", ok, err)
	}
	if !sawUpload.Load() {
		t.Fatal("small upload request was not observed")
	}
}

func TestOneDriveUploadRejectsParentTarget(t *testing.T) {
	client := oneDriveClient{}
	if ok, err := client.Upload(t.Context(), "unused", "/backup/.."); err == nil || ok {
		t.Fatalf("Upload() = (%v, %v), want invalid target failure", ok, err)
	}
}

func TestOneDriveUploadSessionUsesDestinationNameAndFailConflict(t *testing.T) {
	const fileSize = 4 * 1024 * 1024
	var sawSession atomic.Bool
	var sawChunk atomic.Bool
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"id":"folder-id"}`)
		case request.Method == http.MethodPost:
			if request.URL.Path != "/v1.0/me/drive/items/folder-id:/destination-large.tar.gz:/createUploadSession" {
				t.Errorf("unexpected upload-session path: %s", request.URL.Path)
			}
			var payload struct {
				Item map[string]string `json:"item"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode upload-session body: %v", err)
			}
			if got := payload.Item["@microsoft.graph.conflictBehavior"]; got != "fail" {
				t.Errorf("conflict behavior = %q, want fail", got)
			}
			sawSession.Store(true)
			_, _ = fmt.Fprintf(w, `{"uploadUrl":%q}`, server.URL+"/upload?opaque=synthetic")
		case request.Method == http.MethodPut && request.URL.Path == "/upload":
			if got := request.Header.Get("Content-Range"); got != fmt.Sprintf("bytes 0-%d/%d", fileSize-1, fileSize) {
				t.Errorf("content range = %q", got)
			}
			sawChunk.Store(true)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	source := filepath.Join(t.TempDir(), "source-large.bin")
	if err := os.WriteFile(source, make([]byte, fileSize), 0o600); err != nil {
		t.Fatal(err)
	}
	httpClient := server.Client()
	httpClient.CheckRedirect = oneDriveRedirectPolicy
	client := newOneDriveTestClient(t, server.URL+"/v1.0/", httpClient)
	if ok, err := client.Upload(t.Context(), source, "/backup/destination-large.tar.gz"); err != nil || !ok {
		t.Fatalf("Upload() = (%v, %v)", ok, err)
	}
	if !sawSession.Load() || !sawChunk.Load() {
		t.Fatalf("upload session observed=%v chunk observed=%v", sawSession.Load(), sawChunk.Load())
	}
}

func TestOneDriveGraphRequestRefusesRedirectWithoutLeakingLocation(t *testing.T) {
	const sensitiveMarker = "signed-query-must-not-leak"
	var followed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirected" {
			followed.Store(true)
			_, _ = io.WriteString(w, `{"id":"should-not-be-read"}`)
			return
		}
		http.Redirect(w, request, "/redirected?signature="+sensitiveMarker, http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	httpClient := server.Client()
	httpClient.CheckRedirect = oneDriveRedirectPolicy
	client := newOneDriveTestClient(t, server.URL+"/v1.0/", httpClient)
	if _, err := client.Exist("backup/file.tar.gz"); err == nil {
		t.Fatal("Exist() followed a Graph redirect")
	} else if strings.Contains(err.Error(), sensitiveMarker) || strings.Contains(err.Error(), "redirected") {
		t.Fatalf("redirect error leaked target URL: %v", err)
	}
	if followed.Load() {
		t.Fatal("Graph redirect target was requested")
	}
}

func TestOneDriveUploadSessionRefusesRedirectWithoutLeakingLocation(t *testing.T) {
	const sensitiveMarker = "upload-signature-must-not-leak"
	var followed atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/sink" {
			followed.Store(true)
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Redirect(w, request, "/sink?signature="+sensitiveMarker, http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	httpClient := server.Client()
	httpClient.CheckRedirect = oneDriveRedirectPolicy
	client := oneDriveClient{client: httpClient}
	err := client.uploadChunk(t.Context(), server.URL+"/upload?opaque="+sensitiveMarker, 0, 1, []byte("x"))
	if err == nil {
		t.Fatal("uploadChunk() followed a redirect")
	}
	if strings.Contains(err.Error(), sensitiveMarker) || strings.Contains(err.Error(), "sink") {
		t.Fatalf("redirect error leaked upload URL: %v", err)
	}
	if followed.Load() {
		t.Fatal("upload redirect target was requested")
	}
}

func TestOneDriveTransportErrorRedactsSignedURL(t *testing.T) {
	const sensitiveMarker = "transport-query-must-not-leak"
	client := oneDriveClient{client: &http.Client{Transport: oneDriveRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("dial https://storage.example/upload?signature=" + sensitiveMarker)
	})}}
	err := client.uploadChunk(t.Context(), "https://storage.example/upload?signature="+sensitiveMarker, 0, 1, []byte("x"))
	if err == nil {
		t.Fatal("uploadChunk() accepted a transport failure")
	}
	if strings.Contains(err.Error(), sensitiveMarker) || strings.Contains(err.Error(), "storage.example") {
		t.Fatalf("transport error leaked signed URL: %v", err)
	}
}

func TestOneDriveDownloadFailurePreservesExistingTarget(t *testing.T) {
	const sensitiveMarker = "download-query-must-not-leak"
	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "backup.tar.gz")
	if err := os.WriteFile(target, []byte("existing-backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse("https://graph.example/v1.0/")
	if err != nil {
		t.Fatal(err)
	}
	client := oneDriveClient{
		baseURL: baseURL,
		token:   "synthetic-access-token",
		client: &http.Client{Transport: oneDriveRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.Host {
			case "graph.example":
				body := fmt.Sprintf(`{"id":"item-id","@microsoft.graph.downloadUrl":%q}`, "https://storage.example/download?signature="+sensitiveMarker)
				return oneDriveResponse(http.StatusOK, body), nil
			case "storage.example":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(&oneDriveFailingReader{payload: []byte("partial-download")}),
				}, nil
			default:
				return nil, errors.New("unexpected host")
			}
		})},
	}

	if ok, err := client.Download("backup/file.tar.gz", target); err == nil || ok {
		t.Fatalf("Download() = (%v, %v), want failure", ok, err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "existing-backup" {
		t.Fatalf("existing target was modified: %q", content)
	}
	temps, err := filepath.Glob(filepath.Join(targetDir, ".backup.tar.gz.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary downloads were not removed: %v", temps)
	}
}

func TestOneDriveCallerCancellationStopsLookup(t *testing.T) {
	started := make(chan struct{})
	client := oneDriveClient{
		baseURL: mustParseOneDriveURL(t, "https://graph.example/v1.0/"),
		token:   "synthetic-access-token",
		client: &http.Client{Transport: oneDriveRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			close(started)
			<-request.Context().Done()
			return nil, request.Context().Err()
		})},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	unusedSource := filepath.Join(t.TempDir(), "unused")
	go func() {
		_, err := client.Upload(ctx, unusedSource, "/backup/file.tar.gz")
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Upload() error = %v, want context.Canceled", err)
	}
}

func TestNewOneDriveClientWithContextCancelsTokenExchange(t *testing.T) {
	oldClient := oauthTokenHTTPClient
	t.Cleanup(func() { oauthTokenHTTPClient = oldClient })
	oauthTokenHTTPClient = &http.Client{Transport: oneDriveRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewOneDriveClientWithContext(ctx, map[string]interface{}{
		"client_id":     "synthetic-client",
		"client_secret": "synthetic-secret",
		"refresh_token": "synthetic-refresh",
		"redirect_uri":  "https://panel.example/oauth/callback",
		"isCN":          "false",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NewOneDriveClientWithContext() error = %v, want context.Canceled", err)
	}
}

func TestOneDriveTokenExchangeUsesFormBodyWithoutSecretQuery(t *testing.T) {
	oldClient := oauthTokenHTTPClient
	t.Cleanup(func() { oauthTokenHTTPClient = oldClient })
	oauthTokenHTTPClient = &http.Client{Transport: oneDriveRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("client_secret") != "" {
			t.Fatal("client secret must not be placed in the token URL")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		if form.Get("client_secret") != "synthetic-secret" || form.Get("refresh_token") != "synthetic-refresh" {
			t.Fatalf("token form is missing required fields: %v", form)
		}
		return oneDriveResponse(http.StatusOK, `{"access_token":"synthetic-access","refresh_token":"rotated-refresh"}`), nil
	})}

	token, err := RefreshTokenWithContext(t.Context(), "refresh_token", "accessToken", oneDriveOAuthTestVars())
	if err != nil {
		t.Fatal(err)
	}
	if token != "synthetic-access" {
		t.Fatalf("access token = %q", token)
	}
}

func TestOneDriveRefreshTokenPreservesExistingTokenWhenProviderDoesNotRotate(t *testing.T) {
	oldClient := oauthTokenHTTPClient
	t.Cleanup(func() { oauthTokenHTTPClient = oldClient })
	oauthTokenHTTPClient = &http.Client{Transport: oneDriveRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return oneDriveResponse(http.StatusOK, `{"access_token":"synthetic-access"}`), nil
	})}

	token, err := RefreshTokenWithContext(t.Context(), "refresh_token", "refreshToken", oneDriveOAuthTestVars())
	if err != nil {
		t.Fatal(err)
	}
	if token != "synthetic-refresh" {
		t.Fatalf("refresh token = %q, want existing token", token)
	}
}

func TestOneDriveTokenExchangeRedactsProviderAndTransportFailures(t *testing.T) {
	const sensitiveMarker = "oauth-detail-must-not-leak"
	oldClient := oauthTokenHTTPClient
	t.Cleanup(func() { oauthTokenHTTPClient = oldClient })

	oauthTokenHTTPClient = &http.Client{Transport: oneDriveRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return oneDriveResponse(http.StatusBadRequest, `{"error":"invalid_grant","detail":"`+sensitiveMarker+`"}`), nil
	})}
	if _, err := RefreshTokenWithContext(t.Context(), "refresh_token", "accessToken", oneDriveOAuthTestVars()); err == nil {
		t.Fatal("token exchange accepted provider rejection")
	} else if strings.Contains(err.Error(), sensitiveMarker) {
		t.Fatalf("provider failure leaked response body: %v", err)
	}

	oauthTokenHTTPClient = &http.Client{Transport: oneDriveRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("dial token endpoint?secret=" + sensitiveMarker)
	})}
	if _, err := RefreshTokenWithContext(t.Context(), "refresh_token", "accessToken", oneDriveOAuthTestVars()); err == nil {
		t.Fatal("token exchange accepted transport failure")
	} else if strings.Contains(err.Error(), sensitiveMarker) {
		t.Fatalf("transport failure leaked request detail: %v", err)
	}
}

func TestOneDriveListObjectsFollowsSameOriginPagination(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.Path, "/root:/backup"):
			_, _ = io.WriteString(w, `{"id":"folder-id"}`)
		case request.URL.Query().Get("page") == "2":
			_, _ = io.WriteString(w, `{"value":[{"name":"second.tar.gz"}]}`)
		default:
			_, _ = fmt.Fprintf(w, `{"value":[{"name":"first.tar.gz"}],"@odata.nextLink":%q}`, server.URL+request.URL.Path+"?page=2")
		}
	}))
	defer server.Close()

	client := newOneDriveTestClient(t, server.URL+"/v1.0/", server.Client())
	items, err := client.ListObjects("backup")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(items, ","); got != "first.tar.gz,second.tar.gz" {
		t.Fatalf("ListObjects() = %q", got)
	}
}

func TestOneDriveRejectsCrossOriginPaginationURL(t *testing.T) {
	client := oneDriveClient{baseURL: mustParseOneDriveURL(t, "https://graph.example/v1.0/")}
	if _, err := client.resolveGraphURL("https://attacker.example/v1.0/me/drive/items"); err == nil {
		t.Fatal("resolveGraphURL() accepted a cross-origin pagination URL")
	}
}

type oneDriveFailingReader struct {
	payload []byte
	sent    bool
}

func (r *oneDriveFailingReader) Read(buffer []byte) (int, error) {
	if r.sent {
		return 0, errors.New("synthetic read failure")
	}
	r.sent = true
	return copy(buffer, r.payload), nil
}

func newOneDriveTestClient(t *testing.T, rawBaseURL string, httpClient *http.Client) oneDriveClient {
	t.Helper()
	return oneDriveClient{
		client:  httpClient,
		baseURL: mustParseOneDriveURL(t, rawBaseURL),
		token:   "synthetic-access-token",
	}
}

func mustParseOneDriveURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func oneDriveResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func oneDriveOAuthTestVars() map[string]interface{} {
	return map[string]interface{}{
		"client_id":     "synthetic-client",
		"client_secret": "synthetic-secret",
		"refresh_token": "synthetic-refresh",
		"redirect_uri":  "https://panel.example/oauth/callback",
		"isCN":          "false",
	}
}
