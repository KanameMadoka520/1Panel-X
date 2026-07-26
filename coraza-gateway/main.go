// Command coraza-gateway is the 1Panel-X community WAF data plane: a loopback
// reverse proxy that runs each request through a real OWASP Coraza + CRS engine
// before forwarding it to the site origin. It is designed to sit BEHIND nginx/
// OpenResty (which stays the public TLS terminator and proxy_passes cleartext
// into this gateway), so the WAF holds no private keys (W8).
//
// It runs in two modes: -upstream fronts a single origin; -config fronts several
// protected sites, routing each request to its origin by Host (unknown Host is
// denied, W12).
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/1Panel-dev/1Panel/coraza-gateway/gateway"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:9000", "loopback listen address (nginx proxy_pass target)")
	upstream := flag.String("upstream", "", "single-site upstream origin base URL, e.g. http://127.0.0.1:8080")
	config := flag.String("config", "", "path to a multi-site routing config (JSON); takes precedence over -upstream")
	modeStr := flag.String("mode", "detection", "detection|block")
	bodyLimit := flag.Int("body-limit", 13<<20, "max inspected request-body bytes")
	auditLog := flag.String("audit-log", "", "path to append JSON attack-event audit records (empty disables)")
	eventLog := flag.String("event-log", "", "path to append JSON records for non-CRS enforcement decisions (empty disables)")
	realIP := flag.String("real-ip-header", "X-Real-IP", "trusted header carrying the true client IP set by the front proxy (empty disables)")
	adminTokenFile := flag.String("admin-token-file", "", "file holding the shared secret for the loopback management API (empty disables it)")
	geoDB := flag.String("geoip-db", "", "path to the MaxMind-format IP address database used by region access control (empty disables it)")
	flag.Parse()

	if *config == "" && *upstream == "" {
		log.Fatal("coraza-gateway: one of -config or -upstream is required")
	}
	if *listen != "127.0.0.1:9000" && os.Getenv("CORAZA_GATEWAY_ALLOW_NONLOOPBACK") != "1" {
		log.Fatal("coraza-gateway: refusing non-loopback listener; set CORAZA_GATEWAY_ALLOW_NONLOOPBACK=1 only for an isolated container network")
	}

	mode := gateway.Mode(*modeStr)

	// Non-CRS decisions (deny lists, unknown Host, oversize bodies, rate limits)
	// are invisible in the Coraza audit log, so they get their own journal next to
	// it — otherwise those blocks would be enforced but unreportable.
	journal := gateway.NewEventJournal(*eventLog)
	defer func() { _ = journal.Close() }()
	// The enforcer outlives every routing table: bans and rate-limit counters must
	// survive a config reload, otherwise saving an unrelated setting would
	// silently un-ban everyone.
	enforcer := gateway.NewEnforcer(journal)

	// A configured-but-unopenable address database is fatal rather than a
	// warning: coming up without it would leave every region policy silently
	// unenforced while the panel reported the gateway ready.
	geo, err := gateway.OpenGeoDB(*geoDB)
	if err != nil {
		log.Fatalf("coraza-gateway: %v", err)
	}
	defer func() { _ = geo.Close() }()

	// The engine reports rule matches to the enforcer, which is the only way the
	// attack-frequency limit can work in detection mode: nothing is interrupted
	// there, so the response status carries no signal at all.
	engine, engineErr := gateway.NewEngineWithObserver(mode, *bodyLimit, *auditLog, enforcer.AttackObserver())
	if engineErr != nil {
		log.Fatalf("coraza-gateway: %v", engineErr)
	}

	var handler http.Handler
	var desc string
	if *config != "" {
		// The routing table is reloaded IN-PROCESS when the agent rewrites the
		// config file. Restarting the container on every policy save would erase
		// this process's in-memory enforcement state, so the container restart is
		// kept as the control plane's fallback, not its normal path.
		build := func(cfg gateway.Config) (*gateway.Router, error) {
			return gateway.NewRouterWithGeo(cfg, engine, mode, *realIP, journal, enforcer, geo)
		}
		live, rerr := gateway.NewReloadableRouter(*config, mode, build)
		if rerr != nil {
			log.Fatalf("coraza-gateway: %v", rerr)
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		go live.Watch(ctx)
		handler = gateway.WithHealthSource(live, live)
		desc = live.Describe()
	} else {
		origin, uerr := url.Parse(*upstream)
		if uerr != nil || origin.Scheme == "" || origin.Host == "" {
			log.Fatalf("coraza-gateway: invalid -upstream %q: %v", *upstream, uerr)
		}
		gatewayHandler := gateway.NewHandler(engine, gateway.NewReverseProxy(origin), mode).WithRealIPHeader(*realIP)
		handler = gateway.WithHealth(gatewayHandler, 1, mode, "")
		desc = origin.String()
	}

	// The management API sits outside the router so no protected site can shadow
	// it, and behind a shared token because the loopback check alone is not a
	// boundary under network_mode host.
	handler = gateway.WithAdmin(handler, enforcer, gateway.ReadAdminToken(*adminTokenFile))

	srv := &http.Server{
		Addr:    *listen,
		Handler: handler,
		// W3: the public listener must bound slow-loris / header abuse itself.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("coraza-gateway: listening on %s -> %s (mode=%s)", *listen, desc, mode)
	log.Fatal(srv.ListenAndServe())
}
