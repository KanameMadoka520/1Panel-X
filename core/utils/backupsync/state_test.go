package backupsync

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestInitializeTxBootstrapsExistingPublicAccountsOnce(t *testing.T) {
	fixture := newStateTestFixture(t, false)
	if err := fixture.db.Create(&model.BackupAccount{Name: "existing", IsPublic: true}).Error; err != nil {
		t.Fatalf("create existing public account: %v", err)
	}

	if err := fixture.db.Transaction(InitializeTx); err != nil {
		t.Fatalf("initialize sync state: %v", err)
	}
	if err := fixture.db.Transaction(InitializeTx); err != nil {
		t.Fatalf("repeat sync state initialization: %v", err)
	}

	assertRevision(t, fixture.db, 1)
	var events []model.BackupSyncOutbox
	if err := fixture.db.Find(&events).Error; err != nil {
		t.Fatalf("load bootstrap outbox: %v", err)
	}
	if len(events) != 1 || events[0].Operation != model.BackupSyncOperationBootstrap || events[0].Revision != 1 {
		t.Fatalf("bootstrap events = %#v, want one revision-1 bootstrap", events)
	}
	assertTarget(t, fixture.db, model.BackupSyncTargetLocal, 1, 0, model.BackupSyncTargetStatusPending)
}

func TestInitializeTxNewInstallCreatesAuthoritativeEmptySnapshotOnce(t *testing.T) {
	fixture := newStateTestFixture(t, false)
	if err := fixture.db.Transaction(InitializeTx); err != nil {
		t.Fatalf("initialize empty sync state: %v", err)
	}
	if err := fixture.db.Transaction(InitializeTx); err != nil {
		t.Fatalf("repeat empty sync state initialization: %v", err)
	}

	assertRevision(t, fixture.db, 1)
	assertCount(t, fixture.db, &model.BackupSyncOutbox{}, 1)
	assertCount(t, fixture.db, &model.BackupSyncTombstone{}, 0)
	assertTarget(t, fixture.db, model.BackupSyncTargetLocal, 1, 0, model.BackupSyncTargetStatusPending)
	assertDueTargetCount(t, time.Now(), 1)
}

func TestCurrentSequenceRejectsCorruptPersistedIdentity(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	sequence, err := CurrentSequence()
	if err != nil {
		t.Fatalf("load initial sequence: %v", err)
	}

	tests := []struct {
		column string
		value  string
	}{
		{column: "authority", value: "not-a-valid-authority"},
		{column: "generation", value: "not-a-valid-generation"},
		{column: "snapshot_digest", value: "not-a-valid-digest"},
	}
	for _, test := range tests {
		t.Run(test.column, func(t *testing.T) {
			if err := fixture.db.Model(&model.BackupSyncSequence{}).
				Where("id = ?", model.BackupSyncSequenceID).
				Update(test.column, test.value).Error; err != nil {
				t.Fatalf("corrupt %s: %v", test.column, err)
			}
			if _, err := CurrentSequence(); err == nil {
				t.Fatalf("corrupt %s was accepted", test.column)
			}
			if err := fixture.db.Model(&model.BackupSyncSequence{}).
				Where("id = ?", model.BackupSyncSequenceID).
				Updates(map[string]interface{}{
					"authority":       sequence.Authority,
					"generation":      sequence.Generation,
					"snapshot_digest": sequence.SnapshotDigest,
				}).Error; err != nil {
				t.Fatalf("restore sequence after %s: %v", test.column, err)
			}
		})
	}
}

func TestStartupReconciliationForcesFreshRevisionAfterPersistedAck(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	if err := markStateTestTargetSuccess(model.BackupSyncTargetLocal, 1, time.Now()); err != nil {
		t.Fatalf("acknowledge initial snapshot: %v", err)
	}
	assertDueTargetCount(t, time.Now(), 0)
	assertOutboxStatus(t, fixture.db, 1, model.BackupSyncOutboxStatusCompleted)

	if err := EnqueueStartupReconciliation(); err != nil {
		t.Fatalf("enqueue startup reconciliation: %v", err)
	}
	assertRevision(t, fixture.db, 2)
	assertCount(t, fixture.db, &model.BackupSyncOutbox{}, 2)
	assertTarget(t, fixture.db, model.BackupSyncTargetLocal, 2, 1, model.BackupSyncTargetStatusPending)
	assertDueTargetCount(t, time.Now(), 1)
}

func TestInitializeTxBootstrapsEnrolledNodesWithEmptyDesiredState(t *testing.T) {
	fixture := newStateTestFixture(t, false)
	node := model.Node{Name: "existing-node", Enrolled: true, Status: constant.NodeStatusOffline}
	if err := fixture.db.Create(&node).Error; err != nil {
		t.Fatalf("create existing enrolled node: %v", err)
	}

	if err := fixture.db.Transaction(InitializeTx); err != nil {
		t.Fatalf("initialize empty desired state with enrolled node: %v", err)
	}

	assertRevision(t, fixture.db, 1)
	assertCount(t, fixture.db, &model.BackupSyncOutbox{}, 1)
	assertTarget(t, fixture.db, model.BackupSyncTargetLocal, 1, 0, model.BackupSyncTargetStatusPending)
	assertTarget(t, fixture.db, NodeTargetKey(node.ID), 1, 0, model.BackupSyncTargetStatusPending)
}

func TestEnsureNodeTargetCreatesOneFreshBootstrapForNewTarget(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	node := model.Node{Name: "new-node", Enrolled: true, Status: constant.NodeStatusOnline}
	if err := fixture.db.Create(&node).Error; err != nil {
		t.Fatalf("create newly enrolled node: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		if err := fixture.db.Transaction(func(tx *gorm.DB) error {
			return EnsureNodeTargetTx(tx, node.ID)
		}); err != nil {
			t.Fatalf("ensure newly enrolled node target attempt %d: %v", attempt+1, err)
		}
	}

	assertRevision(t, fixture.db, 2)
	assertCount(t, fixture.db, &model.BackupSyncOutbox{}, 2)
	assertTarget(t, fixture.db, model.BackupSyncTargetLocal, 2, 0, model.BackupSyncTargetStatusPending)
	assertTarget(t, fixture.db, NodeTargetKey(node.ID), 2, 0, model.BackupSyncTargetStatusPending)
}

