package service

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/backupsync"
	"github.com/1Panel-dev/1Panel/core/utils/nodepki"
	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func setupNodeTestDB(t *testing.T) {
	t.Helper()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	oldLog := global.LOG
	dbName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dbName)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open node test database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Node{},
		&model.Setting{},
		&model.BackupAccount{},
		&model.BackupSyncSequence{},
		&model.BackupSyncOutbox{},
		&model.BackupSyncTarget{},
		&model.BackupSyncTombstone{},
	); err != nil {
		t.Fatalf("migrate node test database: %v", err)
	}
	if err := backupsync.InitializeTx(db); err != nil {
		t.Fatalf("initialize backup sync state: %v", err)
	}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	global.LOG = logger
	global.DB = db
	global.CONF.Base.EncryptKey = "1234567890abcdef" // 16 bytes for AES
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
		global.LOG = oldLog
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
	if resp.ServerCert == "" || resp.CACert == "" || resp.ProxyID == "" || resp.CoreClientFingerprint == "" ||
		resp.BackupSyncAuthority == "" || resp.BackupSyncGeneration == "" || resp.BackupSyncTargetEpoch == "" {
		t.Fatalf("incomplete enroll response: %+v", resp)
	}
	// the node must now be online
	node, err := nodeRepo.Get(repo.WithByID(tok.NodeID))
	if err != nil || node.Status != "online" || node.ServerFingerprint == "" {
		t.Fatalf("node not marked online with pinned fp: %+v (err %v)", node, err)
	}
	var target model.BackupSyncTarget
	if err := global.DB.Where("target_key = ?", backupsync.NodeTargetKey(node.ID)).First(&target).Error; err != nil {
		t.Fatalf("load enrolled node sync target: %v", err)
	}
	sequence, err := backupsync.CurrentSequence()
	if err != nil {
		t.Fatalf("load enrolled node sync sequence: %v", err)
	}
	if resp.BackupSyncAuthority != sequence.Authority || resp.BackupSyncGeneration != sequence.Generation {
		t.Fatalf("enrollment synchronization namespace = %q/%q, want %q/%q", resp.BackupSyncAuthority, resp.BackupSyncGeneration, sequence.Authority, sequence.Generation)
	}
	if resp.BackupSyncTargetEpoch != target.TargetEpoch {
		t.Fatalf("enrollment target epoch = %q, want %q", resp.BackupSyncTargetEpoch, target.TargetEpoch)
	}
	if !target.Active || target.DesiredGeneration != sequence.Generation || target.DesiredRevision != sequence.Revision || target.AppliedRevision != 0 || target.Status != model.BackupSyncTargetStatusPending {
		t.Fatalf("unexpected enrolled node sync target: %#v", target)
	}

	// replay the SAME token -> must be rejected
	if _, err := svc.Enroll(dto.NodeEnrollRequest{Token: tok.Token, CSR: string(csrPEM)}); err == nil {
		t.Fatal("replayed enrollment token was accepted (N1 violated)")
	}
}

func TestFinalizeNodeEnrollmentDoesNotOverrideRevocation(t *testing.T) {
	setupNodeTestDB(t)
	svc := NewINodeService()
	tok, err := svc.Create(dto.NodeCreate{Name: "web-revoked-before-finalize", Addr: "127.0.0.1", Port: "9999"})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	var node model.Node
	if err := global.DB.First(&node, tok.NodeID).Error; err != nil {
		t.Fatalf("load pending node: %v", err)
	}
	if err := svc.Revoke(node.ID); err != nil {
		t.Fatalf("revoke pending node: %v", err)
	}

	err = global.DB.Transaction(func(tx *gorm.DB) error {
		_, finalizeErr := finalizeNodeEnrollmentTx(tx, node.ID, node.EnrollNonce, "synthetic-fingerprint")
		return finalizeErr
	})
	if err == nil {
		t.Fatal("stale enrollment finalized after revocation")
	}
	if err := global.DB.First(&node, tok.NodeID).Error; err != nil {
		t.Fatalf("reload revoked node: %v", err)
	}
	if node.Status != "revoked" || node.Enrolled || node.ServerFingerprint != "" {
		t.Fatalf("stale enrollment changed revoked node: %#v", node)
	}
}

