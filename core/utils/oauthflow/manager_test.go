package oauthflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	testClientID          = "unit-test-client-id-123456"
	testClientSecret      = "unit-test-client-secret"
	testAccountIdentity   = "unit-test-account"
	testRedirectURI       = "https://panel.example.test/api/v1/oauth/callback"
	testAuthorizationCode = "unit-test-authorization-code"
	testAccessToken       = "unit-test-access-token"
	testRefreshToken      = "unit-test-refresh-token"
)

var fixedNow = time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)

type observedRequest struct {
	method        string
	path          string
	rawQuery      string
	contentType   string
	authorization string
	form          url.Values
}

func TestBeginBuildsDeterministicPKCEAndSafeResults(t *testing.T) {
	server, requests := newSuccessProviderServer(t)
	defer server.Close()

	flowBytes := bytes.Repeat([]byte{0x11}, randomTokenBytes)
	stateBytes := bytes.Repeat([]byte{0x22}, randomTokenBytes)
	verifierBytes := bytes.Repeat([]byte{0x33}, randomTokenBytes)
	randomBytes := append(append(append([]byte{}, flowBytes...), stateBytes...), verifierBytes...)
	manager := NewManager(Options{
		HTTPClient: server.Client(),
		Endpoints:  testEndpoints(server.URL),
		Now:        func() time.Time { return fixedNow },
		Random:     bytes.NewReader(randomBytes),
	})

	begin, err := manager.Begin(testBeginInput(ProviderOneDrive, false))
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	expectedFlowID := base64.RawURLEncoding.EncodeToString(flowBytes)
	expectedState := base64.RawURLEncoding.EncodeToString(stateBytes)
	expectedVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	challengeDigest := sha256.Sum256([]byte(expectedVerifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	if begin.FlowID != expectedFlowID {
		t.Fatalf("FlowID = %q, want %q", begin.FlowID, expectedFlowID)
	}
	if !begin.ExpiresAt.Equal(fixedNow.Add(DefaultFlowTTL)) {
		t.Fatalf("ExpiresAt = %s, want %s", begin.ExpiresAt, fixedNow.Add(DefaultFlowTTL))
	}

	authorizationURL := mustParseURL(t, begin.AuthorizationURL)
	query := authorizationURL.Query()
	assertQueryValue(t, query, "state", expectedState)
	assertQueryValue(t, query, "code_challenge", expectedChallenge)
	assertQueryValue(t, query, "code_challenge_method", "S256")
	assertQueryValue(t, query, "client_id", testClientID)
	assertQueryValue(t, query, "redirect_uri", testRedirectURI)
	assertQueryValue(t, query, "response_type", "code")
	assertQueryValue(t, query, "scope", "offline_access Files.ReadWrite.All User.Read")
	assertStringExcludes(t, begin.AuthorizationURL, testClientSecret, expectedVerifier)
	assertJSONExcludes(t, begin, testClientSecret, expectedVerifier, testAccessToken, testRefreshToken)

	complete, err := manager.Complete(callbackURL(testRedirectURI, expectedState, testAuthorizationCode))
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if complete.FlowID != expectedFlowID || complete.Status != "ready" {
		t.Fatalf("Complete() = %+v, want ready result for %q", complete, expectedFlowID)
	}
	assertJSONExcludes(t, complete, testClientSecret, expectedVerifier, testAccessToken, testRefreshToken)

	tokenRequest := receiveRequest(t, requests)
	driveRequest := receiveRequest(t, requests)
	if !strings.HasSuffix(tokenRequest.path, "/token") || !strings.HasSuffix(driveRequest.path, "/drive") {
		t.Fatalf("request paths = %q, %q; want token then drive", tokenRequest.path, driveRequest.path)
	}

	stored, err := manager.Peek(expectedFlowID)
	if err != nil {
		t.Fatalf("Peek() error = %v", err)
	}
	if stored.ClientSecret != testClientSecret || stored.RefreshToken != testRefreshToken {
		t.Fatalf("Peek() did not retain server-side credentials")
	}
	assertJSONExcludes(t, stored, testClientID, testClientSecret, testRedirectURI, testAccessToken, testRefreshToken)
}

func TestAbandonedFlowIsDeletedAtTTLWithoutAnotherRequest(t *testing.T) {
	manager := NewManager(Options{FlowTTL: 25 * time.Millisecond})
	begin := mustBegin(t, manager, testBeginInput(ProviderOneDrive, false))
	deadline := time.Now().Add(time.Second)
	for {
		_, err := manager.Peek(begin.FlowID)
		if errors.Is(err, ErrFlowNotFound) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("abandoned OAuth flow was not deleted at TTL: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestProviderEndpointSelection(t *testing.T) {
	tests := []struct {
		name          string
		provider      Provider
		isCN          bool
		authorizePath string
		tokenPath     string
		drivePath     string
	}{
		{name: "OneDrive global", provider: ProviderOneDrive, authorizePath: "/onedrive/authorize", tokenPath: "/onedrive/token", drivePath: "/onedrive/drive"},
		{name: "OneDrive China", provider: ProviderOneDrive, isCN: true, authorizePath: "/onedrive-china/authorize", tokenPath: "/onedrive-china/token", drivePath: "/onedrive-china/drive"},
		{name: "Google Drive", provider: ProviderGoogleDrive, authorizePath: "/google/authorize", tokenPath: "/google/token", drivePath: "/google/drive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := newSuccessProviderServer(t)
			defer server.Close()
			manager := NewManager(Options{HTTPClient: server.Client(), Endpoints: testEndpoints(server.URL)})

			begin, err := manager.Begin(testBeginInput(tt.provider, tt.isCN))
			if err != nil {
				t.Fatalf("Begin() error = %v", err)
			}
			if got := mustParseURL(t, begin.AuthorizationURL).Path; got != tt.authorizePath {
				t.Fatalf("authorization path = %q, want %q", got, tt.authorizePath)
			}
			state := mustParseURL(t, begin.AuthorizationURL).Query().Get("state")
			if _, err := manager.Complete(callbackURL(testRedirectURI, state, testAuthorizationCode)); err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if got := receiveRequest(t, requests).path; got != tt.tokenPath {
				t.Fatalf("token path = %q, want %q", got, tt.tokenPath)
			}
			if got := receiveRequest(t, requests).path; got != tt.drivePath {
				t.Fatalf("drive path = %q, want %q", got, tt.drivePath)
			}
		})
	}
}

func TestStateIsValidatedExpiresAndCannotBeReplayed(t *testing.T) {
	t.Run("wrong state does not consume valid flow", func(t *testing.T) {
		server, _ := newSuccessProviderServer(t)
		defer server.Close()
		manager := NewManager(Options{HTTPClient: server.Client(), Endpoints: testEndpoints(server.URL)})
		begin := mustBegin(t, manager, testBeginInput(ProviderOneDrive, false))
		state := mustParseURL(t, begin.AuthorizationURL).Query().Get("state")

		_, err := manager.Complete(callbackURL(testRedirectURI, "wrong-state", testAuthorizationCode))
		if !errors.Is(err, ErrInvalidState) {
			t.Fatalf("Complete(wrong state) error = %v, want %v", err, ErrInvalidState)
		}
		if _, err := manager.Complete(callbackURL(testRedirectURI, state, testAuthorizationCode)); err != nil {
			t.Fatalf("Complete(valid state) error = %v", err)
		}
	})

	t.Run("expired state is removed", func(t *testing.T) {
		server, _ := newSuccessProviderServer(t)
		defer server.Close()
		now := fixedNow
		manager := NewManager(Options{
			HTTPClient: server.Client(), Endpoints: testEndpoints(server.URL),
			Now: func() time.Time { return now }, FlowTTL: time.Minute,
		})
		begin := mustBegin(t, manager, testBeginInput(ProviderOneDrive, false))
		state := mustParseURL(t, begin.AuthorizationURL).Query().Get("state")
		now = now.Add(time.Minute)

		_, err := manager.Complete(callbackURL(testRedirectURI, state, testAuthorizationCode))
		if !errors.Is(err, ErrFlowExpired) {
			t.Fatalf("Complete(expired) error = %v, want %v", err, ErrFlowExpired)
		}
		_, err = manager.Complete(callbackURL(testRedirectURI, state, testAuthorizationCode))
		if !errors.Is(err, ErrInvalidState) {
			t.Fatalf("Complete(expired replay) error = %v, want %v", err, ErrInvalidState)
		}
	})

	t.Run("successful callback is one time", func(t *testing.T) {
		server, _ := newSuccessProviderServer(t)
		defer server.Close()
		manager := NewManager(Options{HTTPClient: server.Client(), Endpoints: testEndpoints(server.URL)})
		begin := mustBegin(t, manager, testBeginInput(ProviderOneDrive, false))
		state := mustParseURL(t, begin.AuthorizationURL).Query().Get("state")
		callback := callbackURL(testRedirectURI, state, testAuthorizationCode)
		if _, err := manager.Complete(callback); err != nil {
			t.Fatalf("first Complete() error = %v", err)
		}
		if _, err := manager.Complete(callback); !errors.Is(err, ErrInvalidState) {
			t.Fatalf("replayed Complete() error = %v, want %v", err, ErrInvalidState)
		}
	})
}

func TestConcurrentCallbackCanOnlyCompleteOnce(t *testing.T) {
	server, _ := newSuccessProviderServer(t)
	defer server.Close()
	manager := NewManager(Options{HTTPClient: server.Client(), Endpoints: testEndpoints(server.URL)})
	begin := mustBegin(t, manager, testBeginInput(ProviderOneDrive, false))
	state := mustParseURL(t, begin.AuthorizationURL).Query().Get("state")
	callback := callbackURL(testRedirectURI, state, testAuthorizationCode)

	start := make(chan struct{})
	results := make(chan error, 2)
	for index := 0; index < 2; index++ {
		go func() {
			<-start
			_, err := manager.Complete(callback)
			results <- err
		}()
	}
	close(start)

	succeeded := 0
	rejected := 0
	for index := 0; index < 2; index++ {
		switch err := <-results; {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrInvalidState):
			rejected++
		default:
			t.Fatalf("concurrent Complete() error = %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent results: success=%d invalid-state=%d, want 1 and 1", succeeded, rejected)
	}
}

func TestCallbackValidationConsumesClaimedFlow(t *testing.T) {
	tests := []struct {
		name     string
		callback func(state string) string
		wantErr  error
	}{
		{
			name: "missing code",
			callback: func(state string) string {
				return testRedirectURI + "?state=" + url.QueryEscape(state)
			},
			wantErr: ErrInvalidCallback,
		},
		{
			name: "duplicate code",
			callback: func(state string) string {
				query := url.Values{"state": {state}, "code": {"first-code", "second-code"}}
				return testRedirectURI + "?" + query.Encode()
			},
			wantErr: ErrInvalidCallback,
		},
		{
			name: "provider denied",
			callback: func(state string) string {
				query := url.Values{"state": {state}, "error": {"access_denied"}, "error_description": {testClientSecret}}
				return testRedirectURI + "?" + query.Encode()
			},
			wantErr: ErrProviderDenied,
		},
		{
			name: "redirect URI mismatch",
			callback: func(state string) string {
				return callbackURL("https://other.example.test/api/v1/oauth/callback", state, testAuthorizationCode)
			},
			wantErr: ErrInvalidCallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := newSuccessProviderServer(t)
			defer server.Close()
			manager := NewManager(Options{HTTPClient: server.Client(), Endpoints: testEndpoints(server.URL)})
			begin := mustBegin(t, manager, testBeginInput(ProviderOneDrive, false))
			state := mustParseURL(t, begin.AuthorizationURL).Query().Get("state")

			_, err := manager.Complete(tt.callback(state))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Complete() error = %v, want %v", err, tt.wantErr)
			}
			assertStringExcludes(t, err.Error(), testClientSecret, testAccessToken, testRefreshToken)
			if len(requests) != 0 {
				t.Fatalf("provider received %d request(s) for rejected callback", len(requests))
			}
			if _, err := manager.Complete(callbackURL(testRedirectURI, state, testAuthorizationCode)); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("Complete(after rejection) error = %v, want %v", err, ErrInvalidState)
			}
		})
	}
}

