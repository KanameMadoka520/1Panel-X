package client

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type oauthRoundTripFunc func(*http.Request) (*http.Response, error)

func (f oauthRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestRequestOAuthTokenUsesFormBodyWithoutSecretQuery(t *testing.T) {
	oldClient := oauthTokenHTTPClient
	t.Cleanup(func() { oauthTokenHTTPClient = oldClient })

	oauthTokenHTTPClient = &http.Client{Transport: oauthRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("client_secret") != "" {
			t.Fatal("client secret must not be placed in the request URL")
		}
		if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("unexpected content type: %s", request.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		if form.Get("client_secret") != "synthetic-secret" || form.Get("code_verifier") != "synthetic-verifier" {
			t.Fatalf("required form fields missing: %v", form)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"access","refresh_token":"refresh"}`)),
		}, nil
	})}

	token, err := requestOAuthToken("https://provider.example/token", url.Values{
		"client_secret": {"synthetic-secret"},
		"code_verifier": {"synthetic-verifier"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "access" || token.RefreshToken != "refresh" {
		t.Fatalf("unexpected token result: %#v", token)
	}
}

func TestRequestOAuthTokenRedactsProviderFailure(t *testing.T) {
	oldClient := oauthTokenHTTPClient
	t.Cleanup(func() { oauthTokenHTTPClient = oldClient })

	providerBody := "provider echoed synthetic-secret and synthetic-refresh-token"
	oauthTokenHTTPClient = &http.Client{Transport: oauthRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(providerBody)),
		}, nil
	})}

	_, err := requestOAuthToken("https://provider.example/token", url.Values{"client_secret": {"synthetic-secret"}})
	if err == nil {
		t.Fatal("expected token request failure")
	}
	if strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), "synthetic-refresh-token") {
		t.Fatalf("provider response leaked through error: %v", err)
	}
}

func TestRequestOAuthTokenRejectsNetworkAndOversizedResponses(t *testing.T) {
	oldClient := oauthTokenHTTPClient
	t.Cleanup(func() { oauthTokenHTTPClient = oldClient })

	oauthTokenHTTPClient = &http.Client{Transport: oauthRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("network detail containing synthetic-secret")
	})}
	if _, err := requestOAuthToken("https://provider.example/token", nil); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("network error was not safely redacted: %v", err)
	}

	oauthTokenHTTPClient = &http.Client{Transport: oauthRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", int(oauthTokenResponseLimit)+1))),
		}, nil
	})}
	if _, err := requestOAuthToken("https://provider.example/token", nil); err == nil {
		t.Fatal("expected oversized response rejection")
	}
}