func TestRetryAccountUsesBootstrapRevisionOnlyForVisibleAccount(t *testing.T) {
	fixture := newStateTestFixture(t, false)
	if err := fixture.db.Create(&model.BackupAccount{Name: "existing", IsPublic: true}).Error; err != nil {
		t.Fatalf("create existing public account: %v", err)
	}
	if err := fixture.db.Transaction(InitializeTx); err != nil {
		t.Fatalf("initialize sync state: %v", err)
	}
	now := time.Date(2026, 8, 4, 11, 0, 0, 0, time.UTC)
	if err := MarkTargetFailure(model.BackupSyncTargetLocal, errors.New("offline"), now); err != nil {
		t.Fatalf("mark bootstrap delivery failure: %v", err)
	}

	if err := RetryAccount("missing"); err != nil {
		t.Fatalf("retry unknown account: %v", err)
	}
	target := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if target.Status != model.BackupSyncTargetStatusFailed || target.NextRetryAt == nil {
		t.Fatalf("unknown account unexpectedly reset bootstrap backoff: %#v", target)
	}

	if err := RetryAccount("existing"); err != nil {
		t.Fatalf("retry bootstrap account: %v", err)
	}
	target = loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if target.Status != model.BackupSyncTargetStatusPending || target.NextRetryAt != nil || target.LastError != "" {
		t.Fatalf("visible bootstrap account did not reset backoff: %#v", target)
	}
}

func TestRetryAccountClearsBackoffForExpiredCurrentRevisionLease(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	if err := fixture.db.Create(&model.BackupAccount{Name: "existing", IsPublic: true}).Error; err != nil {
		t.Fatalf("create existing public account: %v", err)
	}
	sequence, err := CurrentSequence()
	if err != nil {
		t.Fatalf("load current sequence: %v", err)
	}
	digest := strings.Repeat("c", 64)
	if err := fixture.db.Model(&model.BackupSyncSequence{}).
		Where("id = ?", model.BackupSyncSequenceID).
		Update("snapshot_digest", digest).Error; err != nil {
		t.Fatalf("bind current snapshot digest: %v", err)
	}
	target := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	now := time.Now()
	if err := MarkTargetSuccess(model.BackupSyncTargetLocal, SnapshotIdentity{
		Authority:   sequence.Authority,
		Generation:  sequence.Generation,
		TargetEpoch: target.TargetEpoch,
		Revision:    sequence.Revision,
		Digest:      digest,
	}, now.Add(-PublicBackupSyncLeaseDuration-time.Minute)); err != nil {
		t.Fatalf("acknowledge current revision with an old lease: %v", err)
	}
	if err := MarkTargetFailure(model.BackupSyncTargetLocal, errors.New("renewal unavailable"), now); err != nil {
		t.Fatalf("persist renewal backoff: %v", err)
	}
	target = loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if target.AppliedRevision != sequence.Revision || target.NextRetryAt == nil || !target.NextRetryAt.After(now) {
		t.Fatalf("test target does not hold a current revision with future backoff: %#v", target)
	}
	assertDueTargetCount(t, now, 0)

	if err := RetryAccount("existing"); err != nil {
		t.Fatalf("retry expired current revision lease: %v", err)
	}
	target = loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if target.Status != model.BackupSyncTargetStatusPending || target.NextRetryAt != nil || target.LastError != "" {
		t.Fatalf("manual account retry did not clear renewal backoff: %#v", target)
	}
	assertDueTargetCount(t, time.Now(), 1)
}