func TestTokenExchangeAndDriveValidationKeepSecretsOutOfURLs(t *testing.T) {
	server, requests := newSuccessProviderServer(t)
	defer server.Close()
	verifierBytes := bytes.Repeat([]byte{0x66}, randomTokenBytes)
	randomBytes := append(append(append([]byte{}, bytes.Repeat([]byte{0x44}, randomTokenBytes)...), bytes.Repeat([]byte{0x55}, randomTokenBytes)...), verifierBytes...)
	expectedVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	manager := NewManager(Options{
		HTTPClient: server.Client(), Endpoints: testEndpoints(server.URL),
		Random: bytes.NewReader(randomBytes),
	})
	begin := mustBegin(t, manager, testBeginInput(ProviderOneDrive, false))
	state := mustParseURL(t, begin.AuthorizationURL).Query().Get("state")
	if _, err := manager.Complete(callbackURL(testRedirectURI, state, testAuthorizationCode)); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	tokenRequest := receiveRequest(t, requests)
	if tokenRequest.method != http.MethodPost {
		t.Fatalf("token method = %q, want POST", tokenRequest.method)
	}
	if tokenRequest.contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("token Content-Type = %q", tokenRequest.contentType)
	}
	assertQueryValue(t, tokenRequest.form, "client_id", testClientID)
	assertQueryValue(t, tokenRequest.form, "client_secret", testClientSecret)
	assertQueryValue(t, tokenRequest.form, "code", testAuthorizationCode)
	assertQueryValue(t, tokenRequest.form, "redirect_uri", testRedirectURI)
	assertQueryValue(t, tokenRequest.form, "code_verifier", expectedVerifier)
	assertQueryValue(t, tokenRequest.form, "grant_type", "authorization_code")
	assertStringExcludes(t, tokenRequest.rawQuery, testClientSecret, expectedVerifier, testAuthorizationCode)

	driveRequest := receiveRequest(t, requests)
	if driveRequest.method != http.MethodGet {
		t.Fatalf("drive method = %q, want GET", driveRequest.method)
	}
	if driveRequest.authorization != "Bearer "+testAccessToken {
		t.Fatalf("drive Authorization header = %q", driveRequest.authorization)
	}
	assertStringExcludes(t, driveRequest.rawQuery, testAccessToken, testRefreshToken, testClientSecret)
}

