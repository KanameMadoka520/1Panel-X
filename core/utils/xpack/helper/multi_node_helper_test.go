package helper

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/backupsync"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/nodepki"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupHelperTestDB(t *testing.T) {
	t.Helper()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	dbName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dbName)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open helper test database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Node{},
		&model.Setting{},
		&model.BackupAccount{},
		&model.BackupOAuthCredential{},
		&model.BackupSyncSequence{},
		&model.BackupSyncOutbox{},
		&model.BackupSyncTarget{},
		&model.BackupSyncTombstone{},
	); err != nil {
		t.Fatalf("migrate helper test database: %v", err)
	}
	if err := backupsync.InitializeTx(db); err != nil {
		t.Fatalf("initialize backup sync state: %v", err)
	}
	global.DB = db
	global.CONF.Base.EncryptKey = "1234567890abcdef"
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
	})
}

func acknowledgeHelperTarget(targetKey string, revision uint64, now time.Time) error {
	sequence, err := backupsync.CurrentSequence()
	if err != nil {
		return err
	}
	target, err := backupsync.GetActiveTarget(targetKey)
	if err != nil {
		return err
	}
	return backupsync.MarkTargetSuccess(targetKey, backupsync.SnapshotIdentity{
		Authority:   sequence.Authority,
		Generation:  sequence.Generation,
		TargetEpoch: target.TargetEpoch,
		Revision:    revision,
		Digest:      fmt.Sprintf("%064x", revision),
	}, now)
}

// seedPKI initialises core's node CA + client cert in settings and returns the
// loaded CA plus core's client fingerprint (what the node pins).
func seedPKI(t *testing.T) (*nodepki.CA, string) {
	t.Helper()
	caCertPEM, caKeyPEM, err := nodepki.GenerateCA("test-node-ca")
	if err != nil {
		t.Fatal(err)
	}
	ca, err := nodepki.LoadCA(caCertPEM, caKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	clientKeyPEM, clientCSR, err := nodepki.GenerateKeyAndCSR("1panel-core", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	clientCertPEM, err := ca.SignLeaf(clientCSR, nodepki.LeafOptions{CommonName: "1panel-core", ForClient: true})
	if err != nil {
		t.Fatal(err)
	}
	clientKeyEnc, err := encrypt.StringEncrypt(string(clientKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		constant.NodeCACertKey:   string(caCertPEM),
		constant.NodeCoreCertKey: string(clientCertPEM),
		constant.NodeCoreKeyKey:  clientKeyEnc,
	} {
		if err := nodeSettingRepo.UpdateOrCreate(k, v); err != nil {
			t.Fatal(err)
		}
	}
	clientFP, _ := nodepki.FingerprintPEM(clientCertPEM)
	return ca, clientFP
}

// startNodeServer runs a loopback mTLS server acting as an enrolled node.
func startNodeServer(t *testing.T, ca *nodepki.CA, coreClientFP string) (addr, port, serverFP string) {
	t.Helper()
	loopback := []net.IP{net.ParseIP("127.0.0.1")}
	keyPEM, csrPEM, err := nodepki.GenerateKeyAndCSR("node", nil, loopback)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, err := ca.SignLeaf(csrPEM, nodepki.LeafOptions{CommonName: "node-1", IPAddresses: loopback, ForServer: true})
	if err != nil {
		t.Fatal(err)
	}
	serverFP, _ = nodepki.FingerprintPEM(certPEM)
	cfg, err := nodepki.ServerTLSConfig(ca.CertPEM, certPEM, keyPEM, coreClientFP)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxy-id:"+r.Header.Get("Proxy-Id"))
	}))
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)

	u := strings.TrimPrefix(srv.URL, "https://")
	host, p, _ := net.SplitHostPort(u)
	return host, p, serverFP
}

func startBackupUsageNodeServer(
	t *testing.T,
	ca *nodepki.CA,
	coreClientFP string,
	handler http.HandlerFunc,
) (addr, port, serverFP string) {
	t.Helper()
	loopback := []net.IP{net.ParseIP("127.0.0.1")}
	keyPEM, csrPEM, err := nodepki.GenerateKeyAndCSR("backup-usage-node", nil, loopback)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, err := ca.SignLeaf(csrPEM, nodepki.LeafOptions{CommonName: "backup-usage-node", IPAddresses: loopback, ForServer: true})
	if err != nil {
		t.Fatal(err)
	}
	serverFP, _ = nodepki.FingerprintPEM(certPEM)
	cfg, err := nodepki.ServerTLSConfig(ca.CertPEM, certPEM, keyPEM, coreClientFP)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)

	u := strings.TrimPrefix(srv.URL, "https://")
	host, p, _ := net.SplitHostPort(u)
	return host, p, serverFP
}