func TestEnqueueTxProducesMonotonicRevisions(t *testing.T) {
	fixture := newStateTestFixture(t, false)
	operations := []string{
		model.BackupSyncOperationCreate,
		model.BackupSyncOperationUpdate,
		model.BackupSyncOperationRefresh,
	}
	for index, operation := range operations {
		var got uint64
		if err := fixture.db.Transaction(func(tx *gorm.DB) error {
			var err error
			got, err = EnqueueTx(tx, "account", operation)
			return err
		}); err != nil {
			t.Fatalf("enqueue %s: %v", operation, err)
		}
		want := uint64(index + 1)
		if got != want {
			t.Fatalf("revision for %s = %d, want %d", operation, got, want)
		}
	}

	assertRevision(t, fixture.db, 3)
	var events []model.BackupSyncOutbox
	if err := fixture.db.Order("revision ASC").Find(&events).Error; err != nil {
		t.Fatalf("load outbox events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("outbox event count = %d, want 3", len(events))
	}
	for index, event := range events {
		if event.Revision != uint64(index+1) || event.Operation != operations[index] {
			t.Fatalf("outbox event %d = %#v", index, event)
		}
	}
}

func TestEnqueueTxProducesUniqueMonotonicRevisionsConcurrently(t *testing.T) {
	fixture := newStateTestFixture(t, false)
	const workerCount = 8
	type result struct {
		revision uint64
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, workerCount)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for index := 0; index < workerCount; index++ {
		go func(index int) {
			defer workers.Done()
			<-start
			var revision uint64
			err := fixture.db.Transaction(func(tx *gorm.DB) error {
				var err error
				revision, err = EnqueueTx(tx, fmt.Sprintf("account-%d", index), model.BackupSyncOperationUpdate)
				return err
			})
			results <- result{revision: revision, err: err}
		}(index)
	}
	close(start)
	workers.Wait()
	close(results)

	revisions := make([]uint64, 0, workerCount)
	for item := range results {
		if item.err != nil {
			t.Fatalf("concurrent enqueue failed: %v", item.err)
		}
		revisions = append(revisions, item.revision)
	}
	sort.Slice(revisions, func(i, j int) bool { return revisions[i] < revisions[j] })
	for index, revision := range revisions {
		want := uint64(index + 1)
		if revision != want {
			t.Fatalf("sorted concurrent revisions = %v, want contiguous 1..%d", revisions, workerCount)
		}
	}
	assertRevision(t, fixture.db, workerCount)
	assertCount(t, fixture.db, &model.BackupSyncOutbox{}, workerCount)
}

func TestEnqueueTxRollbackLeavesNoRevisionOrOutbox(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	baselineRevision, err := CurrentRevisionTx(fixture.db)
	if err != nil {
		t.Fatalf("load baseline revision: %v", err)
	}
	rollbackErr := errors.New("roll back business mutation")
	err = fixture.db.Transaction(func(tx *gorm.DB) error {
		if _, err := EnqueueTx(tx, "removed", model.BackupSyncOperationDelete); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("transaction error = %v, want rollback sentinel", err)
	}

	assertRevision(t, fixture.db, baselineRevision)
	assertCount(t, fixture.db, &model.BackupSyncOutbox{}, 1)
	assertCount(t, fixture.db, &model.BackupSyncTombstone{}, 0)
	assertTarget(t, fixture.db, model.BackupSyncTargetLocal, baselineRevision, 0, model.BackupSyncTargetStatusPending)
}

func TestTargetsTrackDesiredAndAppliedRevisions(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	nodes := []model.Node{
		{Name: "online", Enrolled: true, Status: constant.NodeStatusOnline},
		{Name: "offline", Enrolled: true, Status: constant.NodeStatusOffline},
		{Name: "revoked", Enrolled: true, Status: constant.NodeStatusRevoked},
		{Name: "unenrolled", Enrolled: false, Status: constant.NodeStatusOnline},
	}
	if err := fixture.db.Create(&nodes).Error; err != nil {
		t.Fatalf("create nodes: %v", err)
	}

	revision := enqueueStateTestEvent(t, fixture.db, "account", model.BackupSyncOperationCreate)
	if revision != 2 {
		t.Fatalf("revision = %d, want 2", revision)
	}
	assertTarget(t, fixture.db, model.BackupSyncTargetLocal, revision, 0, model.BackupSyncTargetStatusPending)
	assertTarget(t, fixture.db, NodeTargetKey(nodes[0].ID), revision, 0, model.BackupSyncTargetStatusPending)
	assertTarget(t, fixture.db, NodeTargetKey(nodes[1].ID), revision, 0, model.BackupSyncTargetStatusPending)
	assertMissingTarget(t, fixture.db, NodeTargetKey(nodes[2].ID))
	assertMissingTarget(t, fixture.db, NodeTargetKey(nodes[3].ID))

	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	if err := markStateTestTargetSuccess(model.BackupSyncTargetLocal, revision, now); err != nil {
		t.Fatalf("mark local target success: %v", err)
	}
	assertTarget(t, fixture.db, model.BackupSyncTargetLocal, revision, revision, model.BackupSyncTargetStatusSynced)
	assertOutboxStatus(t, fixture.db, revision, model.BackupSyncOutboxStatusPending)

	second := enqueueStateTestEvent(t, fixture.db, "account", model.BackupSyncOperationUpdate)
	if second != 3 {
		t.Fatalf("second revision = %d, want 3", second)
	}
	assertTarget(t, fixture.db, model.BackupSyncTargetLocal, second, revision, model.BackupSyncTargetStatusPending)
}

func TestEnqueueTxPreservesFailedTargetBackoffUntilManualRetry(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	now := time.Date(2026, 8, 4, 12, 30, 0, 0, time.UTC)
	if err := MarkTargetFailure(model.BackupSyncTargetLocal, errors.New("node remains offline"), now); err != nil {
		t.Fatalf("mark target failure: %v", err)
	}
	before := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)

	revision := enqueueStateTestEvent(t, fixture.db, "account", model.BackupSyncOperationUpdate)
	after := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if after.DesiredRevision != revision || after.AppliedRevision != before.AppliedRevision {
		t.Fatalf("failed target revisions after enqueue = %#v, before = %#v", after, before)
	}
	if after.Status != model.BackupSyncTargetStatusFailed || after.Attempts != before.Attempts || after.LastError != before.LastError {
		t.Fatalf("new revision reset failed target state: before=%#v after=%#v", before, after)
	}
	if after.NextRetryAt == nil || before.NextRetryAt == nil || !after.NextRetryAt.Equal(*before.NextRetryAt) {
		t.Fatalf("new revision reset failed target retry time: before=%v after=%v", before.NextRetryAt, after.NextRetryAt)
	}
	assertDueTargetCount(t, now.Add(4*time.Second), 0)
	assertDueTargetCount(t, now.Add(5*time.Second), 1)

	if err := RetryTarget(model.BackupSyncTargetLocal); err != nil {
		t.Fatalf("manually retry target: %v", err)
	}
	after = loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if after.Status != model.BackupSyncTargetStatusPending || after.NextRetryAt != nil || after.LastError != "" {
		t.Fatalf("manual retry did not clear failed target state: %#v", after)
	}
}

func TestDeactivateNodeTargetTxParticipatesInCallerTransaction(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	node := model.Node{Name: "remote", Enrolled: true, Status: constant.NodeStatusOnline}
	if err := fixture.db.Create(&node).Error; err != nil {
		t.Fatalf("create remote node: %v", err)
	}
	revision := enqueueStateTestEvent(t, fixture.db, "account", model.BackupSyncOperationUpdate)
	if err := markStateTestTargetSuccess(model.BackupSyncTargetLocal, revision, time.Now()); err != nil {
		t.Fatalf("acknowledge local target: %v", err)
	}

	rollbackErr := errors.New("roll back node lifecycle")
	err := fixture.db.Transaction(func(tx *gorm.DB) error {
		if err := DeactivateNodeTargetTx(tx, node.ID); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("deactivate rollback error = %v, want sentinel", err)
	}
	assertTarget(t, fixture.db, NodeTargetKey(node.ID), revision, 0, model.BackupSyncTargetStatusPending)
	assertOutboxStatus(t, fixture.db, revision, model.BackupSyncOutboxStatusPending)

	if err := fixture.db.Transaction(func(tx *gorm.DB) error {
		return DeactivateNodeTargetTx(tx, node.ID)
	}); err != nil {
		t.Fatalf("deactivate node target: %v", err)
	}
	target := loadTarget(t, fixture.db, NodeTargetKey(node.ID))
	if target.Active || target.Status != model.BackupSyncTargetStatusSynced {
		t.Fatalf("deactivated target = %#v", target)
	}
	assertOutboxStatus(t, fixture.db, revision, model.BackupSyncOutboxStatusCompleted)
}

func TestFailureBackoffAndPendingTargetSurviveRestart(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	revision := enqueueStateTestEvent(t, fixture.db, "account", model.BackupSyncOperationUpdate)
	now := time.Date(2026, 8, 4, 13, 0, 0, 0, time.UTC)
	longError := "transport failed\n\t" + strings.Repeat("x", maxStatusErrorRunes+50)
	if err := MarkTargetFailure(model.BackupSyncTargetLocal, errors.New(longError), now); err != nil {
		t.Fatalf("mark target failure: %v", err)
	}

	fixture.reopen(t)
	target := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if target.DesiredRevision != revision || target.AppliedRevision != 0 || target.Attempts != 1 {
		t.Fatalf("reloaded target revision/attempt state = %#v", target)
	}
	if target.Status != model.BackupSyncTargetStatusFailed {
		t.Fatalf("reloaded target status = %q, want failed", target.Status)
	}
	if target.NextRetryAt == nil || !target.NextRetryAt.Equal(now.Add(5*time.Second)) {
		t.Fatalf("next retry = %v, want %v", target.NextRetryAt, now.Add(5*time.Second))
	}
	if strings.ContainsAny(target.LastError, "\n\r\t") || len([]rune(target.LastError)) > maxStatusErrorRunes {
		t.Fatalf("stored error was not sanitized: %q", target.LastError)
	}

	assertDueTargetCount(t, now.Add(4*time.Second), 0)
	assertDueTargetCount(t, now.Add(5*time.Second), 1)
	if err := RetryTarget(model.BackupSyncTargetLocal); err != nil {
		t.Fatalf("retry target: %v", err)
	}
	target = loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if target.NextRetryAt != nil || target.LastError != "" || target.Status != model.BackupSyncTargetStatusPending {
		t.Fatalf("manual retry did not clear durable backoff: %#v", target)
	}
	assertDueTargetCount(t, now, 1)
}

func TestPendingOutboxSurvivesRestartWithoutDuplicateBootstrap(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	revision := enqueueStateTestEvent(t, fixture.db, "account", model.BackupSyncOperationCreate)
	fixture.reopen(t)

	if err := fixture.db.Transaction(InitializeTx); err != nil {
		t.Fatalf("reinitialize after restart: %v", err)
	}
	assertRevision(t, fixture.db, revision)
	assertCount(t, fixture.db, &model.BackupSyncOutbox{}, 2)
	assertOutboxStatus(t, fixture.db, revision, model.BackupSyncOutboxStatusPending)
	assertDueTargetCount(t, time.Now().Add(24*time.Hour), 1)
}

func TestTombstoneRemainsUntilAllActiveTargetsAcknowledge(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	node := model.Node{Name: "remote", Enrolled: true, Status: constant.NodeStatusOnline}
	if err := fixture.db.Create(&node).Error; err != nil {
		t.Fatalf("create remote node: %v", err)
	}
	revision := enqueueStateTestEvent(t, fixture.db, "removed", model.BackupSyncOperationDelete)
	assertTombstone(t, fixture.db, "removed", revision, true)

	now := time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC)
	if err := markStateTestTargetSuccess(model.BackupSyncTargetLocal, revision, now); err != nil {
		t.Fatalf("acknowledge local deletion: %v", err)
	}
	assertTombstone(t, fixture.db, "removed", revision, true)
	assertOutboxStatus(t, fixture.db, revision, model.BackupSyncOutboxStatusPending)

	if err := markStateTestTargetSuccess(NodeTargetKey(node.ID), revision, now.Add(time.Second)); err != nil {
		t.Fatalf("acknowledge remote deletion: %v", err)
	}
	assertTombstone(t, fixture.db, "removed", revision, false)
	assertOutboxStatus(t, fixture.db, revision, model.BackupSyncOutboxStatusCompleted)

	deleteAgain := enqueueStateTestEvent(t, fixture.db, "recreated", model.BackupSyncOperationDelete)
	assertTombstone(t, fixture.db, "recreated", deleteAgain, true)
	recreated := enqueueStateTestEvent(t, fixture.db, "recreated", model.BackupSyncOperationCreate)
	assertTombstone(t, fixture.db, "recreated", recreated, false)
}

func TestCompletionRequiresAcknowledgementFromCurrentGeneration(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	sequence, err := CurrentSequence()
	if err != nil {
		t.Fatalf("load original sequence: %v", err)
	}
	target := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	oldGeneration := sequence.Generation
	newGeneration := strings.Repeat("f", 64)
	if newGeneration == oldGeneration {
		newGeneration = strings.Repeat("e", 64)
	}
	if err := fixture.db.Model(&model.BackupSyncSequence{}).
		Where("id = ?", model.BackupSyncSequenceID).
		Updates(map[string]interface{}{
			"generation":      newGeneration,
			"revision":        1,
			"snapshot_digest": "",
		}).Error; err != nil {
		t.Fatalf("rotate synthetic generation: %v", err)
	}
	if err := fixture.db.Model(&model.BackupSyncTarget{}).
		Where("target_key = ?", model.BackupSyncTargetLocal).
		Updates(map[string]interface{}{
			"applied_target_epoch": target.TargetEpoch,
			"desired_generation":   newGeneration,
			"desired_revision":     1,
			"applied_authority":    sequence.Authority,
			"applied_generation":   oldGeneration,
			"applied_revision":     999,
			"applied_digest":       strings.Repeat("a", 64),
			"status":               model.BackupSyncTargetStatusSynced,
		}).Error; err != nil {
		t.Fatalf("seed old-generation high revision acknowledgement: %v", err)
	}
	if err := fixture.db.Create(&model.BackupSyncOutbox{
		Generation:  newGeneration,
		Revision:    1,
		AccountName: "removed-in-new-generation",
		Operation:   model.BackupSyncOperationDelete,
		Status:      model.BackupSyncOutboxStatusPending,
	}).Error; err != nil {
		t.Fatalf("seed new-generation outbox: %v", err)
	}
	if err := fixture.db.Create(&model.BackupSyncTombstone{
		AccountName: "removed-in-new-generation",
		Generation:  newGeneration,
		Revision:    1,
		Active:      true,
	}).Error; err != nil {
		t.Fatalf("seed new-generation tombstone: %v", err)
	}

	status, err := GetStatus("removed-in-new-generation")
	if err != nil {
		t.Fatalf("get new-generation delete status: %v", err)
	}
	if status.Status != model.BackupSyncStatusPending || status.Pending != 1 || status.Succeeded != 0 {
		t.Fatalf("old-generation high revision completed current generation: %#v", status)
	}
	var event model.BackupSyncOutbox
	if err := fixture.db.Where("generation = ? AND revision = ?", newGeneration, 1).First(&event).Error; err != nil {
		t.Fatalf("load new-generation outbox: %v", err)
	}
	if event.Status != model.BackupSyncOutboxStatusPending {
		t.Fatalf("new-generation outbox status = %q, want pending", event.Status)
	}
	var tombstone model.BackupSyncTombstone
	if err := fixture.db.Where("account_name = ?", "removed-in-new-generation").First(&tombstone).Error; err != nil {
		t.Fatalf("load new-generation tombstone: %v", err)
	}
	if !tombstone.Active {
		t.Fatal("old-generation high revision garbage-collected current-generation tombstone")
	}
}

func TestCurrentGenerationScopesReadinessAcknowledgementsAndStatus(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	sequence, err := CurrentSequence()
	if err != nil {
		t.Fatalf("load original sequence: %v", err)
	}
	target := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	oldGeneration := sequence.Generation
	oldDigest := strings.Repeat("a", 64)
	if err := fixture.db.Model(&model.BackupSyncSequence{}).
		Where("id = ?", model.BackupSyncSequenceID).
		Update("snapshot_digest", oldDigest).Error; err != nil {
		t.Fatalf("bind old-generation digest: %v", err)
	}
	if err := fixture.db.Model(&model.BackupSyncTarget{}).
		Where("target_key = ?", model.BackupSyncTargetLocal).
		Updates(map[string]interface{}{
			"applied_target_epoch": target.TargetEpoch,
			"desired_generation":   oldGeneration,
			"desired_revision":     sequence.Revision,
			"applied_authority":    sequence.Authority,
			"applied_generation":   oldGeneration,
			"applied_revision":     sequence.Revision,
			"applied_digest":       oldDigest,
			"status":               model.BackupSyncTargetStatusSynced,
			"last_success_at":      time.Now(),
		}).Error; err != nil {
		t.Fatalf("seed old-generation proof: %v", err)
	}
	ready, err := TargetReady(model.BackupSyncTargetLocal)
	if err != nil || !ready {
		t.Fatalf("matching current-generation proof ready=%t err=%v", ready, err)
	}

	newGeneration := strings.Repeat("f", 64)
	if newGeneration == oldGeneration {
		newGeneration = strings.Repeat("e", 64)
	}
	newDigest := strings.Repeat("b", 64)
	if err := fixture.db.Model(&model.BackupSyncSequence{}).
		Where("id = ?", model.BackupSyncSequenceID).
		Updates(map[string]interface{}{
			"generation":      newGeneration,
			"revision":        1,
			"snapshot_digest": newDigest,
		}).Error; err != nil {
		t.Fatalf("rotate synthetic generation: %v", err)
	}
	ready, err = TargetReady(model.BackupSyncTargetLocal)
	if err != nil {
		t.Fatalf("check readiness after generation change: %v", err)
	}
	if ready {
		t.Fatal("old-generation proof remained ready after generation change")
	}
	if err := MarkTargetSuccess(model.BackupSyncTargetLocal, SnapshotIdentity{
		Authority:   sequence.Authority,
		Generation:  oldGeneration,
		TargetEpoch: target.TargetEpoch,
		Revision:    999,
		Digest:      oldDigest,
	}, time.Now()); err == nil {
		t.Fatal("old-generation acknowledgement was accepted after generation change")
	}

	if err := fixture.db.Create(&model.BackupAccount{Name: "visible-current-account", IsPublic: true}).Error; err != nil {
		t.Fatalf("create current visible account: %v", err)
	}
	if err := fixture.db.Create(&model.BackupSyncOutbox{
		Generation:  newGeneration,
		Revision:    1,
		AccountName: "",
		Operation:   model.BackupSyncOperationBootstrap,
		Status:      model.BackupSyncOutboxStatusPending,
	}).Error; err != nil {
		t.Fatalf("create current-generation bootstrap: %v", err)
	}
	if err := fixture.db.Create(&model.BackupSyncOutbox{
		Generation:  oldGeneration,
		Revision:    77,
		AccountName: "visible-current-account",
		Operation:   model.BackupSyncOperationUpdate,
		Status:      model.BackupSyncOutboxStatusPending,
	}).Error; err != nil {
		t.Fatalf("create old-generation account event: %v", err)
	}
	if err := fixture.db.Create(&model.BackupSyncTombstone{
		AccountName: "old-generation-delete",
		Generation:  oldGeneration,
		Revision:    78,
		Active:      true,
	}).Error; err != nil {
		t.Fatalf("create old-generation tombstone: %v", err)
	}

	status, err := GetStatus("visible-current-account")
	if err != nil {
		t.Fatalf("get current-generation fallback status: %v", err)
	}
	if status.Revision != 1 {
		t.Fatalf("current account selected old-generation revision %d, want current bootstrap revision 1", status.Revision)
	}
	statuses, err := ListStatuses()
	if err != nil {
		t.Fatalf("list current-generation statuses: %v", err)
	}
	assertStatusNames(t, statuses, []string{"visible-current-account"})

	now := time.Now()
	if err := MarkTargetFailure(model.BackupSyncTargetLocal, errors.New("synthetic failure"), now); err != nil {
		t.Fatalf("mark target failed before old tombstone retry: %v", err)
	}
	before := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if err := RetryAccount("old-generation-delete"); err != nil {
		t.Fatalf("retry old-generation tombstone: %v", err)
	}
	after := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if after.Status != before.Status || after.Attempts != before.Attempts || after.LastError != before.LastError {
		t.Fatalf("old-generation tombstone changed current retry state: before=%#v after=%#v", before, after)
	}
}

func TestExpiredTargetLeaseIsPendingAndDueForRenewal(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	sequence, err := CurrentSequence()
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("c", 64)
	if err := fixture.db.Model(&model.BackupSyncSequence{}).
		Where("id = ?", model.BackupSyncSequenceID).
		Update("snapshot_digest", digest).Error; err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-PublicBackupSyncLeaseDuration - time.Second)
	target := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if err := MarkTargetSuccess(model.BackupSyncTargetLocal, SnapshotIdentity{
		Authority:   sequence.Authority,
		Generation:  sequence.Generation,
		TargetEpoch: target.TargetEpoch,
		Revision:    sequence.Revision,
		Digest:      digest,
	}, expiredAt); err != nil {
		t.Fatal(err)
	}
	ready, err := TargetReady(model.BackupSyncTargetLocal)
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("expired synchronization lease remained ready")
	}
	due, err := ListDueTargets(time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].TargetKey != model.BackupSyncTargetLocal {
		t.Fatalf("expired lease due targets = %#v", due)
	}
	status, err := buildStatus("shared", sequence.Generation, sequence.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != model.BackupSyncStatusPending || status.Pending != 1 || status.Succeeded != 0 {
		t.Fatalf("expired lease status = %#v", status)
	}
}