func TestPeekAndConsumeLifecycle(t *testing.T) {
	server, _ := newSuccessProviderServer(t)
	defer server.Close()
	manager := NewManager(Options{HTTPClient: server.Client(), Endpoints: testEndpoints(server.URL)})
	begin := mustBegin(t, manager, testBeginInput(ProviderGoogleDrive, false))
	if _, err := manager.Peek(begin.FlowID); !errors.Is(err, ErrFlowNotReady) {
		t.Fatalf("Peek(pending) error = %v, want %v", err, ErrFlowNotReady)
	}
	state := mustParseURL(t, begin.AuthorizationURL).Query().Get("state")
	if _, err := manager.Complete(callbackURL(testRedirectURI, state, testAuthorizationCode)); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	peeked, err := manager.Peek(begin.FlowID)
	if err != nil {
		t.Fatalf("Peek(ready) error = %v", err)
	}
	consumed, err := manager.Consume(begin.FlowID)
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if consumed != peeked {
		t.Fatalf("Consume() = %+v, want %+v", consumed, peeked)
	}
	if _, err := manager.Consume(begin.FlowID); !errors.Is(err, ErrFlowNotFound) {
		t.Fatalf("second Consume() error = %v, want %v", err, ErrFlowNotFound)
	}
}

func TestProviderFailuresAreBoundedSanitizedAndDeleteFlow(t *testing.T) {
	tests := []struct {
		name      string
		stage     string
		behavior  string
		wantError error
	}{
		{name: "token 4xx", stage: "token", behavior: "status", wantError: ErrTokenExchange},
		{name: "token oversized response", stage: "token", behavior: "oversized", wantError: ErrTokenExchange},
		{name: "token redirect", stage: "token", behavior: "redirect", wantError: ErrTokenExchange},
		{name: "drive 4xx", stage: "drive", behavior: "status", wantError: ErrDriveValidation},
		{name: "drive oversized response", stage: "drive", behavior: "oversized", wantError: ErrDriveValidation},
		{name: "drive redirect", stage: "drive", behavior: "redirect", wantError: ErrDriveValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				stage := ""
				switch {
				case strings.HasSuffix(request.URL.Path, "/token"):
					stage = "token"
				case strings.HasSuffix(request.URL.Path, "/drive"):
					stage = "drive"
				default:
					response.WriteHeader(http.StatusTeapot)
					return
				}
				if stage == tt.stage {
					switch tt.behavior {
					case "status":
						response.WriteHeader(http.StatusBadRequest)
						_, _ = io.WriteString(response, testClientSecret+" "+testAccessToken+" "+testRefreshToken)
						return
					case "oversized":
						_, _ = io.WriteString(response, strings.Repeat(testClientSecret+testAccessToken+testRefreshToken, 8))
						return
					case "redirect":
						http.Redirect(response, request, "/redirect-target?leak="+url.QueryEscape(testAccessToken), http.StatusFound)
						return
					}
				}
				if stage == "token" {
					response.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(response, `{"access_token":"`+testAccessToken+`","refresh_token":"`+testRefreshToken+`"}`)
					return
				}
				_, _ = io.WriteString(response, `{}`)
			}))
			defer server.Close()

			manager := NewManager(Options{
				HTTPClient: server.Client(), Endpoints: testEndpoints(server.URL), MaxResponseBytes: 256,
			})
			begin := mustBegin(t, manager, testBeginInput(ProviderOneDrive, false))
			state := mustParseURL(t, begin.AuthorizationURL).Query().Get("state")
			callback := callbackURL(testRedirectURI, state, testAuthorizationCode)
			_, err := manager.Complete(callback)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Complete() error = %v, want %v", err, tt.wantError)
			}
			assertStringExcludes(t, err.Error(), testClientSecret, testAccessToken, testRefreshToken, testAuthorizationCode)
			if _, err := manager.Peek(begin.FlowID); !errors.Is(err, ErrFlowNotFound) {
				t.Fatalf("Peek(after failure) error = %v, want %v", err, ErrFlowNotFound)
			}
			if _, err := manager.Complete(callback); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("Complete(replay after failure) error = %v, want %v", err, ErrInvalidState)
			}
		})
	}
}

