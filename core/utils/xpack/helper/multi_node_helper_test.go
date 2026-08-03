package helper

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
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
	if err := db.AutoMigrate(&model.Node{}, &model.Setting{}, &model.BackupAccount{}, &model.BackupOAuthCredential{}); err != nil {
		t.Fatalf("migrate helper test database: %v", err)
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

	if err := global.DB.Model(&model.BackupOAuthCredential{}).
		Where("backup_account_id = ?", account.ID).
		Update("status", model.BackupOAuthStatusLegacyReconfigurationRequired).Error; err != nil {
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

func TestSyncPublicBackupAccountsToNodesReportsPendingAndAggregatesFailures(t *testing.T) {
	nodes := []model.Node{
		{Name: "online-failed", Enrolled: true, Status: constant.NodeStatusOnline},
		{Name: "offline-pending", Enrolled: true, Status: constant.NodeStatusOffline},
		{Name: "online-succeeded", Enrolled: true, Status: constant.NodeStatusOnline},
		{Name: "not-enrolled", Enrolled: false, Status: constant.NodeStatusPending},
	}
	var attempted []string
	err := syncPublicBackupAccountsToNodes(nodes, []byte(`{"accounts":[]}`), func(node model.Node, _ []byte) error {
		attempted = append(attempted, node.Name)
		if node.Name == "online-failed" {
			return fmt.Errorf("synthetic transport failure")
		}
		return nil
	})
	if err == nil {
		t.Fatal("sync unexpectedly succeeded despite an offline and a failed node")
	}
	if strings.Join(attempted, ",") != "online-failed,online-succeeded" {
		t.Fatalf("online node attempts = %#v", attempted)
	}
	message := err.Error()
	for _, expected := range []string{"online-failed", "offline-pending", "synthetic transport failure"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("aggregated sync error does not contain %q: %s", expected, message)
		}
	}
	if strings.Contains(message, "not-enrolled") {
		t.Fatalf("never-enrolled node was incorrectly reported as pending: %s", message)
	}
}