func TestManualRetryClearsExpiredCurrentRevisionBackoff(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	if err := fixture.db.Create(&model.BackupAccount{Name: "retry-expired", IsPublic: true}).Error; err != nil {
		t.Fatal(err)
	}
	sequence, err := CurrentSequence()
	if err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-PublicBackupSyncLeaseDuration - time.Minute)
	if err := markStateTestTargetSuccess(model.BackupSyncTargetLocal, sequence.Revision, expiredAt); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := MarkTargetFailure(model.BackupSyncTargetLocal, errors.New("synthetic renewal failure"), now); err != nil {
		t.Fatal(err)
	}
	assertDueTargetCount(t, now, 0)

	if err := RetryTarget(model.BackupSyncTargetLocal); err != nil {
		t.Fatal(err)
	}
	assertDueTargetCount(t, now, 1)

	if err := MarkTargetFailure(model.BackupSyncTargetLocal, errors.New("synthetic renewal failure"), now); err != nil {
		t.Fatal(err)
	}
	assertDueTargetCount(t, now, 0)
	if err := RetryAccount("retry-expired"); err != nil {
		t.Fatal(err)
	}
	target := loadTarget(t, fixture.db, model.BackupSyncTargetLocal)
	if target.Status != model.BackupSyncTargetStatusPending || target.NextRetryAt != nil || target.LastError != "" {
		t.Fatalf("account retry did not clear expired-lease backoff: %#v", target)
	}
	assertDueTargetCount(t, now, 1)
}

