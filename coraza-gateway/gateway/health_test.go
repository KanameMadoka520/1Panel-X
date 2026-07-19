package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithHealthReportsReadyWithoutCallingSiteHandler(t *testing.T) {
	called := false
	site := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})
	h := WithHealth(site, 3, ModeBlock)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || called {
		t.Fatalf("health response=%d site-called=%v", rr.Code, called)
	}
	var body Health
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ready" || body.Sites != 3 || body.Mode != ModeBlock {
		t.Fatalf("unexpected health body: %#v", body)
	}

	req = httptest.NewRequest(http.MethodGet, "http://site.example/", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTeapot || !called {
		t.Fatalf("site handler response=%d called=%v", rr.Code, called)
	}
}