func TestRandomSourceFailureIsSanitized(t *testing.T) {
	manager := NewManager(Options{Random: failingReader{}})
	_, err := manager.Begin(testBeginInput(ProviderOneDrive, false))
	if !errors.Is(err, ErrRandomSource) {
		t.Fatalf("Begin() error = %v, want %v", err, ErrRandomSource)
	}
	assertStringExcludes(t, err.Error(), testClientSecret)
}

func TestBeginRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*BeginInput)
	}{
		{name: "missing client ID", mutate: func(input *BeginInput) { input.ClientID = "" }},
		{name: "missing client secret", mutate: func(input *BeginInput) { input.ClientSecret = "" }},
		{name: "missing account identity", mutate: func(input *BeginInput) { input.AccountIdentity = "" }},
		{name: "non TLS remote redirect", mutate: func(input *BeginInput) { input.RedirectURI = "http://panel.example.test/callback" }},
		{name: "redirect userinfo", mutate: func(input *BeginInput) { input.RedirectURI = "https://user:pass@panel.example.test/callback" }},
		{name: "redirect fragment", mutate: func(input *BeginInput) { input.RedirectURI = testRedirectURI + "#fragment" }},
		{name: "redirect reserved query", mutate: func(input *BeginInput) { input.RedirectURI = testRedirectURI + "?state=fixed" }},
		{name: "Google China mode", mutate: func(input *BeginInput) { input.Provider = ProviderGoogleDrive; input.IsCN = true }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := testBeginInput(ProviderOneDrive, false)
			tt.mutate(&input)
			if _, err := NewManager(Options{}).Begin(input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Begin() error = %v, want %v", err, ErrInvalidInput)
			}
		})
	}

	loopback := testBeginInput(ProviderOneDrive, false)
	loopback.RedirectURI = "http://127.0.0.1:8080/oauth/callback"
	if _, err := NewManager(Options{}).Begin(loopback); err != nil {
		t.Fatalf("Begin(loopback redirect) error = %v", err)
	}
}