func TestListDueTargetsRotatesPastBatchLimit(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	sequence, err := CurrentSequence()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 105; i++ {
		target := model.BackupSyncTarget{
			TargetKey:         fmt.Sprintf("fair:%03d", i),
			Active:            true,
			TargetEpoch:       fmt.Sprintf("%064x", i+1000),
			DesiredGeneration: sequence.Generation,
			DesiredRevision:   sequence.Revision,
			Status:            model.BackupSyncTargetStatusPending,
		}
		if err := fixture.db.Create(&target).Error; err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now()
	first, err := ListDueTargets(now, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 100 {
		t.Fatalf("first due batch size = %d, want 100", len(first))
	}
	firstKeys := make(map[string]struct{}, len(first))
	for _, target := range first {
		firstKeys[target.TargetKey] = struct{}{}
		if err := MarkTargetAttempt(target.TargetKey, now); err != nil {
			t.Fatal(err)
		}
	}
	second, err := ListDueTargets(now.Add(time.Second), 100)
	if err != nil {
		t.Fatal(err)
	}
	foundPreviouslySkipped := false
	for _, target := range second {
		if _, existed := firstKeys[target.TargetKey]; !existed {
			foundPreviouslySkipped = true
			break
		}
	}
	if !foundPreviouslySkipped {
		t.Fatal("due-target ordering starved every target beyond the first batch")
	}
}

func TestRotateNodeTargetEpochInvalidatesPreviousAcknowledgement(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	node := model.Node{Name: "reenrolled", Enrolled: true, Status: constant.NodeStatusOnline}
	if err := fixture.db.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureNodeTarget(node.ID); err != nil {
		t.Fatal(err)
	}
	before := loadTarget(t, fixture.db, NodeTargetKey(node.ID))
	if err := markStateTestTargetSuccess(before.TargetKey, before.DesiredRevision, time.Now()); err != nil {
		t.Fatal(err)
	}
	var rotated string
	if err := fixture.db.Transaction(func(tx *gorm.DB) error {
		var rotateErr error
		rotated, rotateErr = RotateNodeTargetEpochTx(tx, node.ID)
		return rotateErr
	}); err != nil {
		t.Fatal(err)
	}
	after := loadTarget(t, fixture.db, before.TargetKey)
	if rotated == before.TargetEpoch || after.TargetEpoch != rotated || after.AppliedTargetEpoch != "" || after.AppliedRevision != 0 || after.LastSuccessAt != nil || after.Status != model.BackupSyncTargetStatusPending {
		t.Fatalf("rotated target retained an old acknowledgement: before=%#v after=%#v", before, after)
	}
}

func TestStatusTransitionsAndPerEventTargetState(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	node := model.Node{Name: "remote", Enrolled: true, Status: constant.NodeStatusOnline}
	if err := fixture.db.Create(&node).Error; err != nil {
		t.Fatalf("create remote node: %v", err)
	}
	if err := fixture.db.Create(&model.BackupAccount{Name: "alpha", IsPublic: true}).Error; err != nil {
		t.Fatalf("create public account: %v", err)
	}
	alphaRevision := enqueueStateTestEvent(t, fixture.db, "alpha", model.BackupSyncOperationCreate)
	assertSyncStatus(t, "alpha", model.BackupSyncStatusPending, 0, 2)

	now := time.Now()
	if err := markStateTestTargetSuccess(model.BackupSyncTargetLocal, alphaRevision, now); err != nil {
		t.Fatalf("acknowledge local target: %v", err)
	}
	assertSyncStatus(t, "alpha", model.BackupSyncStatusPartiallySynced, 1, 1)
	if err := markStateTestTargetSuccess(NodeTargetKey(node.ID), alphaRevision, now); err != nil {
		t.Fatalf("acknowledge remote target: %v", err)
	}
	assertSyncStatus(t, "alpha", model.BackupSyncStatusSynced, 2, 0)

	if err := fixture.db.Create(&model.BackupAccount{Name: "zeta", IsPublic: true}).Error; err != nil {
		t.Fatalf("create second public account: %v", err)
	}
	zetaRevision := enqueueStateTestEvent(t, fixture.db, "zeta", model.BackupSyncOperationCreate)
	if err := MarkTargetFailure(model.BackupSyncTargetLocal, errors.New("later revision failed"), now.Add(time.Minute)); err != nil {
		t.Fatalf("mark later revision failure: %v", err)
	}
	if zetaRevision <= alphaRevision {
		t.Fatalf("later revision = %d, want > %d", zetaRevision, alphaRevision)
	}

	alphaStatus, err := GetStatus("alpha")
	if err != nil {
		t.Fatalf("get alpha status: %v", err)
	}
	if alphaStatus.Status != model.BackupSyncStatusSynced {
		t.Fatalf("alpha status = %q, want synced", alphaStatus.Status)
	}
	for _, target := range alphaStatus.Targets {
		if target.Status != model.BackupSyncTargetStatusSynced {
			t.Fatalf("alpha target %s status = %q for applied event revision, want synced", target.TargetKey, target.Status)
		}
	}
}

func TestStatusReconciliationDeactivatesOnlyMissingOrRevokedNodeTargets(t *testing.T) {
	tests := []struct {
		name             string
		mutateNode       func(*testing.T, *gorm.DB, model.Node)
		wantSynced       bool
		wantTargetActive bool
	}{
		{
			name: "missing node",
			mutateNode: func(t *testing.T, db *gorm.DB, node model.Node) {
				t.Helper()
				if err := db.Unscoped().Where("id = ?", node.ID).Delete(&model.Node{}).Error; err != nil {
					t.Fatalf("delete node without target cleanup: %v", err)
				}
			},
			wantSynced:       true,
			wantTargetActive: false,
		},
		{
			name: "revoked node",
			mutateNode: func(t *testing.T, db *gorm.DB, node model.Node) {
				t.Helper()
				if err := db.Model(&model.Node{}).Where("id = ?", node.ID).Update("status", constant.NodeStatusRevoked).Error; err != nil {
					t.Fatalf("revoke node without target cleanup: %v", err)
				}
			},
			wantSynced:       true,
			wantTargetActive: false,
		},
		{
			name:             "online node",
			mutateNode:       func(*testing.T, *gorm.DB, model.Node) {},
			wantSynced:       false,
			wantTargetActive: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newStateTestFixture(t, true)
			node := model.Node{Name: "remote", Enrolled: true, Status: constant.NodeStatusOnline}
			if err := fixture.db.Create(&node).Error; err != nil {
				t.Fatalf("create remote node: %v", err)
			}
			revision := enqueueStateTestEvent(t, fixture.db, "removed", model.BackupSyncOperationDelete)
			if err := markStateTestTargetSuccess(model.BackupSyncTargetLocal, revision, time.Now()); err != nil {
				t.Fatalf("acknowledge local deletion: %v", err)
			}
			test.mutateNode(t, fixture.db, node)

			status, err := GetStatus("removed")
			if err != nil {
				t.Fatalf("get reconciled status: %v", err)
			}
			target := loadTarget(t, fixture.db, NodeTargetKey(node.ID))
			if target.Active != test.wantTargetActive {
				t.Fatalf("remote target active = %t, want %t: %#v", target.Active, test.wantTargetActive, target)
			}
			if test.wantSynced {
				if status.Status != model.BackupSyncStatusSynced || status.Pending != 0 {
					t.Fatalf("status after unavailable node reconciliation = %#v, want synced", status)
				}
				assertOutboxStatus(t, fixture.db, revision, model.BackupSyncOutboxStatusCompleted)
				assertTombstone(t, fixture.db, "removed", revision, false)
				return
			}
			if status.Status == model.BackupSyncStatusSynced || status.Pending == 0 {
				t.Fatalf("online pending node was ignored: %#v", status)
			}
			assertOutboxStatus(t, fixture.db, revision, model.BackupSyncOutboxStatusPending)
			assertTombstone(t, fixture.db, "removed", revision, true)
		})
	}
}

func TestListStatusesIsSortedAndDropsCompletedHistoricalDeletes(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	for _, name := range []string{"zeta", "alpha"} {
		if err := fixture.db.Create(&model.BackupAccount{Name: name, IsPublic: true}).Error; err != nil {
			t.Fatalf("create public account %s: %v", name, err)
		}
		enqueueStateTestEvent(t, fixture.db, name, model.BackupSyncOperationCreate)
	}
	pendingDeleteRevision := enqueueStateTestEvent(t, fixture.db, "pending-delete", model.BackupSyncOperationDelete)
	completedDeleteRevision := enqueueStateTestEvent(t, fixture.db, "completed-delete", model.BackupSyncOperationDelete)

	statuses, err := ListStatuses()
	if err != nil {
		t.Fatalf("list statuses: %v", err)
	}
	assertStatusNames(t, statuses, []string{"alpha", "completed-delete", "pending-delete", "zeta"})

	now := time.Date(2026, 8, 4, 16, 0, 0, 0, time.UTC)
	if err := markStateTestTargetSuccess(model.BackupSyncTargetLocal, completedDeleteRevision, now); err != nil {
		t.Fatalf("complete deletion revisions: %v", err)
	}
	assertTombstone(t, fixture.db, "pending-delete", pendingDeleteRevision, false)
	assertTombstone(t, fixture.db, "completed-delete", completedDeleteRevision, false)

	statuses, err = ListStatuses()
	if err != nil {
		t.Fatalf("list statuses after deletion completion: %v", err)
	}
	assertStatusNames(t, statuses, []string{"alpha", "zeta"})
}

func TestUnknownAccountStatusDoesNotBorrowAnotherAccountRevision(t *testing.T) {
	fixture := newStateTestFixture(t, true)
	if err := fixture.db.Create(&model.BackupAccount{Name: "known", IsPublic: true}).Error; err != nil {
		t.Fatalf("create public account: %v", err)
	}
	enqueueStateTestEvent(t, fixture.db, "known", model.BackupSyncOperationCreate)

	status, err := GetStatus("unknown")
	if err != nil {
		t.Fatalf("get unknown status: %v", err)
	}
	if status.AccountName != "unknown" || status.Revision != 0 || status.Status != model.BackupSyncStatusSynced || len(status.Targets) != 0 {
		t.Fatalf("unknown account borrowed another event: %#v", status)
	}
}

func TestStatusFailsClosedWhenActiveTargetSetIsMissing(t *testing.T) {
	fixture := newStateTestFixture(t, false)
	if err := fixture.db.Create(&model.BackupAccount{Name: "existing", IsPublic: true}).Error; err != nil {
		t.Fatalf("create public account: %v", err)
	}
	if err := fixture.db.Transaction(InitializeTx); err != nil {
		t.Fatalf("initialize synchronization state: %v", err)
	}
	if err := fixture.db.Where("active = ?", true).Delete(&model.BackupSyncTarget{}).Error; err != nil {
		t.Fatalf("remove active target set: %v", err)
	}

	if _, err := GetStatus("existing"); err == nil {
		t.Fatal("missing active target set was reported as synchronized")
	}
}

func TestMissingTargetIsNotReady(t *testing.T) {
	newStateTestFixture(t, true)
	ready, err := TargetReady("node:999")
	if err != nil {
		t.Fatalf("check missing target readiness: %v", err)
	}
	if ready {
		t.Fatal("missing synchronization target was treated as ready")
	}
}

func TestBootstrapAccountStatusDoesNotBorrowLaterAccountRevision(t *testing.T) {
	fixture := newStateTestFixture(t, false)
	if err := fixture.db.Create(&model.BackupAccount{Name: "legacy", IsPublic: true}).Error; err != nil {
		t.Fatalf("create legacy account: %v", err)
	}
	if err := fixture.db.Transaction(InitializeTx); err != nil {
		t.Fatalf("initialize bootstrap state: %v", err)
	}
	if err := fixture.db.Create(&model.BackupAccount{Name: "newer", IsPublic: true}).Error; err != nil {
		t.Fatalf("create newer account: %v", err)
	}
	enqueueStateTestEvent(t, fixture.db, "newer", model.BackupSyncOperationCreate)

	status, err := GetStatus("legacy")
	if err != nil {
		t.Fatalf("get legacy status: %v", err)
	}
	if status.Revision != 1 {
		t.Fatalf("legacy account revision = %d, want bootstrap revision 1", status.Revision)
	}
}

type stateTestFixture struct {
	db    *gorm.DB
	path  string
	oldDB *gorm.DB
}

func newStateTestFixture(t *testing.T, initialize bool) *stateTestFixture {
	t.Helper()
	fixture := &stateTestFixture{
		path:  filepath.Join(t.TempDir(), "backup-sync.db"),
		oldDB: global.DB,
	}
	fixture.db = openStateTestDB(t, fixture.path)
	global.DB = fixture.db
	t.Cleanup(func() {
		closeStateTestDB(fixture.db)
		global.DB = fixture.oldDB
	})
	if initialize {
		if err := fixture.db.Transaction(InitializeTx); err != nil {
			t.Fatalf("initialize sync state: %v", err)
		}
	}
	return fixture
}

func (fixture *stateTestFixture) reopen(t *testing.T) {
	t.Helper()
	closeStateTestDB(fixture.db)
	fixture.db = openStateTestDB(t, fixture.path)
	global.DB = fixture.db
}

func openStateTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	dsn := path + "?_pragma=busy_timeout(5000)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open state test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get state test sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
	} {
		if err := db.Exec(pragma).Error; err != nil {
			t.Fatalf("configure state test database with %q: %v", pragma, err)
		}
	}
	if err := db.AutoMigrate(
		&model.BackupAccount{},
		&model.Node{},
		&model.BackupSyncSequence{},
		&model.BackupSyncOutbox{},
		&model.BackupSyncTarget{},
		&model.BackupSyncTombstone{},
	); err != nil {
		t.Fatalf("migrate state test database: %v", err)
	}
	return db
}

