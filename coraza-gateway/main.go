// Command coraza-gateway is the 1Panel-X community WAF data plane: a loopback
// reverse proxy that runs each request through a real OWASP Coraza + CRS engine
// before forwarding it to the site origin. It is designed to sit BEHIND nginx/
// OpenResty (which stays the public TLS terminator and proxy_passes cleartext
// into this gateway), so the WAF holds no private keys (W8).
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/1Panel-dev/1Panel/coraza-gateway/gateway"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:9000", "loopback listen address (nginx proxy_pass target)")
	upstream := flag.String("upstream", "", "upstream origin base URL, e.g. http://127.0.0.1:8080 (required)")
	modeStr := flag.String("mode", "detection", "detection|block")
	bodyLimit := flag.Int("body-limit", 13<<20, "max inspected request-body bytes")
	auditLog := flag.String("audit-log", "", "path to append JSON attack-event audit records (empty disables)")
	realIP := flag.String("real-ip-header", "X-Real-IP", "trusted header carrying the true client IP set by the front proxy (empty disables)")
	flag.Parse()

	if *upstream == "" {
		log.Fatal("coraza-gateway: -upstream is required")
	}
	origin, err := url.Parse(*upstream)
	if err != nil || origin.Scheme == "" || origin.Host == "" {
		log.Fatalf("coraza-gateway: invalid -upstream %q: %v", *upstream, err)
	}

	mode := gateway.Mode(*modeStr)
	engine, err := gateway.NewEngineWithAudit(mode, *bodyLimit, *auditLog)
	if err != nil {
		log.Fatalf("coraza-gateway: %v", err)
	}

	rp := httputil.NewSingleHostReverseProxy(origin)
	// W2: single, unambiguous HTTP framing to the origin (no h2 upgrade).
	rp.Transport = &http.Transport{ForceAttemptHTTP2: false}
	// W7: never leak the upstream error/topology in the body.
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusBadGateway)
	}
	// W7: strip upstream fingerprinting/banner headers from the public response.
	rp.ModifyResponse = func(resp *http.Response) error {
		gateway.StripFingerprintHeaders(resp.Header)
		return nil
	}

	srv := &http.Server{
		Addr:    *listen,
		Handler: gateway.NewHandler(engine, rp, mode).WithRealIPHeader(*realIP),
		// W3: the public listener must bound slow-loris / header abuse itself.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("coraza-gateway: listening on %s -> %s (mode=%s)", *listen, origin, mode)
	log.Fatal(srv.ListenAndServe())
}