func TestDefaultEndpointsUseTLS(t *testing.T) {
	endpoints := DefaultEndpoints()
	for name, endpoint := range map[string]EndpointSet{
		"OneDrive": endpoints.OneDrive, "OneDrive China": endpoints.OneDriveChina, "Google Drive": endpoints.GoogleDrive,
	} {
		for kind, raw := range map[string]string{"authorize": endpoint.AuthorizationURL, "token": endpoint.TokenURL, "drive": endpoint.DriveURL} {
			parsed := mustParseURL(t, raw)
			if parsed.Scheme != "https" {
				t.Errorf("%s %s endpoint scheme = %q, want https", name, kind, parsed.Scheme)
			}
			assertStringExcludes(t, raw, testClientSecret, testAccessToken, testRefreshToken)
		}
	}
}

func TestCustomProviderEndpointsRequireTLSOrLoopback(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Endpoints)
	}{
		{name: "authorization", mutate: func(endpoints *Endpoints) {
			endpoints.OneDrive.AuthorizationURL = "http://provider.example.test/authorize"
		}},
		{name: "token", mutate: func(endpoints *Endpoints) { endpoints.OneDrive.TokenURL = "http://provider.example.test/token" }},
		{name: "drive", mutate: func(endpoints *Endpoints) { endpoints.OneDrive.DriveURL = "http://provider.example.test/drive" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoints := DefaultEndpoints()
			tt.mutate(&endpoints)
			manager := NewManager(Options{Endpoints: endpoints})
			if _, err := manager.Begin(testBeginInput(ProviderOneDrive, false)); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Begin() error = %v, want %v", err, ErrInvalidInput)
			}
		})
	}
}