func closeStateTestDB(db *gorm.DB) {
	if db == nil {
		return
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func enqueueStateTestEvent(t *testing.T, db *gorm.DB, accountName, operation string) uint64 {
	t.Helper()
	var revision uint64
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		revision, err = EnqueueTx(tx, accountName, operation)
		return err
	}); err != nil {
		t.Fatalf("enqueue %s for %s: %v", operation, accountName, err)
	}
	return revision
}

func markStateTestTargetSuccess(targetKey string, revision uint64, now time.Time) error {
	sequence, err := CurrentSequence()
	if err != nil {
		return err
	}
	var target model.BackupSyncTarget
	if err := global.DB.Where("target_key = ?", targetKey).First(&target).Error; err != nil {
		return err
	}
	return MarkTargetSuccess(targetKey, SnapshotIdentity{
		Authority:   sequence.Authority,
		Generation:  sequence.Generation,
		TargetEpoch: target.TargetEpoch,
		Revision:    revision,
		Digest:      fmt.Sprintf("%064x", revision),
	}, now)
}

func loadTarget(t *testing.T, db *gorm.DB, targetKey string) model.BackupSyncTarget {
	t.Helper()
	var target model.BackupSyncTarget
	if err := db.Where("target_key = ?", targetKey).First(&target).Error; err != nil {
		t.Fatalf("load target %s: %v", targetKey, err)
	}
	return target
}

