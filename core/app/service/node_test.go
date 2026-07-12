package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/nodepki"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupNodeTestDB(t *testing.T) {
	t.Helper()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	dbName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dbName)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open node test database: %v", err)
	}
	if err := db.AutoMigrate(&model.Node{}, &model.Setting{}); err != nil {
		t.Fatalf("migrate node test database: %v", err)
	}
	global.DB = db
	global.CONF.Base.EncryptKey = "1234567890abcdef" // 16 bytes for AES
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
	})
}

// N14: the node address must be a bare host/IP — no scheme, creds, path, space.
func TestValidateNodeAddr(t *testing.T) {
	for _, ok := range []string{"1.2.3.4", "10.0.0.5", "::1", "node.example.com", "host-1"} {
		if err := validateNodeAddr(ok); err != nil {
			t.Errorf("addr %q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", " ", "http://1.2.3.4", "1.2.3.4/path", "user@1.2.3.4",
		"1.2.3.4 8080", "a b", "1.2.3.4?x=1", "ho\nst", "a//b",
	} {
		if err := validateNodeAddr(bad); err == nil {
			t.Errorf("addr %q should be rejected", bad)
		}
	}
}

func TestValidateNodePort(t *testing.T) {
	for _, ok := range []string{"1", "22", "9999", "65535"} {
		if err := validateNodePort(ok); err != nil {
			t.Errorf("port %q should be valid: %v", ok, err)
		}
	}
	// note: surrounding whitespace is trimmed before validation, so " 22" is valid
	for _, bad := range []string{"", "0", "70000", "-1", "22a", "abc", "2 2"} {
		if err := validateNodePort(bad); err == nil {
			t.Errorf("port %q should be rejected", bad)
		}
	}
}

// N1: a redeemed enrollment token cannot be replayed (single-use atomic burn).
func TestNodeEnrollSingleUse(t *testing.T) {
	setupNodeTestDB(t)
	svc := NewINodeService()

	tok, err := svc.Create(dto.NodeCreate{Name: "web-1", Addr: "127.0.0.1", Port: "9999"})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if tok.Token == "" || tok.NodeID == 0 {
		t.Fatal("empty enrollment token")
	}

	_, csrPEM, err := nodepki.GenerateKeyAndCSR("whatever", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := svc.Enroll(dto.NodeEnrollRequest{Token: tok.Token, CSR: string(csrPEM)})
	if err != nil {
		t.Fatalf("first enroll should succeed: %v", err)
	}
	if resp.ServerCert == "" || resp.CACert == "" || resp.ProxyID == "" || resp.CoreClientFingerprint == "" {
		t.Fatalf("incomplete enroll response: %+v", resp)
	}
	// the node must now be online
	node, err := nodeRepo.Get(repo.WithByID(tok.NodeID))
	if err != nil || node.Status != "online" || node.ServerFingerprint == "" {
		t.Fatalf("node not marked online with pinned fp: %+v (err %v)", node, err)
	}

	// replay the SAME token -> must be rejected
	if _, err := svc.Enroll(dto.NodeEnrollRequest{Token: tok.Token, CSR: string(csrPEM)}); err == nil {
		t.Fatal("replayed enrollment token was accepted (N1 violated)")
	}
}

// N2: a token whose signature was tampered is rejected.
func TestNodeEnrollRejectsForgedToken(t *testing.T) {
	setupNodeTestDB(t)
	svc := NewINodeService()
	tok, err := svc.Create(dto.NodeCreate{Name: "web-2", Addr: "127.0.0.1", Port: "9999"})
	if err != nil {
		t.Fatal(err)
	}
	_, csrPEM, _ := nodepki.GenerateKeyAndCSR("x", nil, nil)

	// flip the last character of the token signature
	forged := tok.Token[:len(tok.Token)-1]
	if strings.HasSuffix(tok.Token, "A") {
		forged += "B"
	} else {
		forged += "A"
	}
	if _, err := svc.Enroll(dto.NodeEnrollRequest{Token: forged, CSR: string(csrPEM)}); err == nil {
		t.Fatal("forged enrollment token was accepted (N2 violated)")
	}
}