func newSuccessProviderServer(t *testing.T) (*httptest.Server, chan observedRequest) {
	t.Helper()
	requests := make(chan observedRequest, 16)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		observed := observedRequest{
			method: request.Method, path: request.URL.Path, rawQuery: request.URL.RawQuery,
			contentType: request.Header.Get("Content-Type"), authorization: request.Header.Get("Authorization"),
		}
		switch {
		case strings.HasSuffix(request.URL.Path, "/token"):
			if err := request.ParseForm(); err != nil {
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			observed.form = cloneValues(request.PostForm)
			requests <- observed
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{"access_token":"`+testAccessToken+`","refresh_token":"`+testRefreshToken+`"}`)
		case strings.HasSuffix(request.URL.Path, "/drive"):
			requests <- observed
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{}`)
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	return server, requests
}

func testEndpoints(baseURL string) Endpoints {
	return Endpoints{
		OneDrive: EndpointSet{
			AuthorizationURL: baseURL + "/onedrive/authorize",
			TokenURL:         baseURL + "/onedrive/token",
			DriveURL:         baseURL + "/onedrive/drive?fields=id",
		},
		OneDriveChina: EndpointSet{
			AuthorizationURL: baseURL + "/onedrive-china/authorize",
			TokenURL:         baseURL + "/onedrive-china/token",
			DriveURL:         baseURL + "/onedrive-china/drive?fields=id",
		},
		GoogleDrive: EndpointSet{
			AuthorizationURL: baseURL + "/google/authorize",
			TokenURL:         baseURL + "/google/token",
			DriveURL:         baseURL + "/google/drive?fields=user",
		},
	}
}

func testBeginInput(provider Provider, isCN bool) BeginInput {
	return BeginInput{
		Provider: provider, ClientID: testClientID, ClientSecret: testClientSecret,
		RedirectURI: testRedirectURI, IsCN: isCN, AccountIdentity: testAccountIdentity,
	}
}

func mustBegin(t *testing.T, manager *Manager, input BeginInput) BeginResult {
	t.Helper()
	result, err := manager.Begin(input)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	return result
}

func callbackURL(redirectURI, state, code string) string {
	parsed, _ := url.Parse(redirectURI)
	query := parsed.Query()
	query.Set("state", state)
	query.Set("code", code)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return parsed
}

func assertQueryValue(t *testing.T, values url.Values, key, want string) {
	t.Helper()
	if got := values.Get(key); got != want {
		t.Fatalf("query/form %q = %q, want %q", key, got, want)
	}
}

func assertJSONExcludes(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	assertStringExcludes(t, string(encoded), forbidden...)
}

func assertStringExcludes(t *testing.T, value string, forbidden ...string) {
	t.Helper()
	for _, marker := range forbidden {
		if marker != "" && strings.Contains(value, marker) {
			t.Fatalf("value contains sensitive marker %q: %q", marker, value)
		}
	}
}

func receiveRequest(t *testing.T, requests <-chan observedRequest) observedRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider request")
		return observedRequest{}
	}
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, entries := range values {
		cloned[key] = append([]string(nil), entries...)
	}
	return cloned
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("unit-test random failure containing " + testClientSecret)
}