func assertTarget(t *testing.T, db *gorm.DB, targetKey string, desired, applied uint64, status string) {
	t.Helper()
	target := loadTarget(t, db, targetKey)
	if target.DesiredRevision != desired || target.AppliedRevision != applied || target.Status != status || !target.Active {
		t.Fatalf("target %s = %#v, want desired=%d applied=%d status=%s active", targetKey, target, desired, applied, status)
	}
}

func assertMissingTarget(t *testing.T, db *gorm.DB, targetKey string) {
	t.Helper()
	var count int64
	if err := db.Model(&model.BackupSyncTarget{}).Where("target_key = ?", targetKey).Count(&count).Error; err != nil {
		t.Fatalf("count target %s: %v", targetKey, err)
	}
	if count != 0 {
		t.Fatalf("target %s exists, want absent", targetKey)
	}
}

func assertRevision(t *testing.T, db *gorm.DB, want uint64) {
	t.Helper()
	got, err := CurrentRevisionTx(db)
	if err != nil {
		t.Fatalf("load current revision: %v", err)
	}
	if got != want {
		t.Fatalf("current revision = %d, want %d", got, want)
	}
}

func assertCount(t *testing.T, db *gorm.DB, value interface{}, want int64) {
	t.Helper()
	var got int64
	if err := db.Model(value).Count(&got).Error; err != nil {
		t.Fatalf("count %T: %v", value, err)
	}
	if got != want {
		t.Fatalf("count %T = %d, want %d", value, got, want)
	}
}

