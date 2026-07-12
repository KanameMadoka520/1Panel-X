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
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
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
	proxyFile := filepath.Join(t.TempDir(), ".nodeProxyID")
	resp := helper.EnrollResponse{
		ServerCert:            "-----SERVER CERT-----",
		CACert:                "-----CA CERT-----",
		ProxyID:               "proxy-xyz",
		CoreClientFingerprint: "AABBCC",
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

	// incomplete response must be rejected
	if err := helper.ApplyEnrollment(helper.EnrollResponse{}, nil, 1, proxyFile); err == nil {
		t.Fatal("incomplete enrollment response accepted")
	}
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
