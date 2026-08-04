package helper_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/utils/nodepki"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack/helper"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAgentDB(t *testing.T) {
	t.Helper()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.BackupPublicSyncState{}); err != nil {
		t.Fatalf("migrate: %v", err)
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

func genTestCert(t *testing.T) *x509.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "core"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// Default single-host posture: only an explicit scope=node + certs enters node mode.
func TestIsProvisionedNode(t *testing.T) {
	cases := []struct {
		scope, crt, root string
		want             bool
	}{
		{"node", "crt", "root", true},
		{"master", "crt", "root", false},
		{"", "", "", false},
		{"node", "", "root", false},
		{"node", "crt", "", false},
		{"node", "", "", false},
	}
	for _, c := range cases {
		if got := helper.IsProvisionedNode(c.scope, c.crt, c.root); got != c.want {
			t.Errorf("IsProvisionedNode(%q,%q,%q)=%v want %v", c.scope, c.crt, c.root, got, c.want)
		}
	}
}

func TestApplyEnrollmentPersists(t *testing.T) {
	setupAgentDB(t)
	if err := global.DB.Create(&model.BackupPublicSyncState{
		ID:              model.BackupPublicSyncStateID,
		Authority:       "old-authority",
		Generation:      "old-generation",
		TargetEpoch:     "old-target-epoch",
		AppliedRevision: 42,
		AppliedDigest:   "old-digest",
	}).Error; err != nil {
		t.Fatalf("seed old sync state: %v", err)
	}
	proxyFile := filepath.Join(t.TempDir(), ".nodeProxyID")
	if err := os.WriteFile(proxyFile, []byte("old-proxy-id"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(proxyFile, 0o666); err != nil {
		t.Fatal(err)
	}
	resp := helper.EnrollResponse{
		ServerCert:            "-----SERVER CERT-----",
		CACert:                "-----CA CERT-----",
		ProxyID:               "proxy-xyz",
		CoreClientFingerprint: "AABBCC",
		BackupSyncAuthority:   strings.Repeat("a", 64),
		BackupSyncGeneration:  strings.Repeat("b", 64),
		BackupSyncTargetEpoch: strings.Repeat("c", 64),
	}
	if err := helper.ApplyEnrollment(resp, []byte("-----NODE KEY-----"), 9101, proxyFile); err != nil {
		t.Fatalf("apply: %v", err)
	}

	settingRepo := repo.NewISettingRepo()
	scope, _ := settingRepo.GetValueByKey("NodeScope")
	crtEnc, _ := settingRepo.GetValueByKey("ServerCrt")
	rootEnc, _ := settingRepo.GetValueByKey("RootCrt")
	if !helper.IsProvisionedNode(scope, crtEnc, rootEnc) {
		t.Fatal("node should read back as provisioned")
	}
	crt, _ := encrypt.StringDecrypt(crtEnc)
	if crt != resp.ServerCert {
		t.Fatalf("server cert not round-tripped: %q", crt)
	}
	fp, _ := settingRepo.GetValueByKey("MasterClientFingerprint")
	if fp != "aabbcc" { // normalised lowercase
		t.Fatalf("master fingerprint = %q", fp)
	}
	if port, _ := settingRepo.GetValueByKey("NodePort"); port != "9101" {
		t.Fatalf("node port = %q", port)
	}
	if data, err := os.ReadFile(proxyFile); err != nil || string(data) != "proxy-xyz" {
		t.Fatalf("proxy id file = %q err %v", string(data), err)
	}
	info, err := os.Stat(proxyFile)
	if err != nil {
		t.Fatalf("stat proxy id file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("proxy id file mode = %v", info.Mode().Perm())
	}
	var syncState model.BackupPublicSyncState
	if err := global.DB.First(&syncState, model.BackupPublicSyncStateID).Error; err != nil {
		t.Fatalf("load reset sync state: %v", err)
	}
	if syncState.Authority != resp.BackupSyncAuthority || syncState.Generation != resp.BackupSyncGeneration || syncState.TargetEpoch != resp.BackupSyncTargetEpoch || syncState.AppliedRevision != 0 || syncState.AppliedDigest != "" || syncState.AppliedAt != nil {
		t.Fatalf("re-enrollment did not reset sync state: %#v", syncState)
	}

	// incomplete response must be rejected
	if err := helper.ApplyEnrollment(helper.EnrollResponse{}, nil, 1, proxyFile); err == nil {
		t.Fatal("incomplete enrollment response accepted")
	}
}

func TestApplyEnrollmentFailurePreservesPreviousSyncGuard(t *testing.T) {
	newResponse := func() helper.EnrollResponse {
		return helper.EnrollResponse{
			ServerCert:            "-----NEW SERVER CERT-----",
			CACert:                "-----NEW CA CERT-----",
			ProxyID:               "new-proxy-id",
			CoreClientFingerprint: "DDEEFF",
			BackupSyncAuthority:   strings.Repeat("c", 64),
			BackupSyncGeneration:  strings.Repeat("d", 64),
			BackupSyncTargetEpoch: strings.Repeat("e", 64),
		}
	}
	seedState := func(t *testing.T) {
		t.Helper()
		if err := global.DB.Create(&model.BackupPublicSyncState{
			ID:              model.BackupPublicSyncStateID,
			Authority:       "old-authority",
			Generation:      "old-generation",
			TargetEpoch:     "old-target-epoch",
			AppliedRevision: 42,
			AppliedDigest:   "old-digest",
		}).Error; err != nil {
			t.Fatalf("seed previous synchronization guard: %v", err)
		}
	}
	assertStatePreserved := func(t *testing.T) {
		t.Helper()
		var state model.BackupPublicSyncState
		if err := global.DB.First(&state, model.BackupPublicSyncStateID).Error; err != nil {
			t.Fatalf("load previous synchronization guard: %v", err)
		}
		if state.Authority != "old-authority" || state.Generation != "old-generation" || state.TargetEpoch != "old-target-epoch" || state.AppliedRevision != 42 || state.AppliedDigest != "old-digest" {
			t.Fatalf("failed re-enrollment cleared previous synchronization guard: %#v", state)
		}
	}

	t.Run("proxy id write failure", func(t *testing.T) {
		setupAgentDB(t)
		seedState(t)
		proxyFile := filepath.Join(t.TempDir(), "missing", ".nodeProxyID")
		if err := helper.ApplyEnrollment(newResponse(), []byte("-----NEW NODE KEY-----"), 9101, proxyFile); err == nil {
			t.Fatal("re-enrollment unexpectedly succeeded when Proxy-Id could not be written")
		}
		assertStatePreserved(t)
	})

	t.Run("node scope persistence failure", func(t *testing.T) {
		setupAgentDB(t)
		seedState(t)
		if err := global.DB.Exec(`
			CREATE TRIGGER reject_node_scope_insert
			BEFORE INSERT ON settings
			WHEN NEW.key = 'NodeScope'
			BEGIN
				SELECT RAISE(ABORT, 'synthetic NodeScope failure');
			END;
		`).Error; err != nil {
			t.Fatalf("install NodeScope failure trigger: %v", err)
		}
		proxyFile := filepath.Join(t.TempDir(), ".nodeProxyID")
		if err := helper.ApplyEnrollment(newResponse(), []byte("-----NEW NODE KEY-----"), 9101, proxyFile); err == nil {
			t.Fatal("re-enrollment unexpectedly succeeded when NodeScope could not be persisted")
		}
		assertStatePreserved(t)
		var settingCount int64
		if err := global.DB.Model(&model.Setting{}).Count(&settingCount).Error; err != nil {
			t.Fatal(err)
		}
		if settingCount != 0 {
			t.Fatalf("failed enrollment committed %d partial settings", settingCount)
		}
	})

	t.Run("proxy id rename failure leaves new epoch fail closed", func(t *testing.T) {
		setupAgentDB(t)
		seedState(t)
		proxyTarget := filepath.Join(t.TempDir(), "proxy-target")
		if err := os.Mkdir(proxyTarget, 0o700); err != nil {
			t.Fatal(err)
		}
		response := newResponse()
		if err := helper.ApplyEnrollment(response, []byte("-----NEW NODE KEY-----"), 9101, proxyTarget); err == nil {
			t.Fatal("re-enrollment unexpectedly replaced a directory with the Proxy-Id file")
		}
		var state model.BackupPublicSyncState
		if err := global.DB.First(&state, model.BackupPublicSyncStateID).Error; err != nil {
			t.Fatal(err)
		}
		if state.Authority != response.BackupSyncAuthority || state.Generation != response.BackupSyncGeneration || state.TargetEpoch != response.BackupSyncTargetEpoch || state.AppliedRevision != 0 || state.AppliedDigest != "" || state.AppliedAt != nil {
			t.Fatalf("rename failure left the previous synchronization lease usable: %#v", state)
		}
		if scope, err := repo.NewISettingRepo().GetValueByKey("NodeScope"); err != nil || scope != "node" {
			t.Fatalf("committed enrollment settings were not retained for retry: scope=%q err=%v", scope, err)
		}
	})
}

func TestValidateCertificatePinsMaster(t *testing.T) {
	setupAgentDB(t)
	cert := genTestCert(t)
	settingRepo := repo.NewISettingRepo()
	_ = settingRepo.UpdateOrCreate("MasterClientFingerprint", nodepki.FingerprintDER(cert.Raw))

	provider := helper.NewIMultiNodeProvider()

	// matching peer cert -> accepted
	if !provider.ValidateCertificate(ctxWithPeer(cert)) {
		t.Fatal("pinned master certificate should be accepted")
	}
	// different peer cert -> rejected (N6)
	if provider.ValidateCertificate(ctxWithPeer(genTestCert(t))) {
		t.Fatal("stranger certificate should be rejected")
	}
	// no TLS peer -> rejected
	noTLS, _ := gin.CreateTestContext(httptest.NewRecorder())
	noTLS.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if provider.ValidateCertificate(noTLS) {
		t.Fatal("request without a TLS peer should be rejected")
	}
}

func ctxWithPeer(cert *x509.Certificate) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	c.Request = req
	return c
}