func assertOutboxStatus(t *testing.T, db *gorm.DB, revision uint64, want string) {
	t.Helper()
	var event model.BackupSyncOutbox
	if err := db.Where("revision = ?", revision).First(&event).Error; err != nil {
		t.Fatalf("load outbox revision %d: %v", revision, err)
	}
	if event.Status != want {
		t.Fatalf("outbox revision %d status = %q, want %q", revision, event.Status, want)
	}
}

func assertTombstone(t *testing.T, db *gorm.DB, accountName string, revision uint64, active bool) {
	t.Helper()
	var tombstone model.BackupSyncTombstone
	if err := db.Where("account_name = ?", accountName).First(&tombstone).Error; err != nil {
		t.Fatalf("load tombstone %s: %v", accountName, err)
	}
	if tombstone.Revision != revision || tombstone.Active != active {
		t.Fatalf("tombstone %s = %#v, want revision=%d active=%t", accountName, tombstone, revision, active)
	}
}

func assertDueTargetCount(t *testing.T, now time.Time, want int) {
	t.Helper()
	targets, err := ListDueTargets(now, 100)
	if err != nil {
		t.Fatalf("list due targets: %v", err)
	}
	if len(targets) != want {
		t.Fatalf("due target count = %d, want %d: %#v", len(targets), want, targets)
	}
}

func assertSyncStatus(t *testing.T, accountName, wantStatus string, wantSucceeded, wantPending int) {
	t.Helper()
	status, err := GetStatus(accountName)
	if err != nil {
		t.Fatalf("get status for %s: %v", accountName, err)
	}
	if status.Status != wantStatus || status.Succeeded != wantSucceeded || status.Pending != wantPending {
		t.Fatalf("status for %s = %#v, want status=%s succeeded=%d pending=%d", accountName, status, wantStatus, wantSucceeded, wantPending)
	}
}

func assertStatusNames(t *testing.T, statuses []dto.BackupSyncStatus, want []string) {
	t.Helper()
	got := make([]string, 0, len(statuses))
	for _, status := range statuses {
		got = append(got, status.AccountName)
	}
	if len(got) != len(want) {
		t.Fatalf("status names = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("status names = %v, want %v", got, want)
		}
	}
}