func TestFinalizeNodeEnrollmentRotatesOnlyTheNodeTargetEpoch(t *testing.T) {
	setupNodeTestDB(t)
	svc := NewINodeService()
	token, err := svc.Create(dto.NodeCreate{Name: "reenroll-epoch", Addr: "127.0.0.1", Port: "9999"})
	if err != nil {
		t.Fatal(err)
	}
	var node model.Node
	if err := global.DB.First(&node, token.NodeID).Error; err != nil {
		t.Fatal(err)
	}
	var firstEpoch string
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		var finalizeErr error
		firstEpoch, finalizeErr = finalizeNodeEnrollmentTx(tx, node.ID, node.EnrollNonce, "first-fingerprint")
		return finalizeErr
	}); err != nil {
		t.Fatal(err)
	}
	firstSequence, err := backupsync.CurrentSequence()
	if err != nil {
		t.Fatal(err)
	}

	const secondNonce = "synthetic-second-enrollment-nonce"
	if err := global.DB.Model(&model.Node{}).Where("id = ?", node.ID).Updates(map[string]interface{}{
		"enrolled":     false,
		"status":       constant.NodeStatusPending,
		"enroll_nonce": secondNonce,
	}).Error; err != nil {
		t.Fatal(err)
	}
	var secondEpoch string
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		var finalizeErr error
		secondEpoch, finalizeErr = finalizeNodeEnrollmentTx(tx, node.ID, secondNonce, "second-fingerprint")
		return finalizeErr
	}); err != nil {
		t.Fatal(err)
	}
	secondSequence, err := backupsync.CurrentSequence()
	if err != nil {
		t.Fatal(err)
	}
	if firstEpoch == secondEpoch {
		t.Fatal("re-enrollment reused the previous node target epoch")
	}
	if firstSequence.Authority != secondSequence.Authority || firstSequence.Generation != secondSequence.Generation {
		t.Fatal("re-enrollment rotated the global synchronization namespace")
	}
	var target model.BackupSyncTarget
	if err := global.DB.Where("target_key = ?", backupsync.NodeTargetKey(node.ID)).First(&target).Error; err != nil {
		t.Fatal(err)
	}
	if target.TargetEpoch != secondEpoch || target.AppliedTargetEpoch != "" || target.AppliedRevision != 0 || target.LastSuccessAt != nil {
		t.Fatalf("re-enrolled target retained a previous acknowledgement: %#v", target)
	}
}