func startBackupSyncNodeServer(
	t *testing.T,
	ca *nodepki.CA,
	coreClientFP, proxyID string,
	beforeAck func() error,
) (addr, port, serverFP string) {
	t.Helper()
	loopback := []net.IP{net.ParseIP("127.0.0.1")}
	keyPEM, csrPEM, err := nodepki.GenerateKeyAndCSR("sync-node", nil, loopback)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, err := ca.SignLeaf(csrPEM, nodepki.LeafOptions{CommonName: "node-sync", IPAddresses: loopback, ForServer: true})
	if err != nil {
		t.Fatal(err)
	}
	serverFP, _ = nodepki.FingerprintPEM(certPEM)
	cfg, err := nodepki.ServerTLSConfig(ca.CertPEM, certPEM, keyPEM, coreClientFP)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/backups/public/sync" || r.Header.Get("Proxy-Id") != proxyID {
			http.Error(w, "invalid synchronization request", http.StatusForbidden)
			return
		}
		var payload struct {
			Authority      string `json:"authority"`
			Generation     string `json:"generation"`
			TargetEpoch    string `json:"targetEpoch"`
			Revision       uint64 `json:"revision"`
			SnapshotDigest string `json:"snapshotDigest"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Authority == "" || payload.Generation == "" || payload.TargetEpoch == "" || payload.Revision == 0 || payload.SnapshotDigest == "" {
			http.Error(w, "invalid synchronization payload", http.StatusBadRequest)
			return
		}
		if beforeAck != nil {
			if err := beforeAck(); err != nil {
				http.Error(w, "synchronization test hook failed", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": http.StatusOK,
			"data": map[string]interface{}{
				"authority":       payload.Authority,
				"generation":      payload.Generation,
				"targetEpoch":     payload.TargetEpoch,
				"appliedRevision": payload.Revision,
				"snapshotDigest":  payload.SnapshotDigest,
				"result":          "applied",
			},
		})
	}))
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)

	u := strings.TrimPrefix(srv.URL, "https://")
	host, p, _ := net.SplitHostPort(u)
	return host, p, serverFP
}

func TestBuildNodeTransportPinsAndConnects(t *testing.T) {
	setupHelperTestDB(t)
	ca, coreClientFP := seedPKI(t)
	addr, port, serverFP := startNodeServer(t, ca, coreClientFP)

	node := model.Node{Addr: addr, Port: port, ServerFingerprint: serverFP, ProxyID: "pid-123"}
	transport, err := buildNodeTransport(node)
	if err != nil {
		t.Fatalf("buildNodeTransport: %v", err)
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	resp, err := client.Get(fmt.Sprintf("https://%s/", net.JoinHostPort(addr, port)))
	if err != nil {
		t.Fatalf("mTLS request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	// wrong pinned fingerprint -> connection must fail (N5)
	bad := model.Node{Addr: addr, Port: port, ServerFingerprint: strings.Repeat("00", 32), ProxyID: "x"}
	badTransport, err := buildNodeTransport(bad)
	if err != nil {
		t.Fatalf("buildNodeTransport(bad): %v", err)
	}
	badClient := &http.Client{Timeout: 5 * time.Second, Transport: badTransport}
	if _, err := badClient.Get(fmt.Sprintf("https://%s/", net.JoinHostPort(addr, port))); err == nil {
		t.Fatal("connection with wrong pinned fingerprint was accepted (N5 violated)")
	}
}

func TestBuildNodeTransportRejectsUnenrolled(t *testing.T) {
	setupHelperTestDB(t)
	seedPKI(t)
	if _, err := buildNodeTransport(model.Node{Name: "x", Addr: "127.0.0.1", Port: "9"}); err == nil {
		t.Fatal("unenrolled node (no fingerprint) should not build a transport")
	}
}

// N10: a revoked node (even with a valid fingerprint) is refused by the dialer.
func TestBuildNodeTransportRejectsRevoked(t *testing.T) {
	setupHelperTestDB(t)
	seedPKI(t)
	revoked := model.Node{Name: "r", Addr: "127.0.0.1", Port: "9", ServerFingerprint: "deadbeef", Status: constant.NodeStatusRevoked}
	if _, err := buildNodeTransport(revoked); err == nil {
		t.Fatal("revoked node must not build a transport")
	}
	offline := model.Node{Name: "o", Addr: "127.0.0.1", Port: "9", ServerFingerprint: "deadbeef", Status: constant.NodeStatusOffline}
	if _, err := buildNodeTransport(offline); err == nil {
		t.Fatal("offline node must not build a transport")
	}
	if _, err := buildNodeTransportWithOffline(offline, true); err != nil {
		t.Fatalf("offline reconciliation transport was rejected: %v", err)
	}
	if _, err := buildNodeTransportWithOffline(revoked, true); err == nil {
		t.Fatal("revoked node must not build a reconciliation transport")
	}
}

func TestResolveNode(t *testing.T) {
	setupHelperTestDB(t)
	n := &model.Node{Name: "web-1", Addr: "10.0.0.7", Port: "9999", Status: constant.NodeStatusOnline}
	if err := nodeRepo.Create(n); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"10.0.0.7", "web-1", fmt.Sprintf("%d", n.ID)} {
		got, err := resolveNode(key)
		if err != nil || got.ID != n.ID {
			t.Fatalf("resolveNode(%q) = %+v, err %v", key, got, err)
		}
	}
	if _, err := resolveNode("nope"); err == nil {
		t.Fatal("resolveNode should fail for an unknown node")
	}
}

func TestCheckBackupUsedUsesPinnedMTLSAndFailsClosedOnAgentResponse(t *testing.T) {
	tests := []struct {
		name       string
		httpStatus int
		body       string
		wantErr    bool
	}{
		{name: "available", httpStatus: http.StatusOK, body: `{"code":200,"message":"success"}`},
		{name: "used by remote schedule", httpStatus: http.StatusOK, body: `{"code":400,"message":"in use"}`, wantErr: true},
		{name: "non success HTTP status", httpStatus: http.StatusServiceUnavailable, body: `{"code":503}`, wantErr: true},
		{name: "invalid response", httpStatus: http.StatusOK, body: `not-json`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupHelperTestDB(t)
			ca, coreClientFP := seedPKI(t)
			const proxyID = "backup-usage-proxy"
			called := make(chan struct{}, 1)
			addr, port, serverFP := startBackupUsageNodeServer(t, ca, coreClientFP, func(w http.ResponseWriter, r *http.Request) {
				called <- struct{}{}
				if r.Method != http.MethodGet || r.URL.Path != "/api/v2/backups/check/shared-account" {
					t.Errorf("unexpected backup usage request %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Proxy-Id") != proxyID {
					t.Errorf("Proxy-Id = %q, want %q", r.Header.Get("Proxy-Id"), proxyID)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.httpStatus)
				_, _ = io.WriteString(w, tt.body)
			})
			if err := global.DB.Create(&model.Node{
				Name:              "checked-node",
				Addr:              addr,
				Port:              port,
				Status:            constant.NodeStatusOnline,
				Enrolled:          true,
				ProxyID:           proxyID,
				ServerFingerprint: serverFP,
			}).Error; err != nil {
				t.Fatalf("create checked node: %v", err)
			}
			if err := global.DB.Create(&model.Node{
				Name:     "unenrolled-node",
				Status:   constant.NodeStatusOnline,
				Enrolled: false,
			}).Error; err != nil {
				t.Fatalf("create unenrolled node: %v", err)
			}
			if err := global.DB.Create(&model.Node{
				Name:     "revoked-node",
				Status:   constant.NodeStatusRevoked,
				Enrolled: true,
			}).Error; err != nil {
				t.Fatalf("create revoked node: %v", err)
			}

			err := NewIMultiNodeProvider().CheckBackupUsed(" shared-account ")
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckBackupUsed() error = %v, wantErr=%v", err, tt.wantErr)
			}
			select {
			case <-called:
			default:
				t.Fatal("registered node did not receive the backup usage check")
			}
		})
	}
}

func TestCheckBackupUsedSkipsOfflineNodesButFailsClosedForUnreachableOnlineNodes(t *testing.T) {
	t.Run("offline is reconciled from the tombstone", func(t *testing.T) {
		setupHelperTestDB(t)
		if err := global.DB.Create(&model.Node{
			Name:     "offline-node",
			Status:   constant.NodeStatusOffline,
			Enrolled: true,
		}).Error; err != nil {
			t.Fatalf("create offline node: %v", err)
		}
		if err := NewIMultiNodeProvider().CheckBackupUsed("shared-account"); err != nil {
			t.Fatalf("offline registered node blocked tombstone creation: %v", err)
		}
	})

	t.Run("online but unreachable fails closed", func(t *testing.T) {
		setupHelperTestDB(t)
		seedPKI(t)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve unreachable port: %v", err)
		}
		addr, port, err := net.SplitHostPort(listener.Addr().String())
		if err != nil {
			t.Fatalf("split unreachable address: %v", err)
		}
		if err := listener.Close(); err != nil {
			t.Fatalf("close reserved port: %v", err)
		}
		if err := global.DB.Create(&model.Node{
			Name:              "unreachable-node",
			Addr:              addr,
			Port:              port,
			Status:            constant.NodeStatusOnline,
			Enrolled:          true,
			ProxyID:           "unreachable-proxy",
			ServerFingerprint: strings.Repeat("ab", 32),
		}).Error; err != nil {
			t.Fatalf("create unreachable node: %v", err)
		}
		if err := NewIMultiNodeProvider().CheckBackupUsed("shared-account"); err == nil {
			t.Fatal("unreachable registered node did not block backup account deletion")
		}
	})
}

func TestEnsureBackupSyncReadyDoesNotUpgradeExecutionGuard(t *testing.T) {
	setupHelperTestDB(t)
	sequence, err := backupsync.CurrentSequence()
	if err != nil {
		t.Fatalf("load initial synchronization sequence: %v", err)
	}
	if err := acknowledgeHelperTarget(model.BackupSyncTargetLocal, sequence.Revision, time.Now()); err != nil {
		t.Fatalf("acknowledge local target: %v", err)
	}
	node := model.Node{
		Name:     "missing-sync-target",
		Status:   constant.NodeStatusOnline,
		Enrolled: true,
	}
	if err := global.DB.Create(&node).Error; err != nil {
		t.Fatalf("create node without synchronization target: %v", err)
	}

	releaseExecution := backupsync.AcquireDesiredStateExecution()
	result := make(chan error, 1)
	go func() {
		result <- NewIMultiNodeProvider().EnsureBackupSyncReady(node.Name)
	}()
	select {
	case err := <-result:
		if err == nil {
			releaseExecution()
			t.Fatal("missing target was treated as ready")
		}
	case <-time.After(2 * time.Second):
		releaseExecution()
		t.Fatal("execution readiness attempted a desired-state lock upgrade")
	}
	releaseExecution()

	current, err := backupsync.CurrentSequence()
	if err != nil {
		t.Fatalf("reload synchronization sequence: %v", err)
	}
	if current.Revision != sequence.Revision {
		t.Fatalf("read-only execution readiness changed revision from %d to %d", sequence.Revision, current.Revision)
	}
	var targetCount int64
	if err := global.DB.Model(&model.BackupSyncTarget{}).
		Where("target_key = ?", backupsync.NodeTargetKey(node.ID)).
		Count(&targetCount).Error; err != nil {
		t.Fatalf("count missing synchronization target: %v", err)
	}
	if targetCount != 0 {
		t.Fatalf("execution readiness created %d synchronization targets", targetCount)
	}
}

func TestEnsureBackupSyncReadyOnlyReconcilesRequestedTarget(t *testing.T) {
	setupHelperTestDB(t)
	ca, coreClientFP := seedPKI(t)
	const requestedProxyID = "requested-target-proxy"
	requestedAddr, requestedPort, requestedFingerprint := startBackupSyncNodeServer(t, ca, coreClientFP, requestedProxyID, nil)

	var unrelatedRequests atomic.Int32
	const unrelatedProxyID = "unrelated-target-proxy"
	unrelatedAddr, unrelatedPort, unrelatedFingerprint := startBackupSyncNodeServer(t, ca, coreClientFP, unrelatedProxyID, func() error {
		unrelatedRequests.Add(1)
		return nil
	})

	requestedNode := model.Node{
		Name:              "requested-target",
		Addr:              requestedAddr,
		Port:              requestedPort,
		Status:            constant.NodeStatusOnline,
		Enrolled:          true,
		ProxyID:           requestedProxyID,
		ServerFingerprint: requestedFingerprint,
	}
	unrelatedNode := model.Node{
		Name:              "unrelated-target",
		Addr:              unrelatedAddr,
		Port:              unrelatedPort,
		Status:            constant.NodeStatusOnline,
		Enrolled:          true,
		ProxyID:           unrelatedProxyID,
		ServerFingerprint: unrelatedFingerprint,
	}
	if err := global.DB.Create(&requestedNode).Error; err != nil {
		t.Fatal(err)
	}
	if err := global.DB.Create(&unrelatedNode).Error; err != nil {
		t.Fatal(err)
	}
	if err := backupsync.EnsureNodeTarget(requestedNode.ID); err != nil {
		t.Fatal(err)
	}
	if err := backupsync.EnsureNodeTarget(unrelatedNode.ID); err != nil {
		t.Fatal(err)
	}

	releaseExecution := backupsync.AcquireDesiredStateExecution()
	err := NewIMultiNodeProvider().EnsureBackupSyncReady(requestedNode.Name)
	releaseExecution()
	if err != nil {
		t.Fatalf("reconcile requested target: %v", err)
	}
	if unrelatedRequests.Load() != 0 {
		t.Fatalf("readiness reconciliation contacted %d unrelated targets", unrelatedRequests.Load())
	}
	ready, err := backupsync.TargetReady(backupsync.NodeTargetKey(requestedNode.ID))
	if err != nil || !ready {
		t.Fatalf("requested target ready=%v err=%v", ready, err)
	}
	ready, err = backupsync.TargetReady(backupsync.NodeTargetKey(unrelatedNode.ID))
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("unrelated target was marked ready by request-scoped reconciliation")
	}
}

func TestEnsureBackupSyncReadyIsNotBlockedByDifferentBackgroundTarget(t *testing.T) {
	setupHelperTestDB(t)
	ca, coreClientFP := seedPKI(t)

	blocked := make(chan struct{})
	releaseBlocked := make(chan struct{})
	var blockedOnce sync.Once
	blockedAddr, blockedPort, blockedFingerprint := startBackupSyncNodeServer(
		t,
		ca,
		coreClientFP,
		"blocked-target-proxy",
		func() error {
			blockedOnce.Do(func() { close(blocked) })
			<-releaseBlocked
			return nil
		},
	)
	fastAddr, fastPort, fastFingerprint := startBackupSyncNodeServer(t, ca, coreClientFP, "fast-target-proxy", nil)
	blockedNode := model.Node{
		Name:              "blocked-target",
		Addr:              blockedAddr,
		Port:              blockedPort,
		Status:            constant.NodeStatusOnline,
		Enrolled:          true,
		ProxyID:           "blocked-target-proxy",
		ServerFingerprint: blockedFingerprint,
	}
	fastNode := model.Node{
		Name:              "fast-target",
		Addr:              fastAddr,
		Port:              fastPort,
		Status:            constant.NodeStatusOnline,
		Enrolled:          true,
		ProxyID:           "fast-target-proxy",
		ServerFingerprint: fastFingerprint,
	}
	if err := global.DB.Create(&blockedNode).Error; err != nil {
		t.Fatal(err)
	}
	if err := global.DB.Create(&fastNode).Error; err != nil {
		t.Fatal(err)
	}
	if err := backupsync.EnsureNodeTarget(blockedNode.ID); err != nil {
		t.Fatal(err)
	}
	if err := backupsync.EnsureNodeTarget(fastNode.ID); err != nil {
		t.Fatal(err)
	}
	sequence, err := backupsync.CurrentSequence()
	if err != nil {
		t.Fatal(err)
	}
	if err := acknowledgeHelperTarget(model.BackupSyncTargetLocal, sequence.Revision, time.Now()); err != nil {
		t.Fatal(err)
	}

	provider := NewIMultiNodeProvider()
	backgroundDone := make(chan error, 1)
	go func() { backgroundDone <- provider.Sync(constant.SyncBackupAccounts) }()
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		close(releaseBlocked)
		t.Fatal("background synchronization did not reach the blocked target")
	}

	readyDone := make(chan error, 1)
	go func() { readyDone <- provider.EnsureBackupSyncReady(fastNode.Name) }()
	select {
	case err := <-readyDone:
		if err != nil {
			close(releaseBlocked)
			t.Fatalf("fast target readiness failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		close(releaseBlocked)
		t.Fatal("fast target readiness waited for an unrelated blocked target")
	}

	close(releaseBlocked)
	select {
	case err := <-backgroundDone:
		if err != nil {
			t.Fatalf("background synchronization after release: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("background synchronization did not finish after release")
	}
}

func TestSyncUsesBoundedConcurrencyAndCoalescesOverlappingBatch(t *testing.T) {
	setupHelperTestDB(t)
	ca, coreClientFP := seedPKI(t)

	releaseTargets := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseTargets) }) }
	defer release()

	started := []chan struct{}{make(chan struct{}), make(chan struct{})}
	proxies := []string{"parallel-target-one", "parallel-target-two"}
	nodes := make([]model.Node, 0, len(started))
	for index := range started {
		index := index
		var startOnce sync.Once
		addr, port, fingerprint := startBackupSyncNodeServer(t, ca, coreClientFP, proxies[index], func() error {
			startOnce.Do(func() { close(started[index]) })
			<-releaseTargets
			return nil
		})
		nodes = append(nodes, model.Node{
			Name:              fmt.Sprintf("parallel-target-%d", index+1),
			Addr:              addr,
			Port:              port,
			Status:            constant.NodeStatusOnline,
			Enrolled:          true,
			ProxyID:           proxies[index],
			ServerFingerprint: fingerprint,
		})
	}
	for index := range nodes {
		if err := global.DB.Create(&nodes[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, node := range nodes {
		if err := backupsync.EnsureNodeTarget(node.ID); err != nil {
			t.Fatal(err)
		}
	}
	sequence, err := backupsync.CurrentSequence()
	if err != nil {
		t.Fatal(err)
	}
	if err := acknowledgeHelperTarget(model.BackupSyncTargetLocal, sequence.Revision, time.Now()); err != nil {
		t.Fatal(err)
	}

	provider := NewIMultiNodeProvider()
	backgroundDone := make(chan error, 1)
	go func() { backgroundDone <- provider.Sync(constant.SyncBackupAccounts) }()
	for index, signal := range started {
		select {
		case <-signal:
		case <-time.After(2 * time.Second):
			release()
			t.Fatalf("parallel synchronization did not start target %d", index+1)
		}
	}

	overlapDone := make(chan error, 1)
	go func() { overlapDone <- provider.Sync(constant.SyncBackupAccounts) }()
	select {
	case err := <-overlapDone:
		if err != nil {
			release()
			t.Fatalf("coalesced overlapping batch: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		release()
		t.Fatal("overlapping reconciliation batch waited instead of coalescing")
	}

	release()
	select {
	case err := <-backgroundDone:
		if err != nil {
			t.Fatalf("parallel synchronization after release: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("parallel synchronization did not finish after release")
	}
}

func TestBuildPublicBackupSyncPayloadDecryptsAndSanitizes(t *testing.T) {
	setupHelperTestDB(t)
	accessKey, err := encrypt.StringEncrypt("synthetic-access-key")
	if err != nil {
		t.Fatal(err)
	}
	accountCredential, err := encrypt.StringEncrypt("synthetic-account-credential")
	if err != nil {
		t.Fatal(err)
	}
	account := model.BackupAccount{
		Name:         "shared-drive",
		Type:         constant.OneDrive,
		IsPublic:     true,
		AccessKey:    accessKey,
		Credential:   accountCredential,
		Vars:         `{"directory":"safe","clientSecret":"remove-me","refresh_token":"remove-me","codeChallenge":"remove-me"}`,
		RememberAuth: true,
	}
	if err := global.DB.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	clientSecret, err := encrypt.StringEncryptGCM("administrator-client-secret", model.BackupOAuthClientSecretEncryptionDomain)
	if err != nil {
		t.Fatal(err)
	}
	refreshToken, err := encrypt.StringEncryptGCM("administrator-refresh-token", model.BackupOAuthRefreshTokenEncryptionDomain)
	if err != nil {
		t.Fatal(err)
	}
	if err := global.DB.Create(&model.BackupOAuthCredential{
		BackupAccountID: account.ID,
		Provider:        model.BackupOAuthProviderMicrosoft,
		ClientID:        "administrator-client-id",
		ClientSecret:    clientSecret,
		RefreshToken:    refreshToken,
		Status:          model.BackupOAuthStatusConfigured,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		_, err := backupsync.EnqueueTx(tx, account.Name, model.BackupSyncOperationCreate)
		return err
	}); err != nil {
		t.Fatalf("enqueue public account snapshot: %v", err)
	}

	payload, err := buildPublicBackupSyncPayload()
	if err != nil {
		t.Fatalf("build public backup sync payload: %v", err)
	}
	if len(payload.Accounts) != 1 || payload.Accounts[0].OAuth == nil {
		t.Fatalf("unexpected sync payload: %#v", payload)
	}
	item := payload.Accounts[0]
	if item.AccessKey != "synthetic-access-key" || item.Credential != "synthetic-account-credential" {
		t.Fatalf("account credentials were not decrypted for the trusted transport")
	}
	if item.OAuth.ClientSecret != "administrator-client-secret" || item.OAuth.RefreshToken != "administrator-refresh-token" {
		t.Fatal("OAuth credentials were not decrypted for the trusted transport")
	}
	for _, forbidden := range []string{"clientSecret", "refresh_token", "codeChallenge", "remove-me"} {
		if strings.Contains(item.Vars, forbidden) {
			t.Fatalf("sync Vars contains %q", forbidden)
		}
	}

	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.BackupOAuthCredential{}).
			Where("backup_account_id = ?", account.ID).
			Update("status", model.BackupOAuthStatusLegacyReconfigurationRequired).Error; err != nil {
			return err
		}
		_, err := backupsync.EnqueueTx(tx, account.Name, model.BackupSyncOperationUpdate)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	payload, err = buildPublicBackupSyncPayload()
	if err != nil {
		t.Fatalf("build legacy public backup sync payload: %v", err)
	}
	if payload.Accounts[0].OAuth.ClientSecret != "" || payload.Accounts[0].OAuth.RefreshToken != "" {
		t.Fatal("retired shared OAuth material was propagated to an agent")
	}
}

func TestSyncReconcilesOfflineNodeAndPersistsAck(t *testing.T) {
	setupHelperTestDB(t)
	ca, coreClientFP := seedPKI(t)
	const proxyID = "offline-reconciliation-proxy"
	addr, port, serverFP := startBackupSyncNodeServer(t, ca, coreClientFP, proxyID, nil)
	node := model.Node{
		Name:              "offline-node",
		Addr:              addr,
		Port:              port,
		Status:            constant.NodeStatusOffline,
		Enrolled:          true,
		ProxyID:           proxyID,
		ServerFingerprint: serverFP,
	}
	if err := global.DB.Create(&node).Error; err != nil {
		t.Fatalf("create offline node: %v", err)
	}
	accessKey, err := encrypt.StringEncrypt("synthetic-access-key")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := encrypt.StringEncrypt("synthetic-credential")
	if err != nil {
		t.Fatal(err)
	}
	if err := global.DB.Create(&model.BackupAccount{
		Name:       "shared-s3",
		Type:       constant.S3,
		IsPublic:   true,
		AccessKey:  accessKey,
		Credential: credential,
		Vars:       "{}",
	}).Error; err != nil {
		t.Fatalf("create public account: %v", err)
	}
	var revision uint64
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		var enqueueErr error
		revision, enqueueErr = backupsync.EnqueueTx(tx, "shared-s3", model.BackupSyncOperationCreate)
		return enqueueErr
	}); err != nil {
		t.Fatalf("enqueue public account snapshot: %v", err)
	}
	if err := acknowledgeHelperTarget(model.BackupSyncTargetLocal, revision, time.Now()); err != nil {
		t.Fatalf("acknowledge local target: %v", err)
	}

	if err := NewIMultiNodeProvider().Sync(constant.SyncBackupAccounts); err != nil {
		t.Fatalf("reconcile offline node: %v", err)
	}
	var target model.BackupSyncTarget
	if err := global.DB.Where("target_key = ?", backupsync.NodeTargetKey(node.ID)).First(&target).Error; err != nil {
		t.Fatalf("load reconciled target: %v", err)
	}
	if target.AppliedRevision != revision || target.Status != model.BackupSyncTargetStatusSynced || target.Attempts != 0 {
		t.Fatalf("unexpected reconciled target: %#v", target)
	}
	var updated model.Node
	if err := global.DB.First(&updated, node.ID).Error; err != nil {
		t.Fatalf("reload reconciled node: %v", err)
	}
	if updated.Status != constant.NodeStatusOnline {
		t.Fatalf("reconciled node status = %q, want online", updated.Status)
	}
}

func TestSyncDoesNotResurrectNodeRevokedWhileAcknowledging(t *testing.T) {
	setupHelperTestDB(t)
	ca, coreClientFP := seedPKI(t)
	const proxyID = "revoked-during-reconciliation"
	var node model.Node
	addr, port, serverFP := startBackupSyncNodeServer(t, ca, coreClientFP, proxyID, func() error {
		if err := global.DB.Model(&model.Node{}).
			Where("id = ?", node.ID).
			Update("status", constant.NodeStatusRevoked).Error; err != nil {
			return err
		}
		return backupsync.DeactivateNodeTarget(node.ID)
	})
	node = model.Node{
		Name:              "revoked-node",
		Addr:              addr,
		Port:              port,
		Status:            constant.NodeStatusOffline,
		Enrolled:          true,
		ProxyID:           proxyID,
		ServerFingerprint: serverFP,
	}
	if err := global.DB.Create(&node).Error; err != nil {
		t.Fatalf("create offline node: %v", err)
	}
	accessKey, err := encrypt.StringEncrypt("synthetic-access-key")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := encrypt.StringEncrypt("synthetic-credential")
	if err != nil {
		t.Fatal(err)
	}
	if err := global.DB.Create(&model.BackupAccount{
		Name:       "shared-revocation-race",
		Type:       constant.S3,
		IsPublic:   true,
		AccessKey:  accessKey,
		Credential: credential,
		Vars:       "{}",
	}).Error; err != nil {
		t.Fatalf("create public account: %v", err)
	}
	revision := uint64(0)
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		var enqueueErr error
		revision, enqueueErr = backupsync.EnqueueTx(tx, "shared-revocation-race", model.BackupSyncOperationCreate)
		return enqueueErr
	}); err != nil {
		t.Fatalf("enqueue public account snapshot: %v", err)
	}
	if err := acknowledgeHelperTarget(model.BackupSyncTargetLocal, revision, time.Now()); err != nil {
		t.Fatalf("acknowledge local target: %v", err)
	}

	if err := NewIMultiNodeProvider().Sync(constant.SyncBackupAccounts); err != nil {
		t.Fatalf("reconcile node revoked during acknowledgement: %v", err)
	}
	var updated model.Node
	if err := global.DB.First(&updated, node.ID).Error; err != nil {
		t.Fatalf("reload revoked node: %v", err)
	}
	if updated.Status != constant.NodeStatusRevoked {
		t.Fatalf("reconciliation resurrected revoked node as %q", updated.Status)
	}
	var target model.BackupSyncTarget
	if err := global.DB.Where("target_key = ?", backupsync.NodeTargetKey(node.ID)).First(&target).Error; err != nil {
		t.Fatalf("load revoked node target: %v", err)
	}
	if target.Active {
		t.Fatalf("revoked node target was reactivated: %#v", target)
	}
}

func TestSendPublicBackupSyncRequiresStructuredRevisionAck(t *testing.T) {
	const authority = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const generation = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const targetEpoch = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	const digest = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sync" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200,"data":{"authority":"`+authority+`","generation":"`+generation+`","targetEpoch":"`+targetEpoch+`","appliedRevision":7,"snapshotDigest":"`+digest+`","result":"applied"}}`)
	}))
	t.Cleanup(server.Close)

	result, err := sendPublicBackupSync(server.Client(), server.URL+"/sync", "", []byte(`{"revision":7}`))
	if err != nil {
		t.Fatalf("send public backup sync: %v", err)
	}
	if result.Authority != authority || result.Generation != generation || result.TargetEpoch != targetEpoch || result.AppliedRevision != 7 || result.SnapshotDigest != digest || result.Result != "applied" {
		t.Fatalf("unexpected acknowledgement: %#v", result)
	}

	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200}`)
	}))
	t.Cleanup(legacy.Close)
	if _, err := sendPublicBackupSync(legacy.Client(), legacy.URL, "", []byte(`{"revision":7}`)); err == nil {
		t.Fatal("legacy response without a revision acknowledgement was accepted")
	}
}

func TestPublicBackupSyncCanonicalDigestGoldenVector(t *testing.T) {
	authorizedAt := time.Date(2026, 8, 4, 12, 0, 0, 123000000, time.FixedZone("test-offset", 8*60*60))
	payload := dto.BackupPublicSync{
		Authority:  strings.Repeat("a", 64),
		Generation: strings.Repeat("b", 64),
		Revision:   42,
		Accounts: []dto.BackupPublicSyncAccount{
			{Name: " zeta ", Type: constant.S3, IsPublic: true, Vars: `{"z":1}`},
			{Name: "alpha", Type: constant.GoogleDrive, IsPublic: true, Vars: `{"a":1}`, OAuth: &dto.BackupOAuthSecretSync{
				Provider:     model.BackupOAuthProviderGoogle,
				ClientID:     "synthetic-client-id",
				ClientSecret: "synthetic-client-secret",
				RefreshToken: "synthetic-refresh-token",
				Status:       model.BackupOAuthStatusConfigured,
				AuthorizedAt: &authorizedAt,
			}},
		},
		Tombstones: []dto.BackupPublicSyncTombstone{
			{Name: " old-zeta ", Revision: 41},
			{Name: "old-alpha", Revision: 40},
		},
	}
	sealed, err := sealPublicBackupSyncPayload(payload)
	if err != nil {
		t.Fatalf("seal canonical payload: %v", err)
	}
	const expectedDigest = "56ba03b9c591e0a6c67153568e75923b195e95e87142c550bee4fddc51d07c45"
	if sealed.SnapshotDigest != expectedDigest {
		t.Fatalf("canonical digest = %s, want %s", sealed.SnapshotDigest, expectedDigest)
	}

	reversed := payload
	reversed.Accounts = []dto.BackupPublicSyncAccount{payload.Accounts[1], payload.Accounts[0]}
	reversed.Tombstones = []dto.BackupPublicSyncTombstone{payload.Tombstones[1], payload.Tombstones[0]}
	resealed, err := sealPublicBackupSyncPayload(reversed)
	if err != nil {
		t.Fatalf("seal reordered payload: %v", err)
	}
	if resealed.SnapshotDigest != sealed.SnapshotDigest {
		t.Fatalf("canonical digest changed with input order: %s != %s", resealed.SnapshotDigest, sealed.SnapshotDigest)
	}
}

func TestValidPublicBackupSyncAcknowledgementRequiresExactTuple(t *testing.T) {
	sent, err := sealPublicBackupSyncPayload(dto.BackupPublicSync{
		Authority:   strings.Repeat("a", 64),
		Generation:  strings.Repeat("b", 64),
		TargetEpoch: strings.Repeat("c", 64),
		Revision:    7,
	})
	if err != nil {
		t.Fatalf("seal sent payload: %v", err)
	}
	valid := dto.BackupPublicSyncResult{
		Authority:       sent.Authority,
		Generation:      sent.Generation,
		TargetEpoch:     sent.TargetEpoch,
		AppliedRevision: sent.Revision,
		SnapshotDigest:  sent.SnapshotDigest,
		Result:          "applied",
	}
	if !validPublicBackupSyncAcknowledgement(sent, valid) {
		t.Fatal("exact acknowledgement was rejected")
	}

	cases := map[string]func(*dto.BackupPublicSyncResult){
		"missing authority":    func(result *dto.BackupPublicSyncResult) { result.Authority = "" },
		"wrong authority":      func(result *dto.BackupPublicSyncResult) { result.Authority = strings.Repeat("c", 64) },
		"wrong generation":     func(result *dto.BackupPublicSyncResult) { result.Generation = strings.Repeat("d", 64) },
		"missing target epoch": func(result *dto.BackupPublicSyncResult) { result.TargetEpoch = "" },
		"wrong target epoch":   func(result *dto.BackupPublicSyncResult) { result.TargetEpoch = strings.Repeat("f", 64) },
		"older revision":       func(result *dto.BackupPublicSyncResult) { result.AppliedRevision-- },
		"newer revision":       func(result *dto.BackupPublicSyncResult) { result.AppliedRevision++ },
		"wrong digest":         func(result *dto.BackupPublicSyncResult) { result.SnapshotDigest = strings.Repeat("e", 64) },
		"missing result":       func(result *dto.BackupPublicSyncResult) { result.Result = "" },
		"stale result":         func(result *dto.BackupPublicSyncResult) { result.Result = "stale_ignored" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			result := valid
			mutate(&result)
			if validPublicBackupSyncAcknowledgement(sent, result) {
				t.Fatalf("invalid acknowledgement accepted: %#v", result)
			}
		})
	}
}