func TestNodeEnrollRollsBackTokenWhenSyncStateCannotPersist(t *testing.T) {
	setupNodeTestDB(t)
	svc := NewINodeService()
	tok, err := svc.Create(dto.NodeCreate{Name: "web-enroll-rollback", Addr: "127.0.0.1", Port: "9999"})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	_, csrPEM, err := nodepki.GenerateKeyAndCSR("whatever", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := global.DB.Migrator().DropTable(&model.BackupSyncTarget{}); err != nil {
		t.Fatalf("drop sync target table: %v", err)
	}
	if _, err := svc.Enroll(dto.NodeEnrollRequest{Token: tok.Token, CSR: string(csrPEM)}); err == nil {
		t.Fatal("enrollment succeeded without durable synchronization state")
	}
	var node model.Node
	if err := global.DB.First(&node, tok.NodeID).Error; err != nil {
		t.Fatalf("reload rolled-back node: %v", err)
	}
	if node.Enrolled || node.Status != "pending" || node.ServerFingerprint != "" {
		t.Fatalf("failed enrollment consumed token state: %#v", node)
	}
	if err := global.DB.AutoMigrate(&model.BackupSyncTarget{}); err != nil {
		t.Fatalf("restore sync target table: %v", err)
	}
	if _, err := svc.Enroll(dto.NodeEnrollRequest{Token: tok.Token, CSR: string(csrPEM)}); err != nil {
		t.Fatalf("retry enrollment with unconsumed token: %v", err)
	}
}

func TestNodeEnrollWaitsForDesiredStateExecution(t *testing.T) {
	setupNodeTestDB(t)
	svc := NewINodeService()
	tok, err := svc.Create(dto.NodeCreate{Name: "web-enroll-guard", Addr: "127.0.0.1", Port: "9999"})
	if err != nil {
		t.Fatalf("create guarded node: %v", err)
	}
	_, csrPEM, err := nodepki.GenerateKeyAndCSR("whatever", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	releaseExecution := backupsync.AcquireDesiredStateExecution()
	result := make(chan error, 1)
	go func() {
		_, enrollErr := svc.Enroll(dto.NodeEnrollRequest{Token: tok.Token, CSR: string(csrPEM)})
		result <- enrollErr
	}()
	select {
	case err := <-result:
		releaseExecution()
		t.Fatalf("enrollment crossed active execution guard: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	var pending model.Node
	if err := global.DB.First(&pending, tok.NodeID).Error; err != nil {
		releaseExecution()
		t.Fatalf("load guarded pending node: %v", err)
	}
	if pending.Enrolled || pending.Status != "pending" {
		releaseExecution()
		t.Fatalf("enrollment mutated desired state while execution guard was active: %#v", pending)
	}

	releaseExecution()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("enroll after execution guard release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("enrollment did not resume after execution guard release")
	}
}

// N10: Revoke marks the node revoked without deleting its row.
func TestNodeRevoke(t *testing.T) {
	setupNodeTestDB(t)
	svc := NewINodeService()
	tok, err := svc.Create(dto.NodeCreate{Name: "web-r", Addr: "127.0.0.1", Port: "9999"})
	if err != nil {
		t.Fatal(err)
	}
	_, csrPEM, err := nodepki.GenerateKeyAndCSR("whatever", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enroll(dto.NodeEnrollRequest{Token: tok.Token, CSR: string(csrPEM)}); err != nil {
		t.Fatalf("enroll node before revoke: %v", err)
	}
	if err := svc.Revoke(tok.NodeID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	node, err := nodeRepo.Get(repo.WithByID(tok.NodeID))
	if err != nil || node.Status != "revoked" {
		t.Fatalf("node not revoked: %+v (err %v)", node, err)
	}
	// the row is preserved (not deleted)
	if node.ID == 0 {
		t.Fatal("revoke must not delete the row")
	}
	var target model.BackupSyncTarget
	if err := global.DB.Where("target_key = ?", backupsync.NodeTargetKey(tok.NodeID)).First(&target).Error; err != nil {
		t.Fatalf("load revoked node sync target: %v", err)
	}
	if target.Active {
		t.Fatalf("revoked node sync target remained active: %#v", target)
	}
	if err := svc.Revoke(999999); err == nil {
		t.Fatal("revoking a missing node should error")
	}
}

func TestNodeRevokeWaitsForSnapshotDeliveryBarrier(t *testing.T) {
	setupNodeTestDB(t)
	svc := NewINodeService()
	tok, err := svc.Create(dto.NodeCreate{Name: "web-delivery-barrier", Addr: "127.0.0.1", Port: "9999"})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	releaseDeliveryBarrier := backupsync.AcquireDeliveryBarrier()
	result := make(chan error, 1)
	go func() {
		result <- svc.Revoke(tok.NodeID)
	}()
	select {
	case err := <-result:
		releaseDeliveryBarrier()
		t.Fatalf("revoke crossed active snapshot delivery barrier: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	releaseDeliveryBarrier()
	if err := <-result; err != nil {
		t.Fatalf("revoke after delivery barrier release: %v", err)
	}
	var node model.Node
	if err := global.DB.First(&node, tok.NodeID).Error; err != nil {
		t.Fatalf("reload revoked node: %v", err)
	}
	if node.Status != "revoked" {
		t.Fatalf("node status after barrier-protected revoke = %q", node.Status)
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
