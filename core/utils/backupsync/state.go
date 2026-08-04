package backupsync

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxStatusErrorRunes           = 240
	PublicBackupSyncLeaseDuration = 5 * time.Minute
	publicBackupSyncRenewAfter    = 2 * time.Minute
	sqliteWriteRetryAttempts      = 8
	sqliteWriteRetryInitialDelay  = 2 * time.Millisecond
	sqliteWriteRetryMaxDelay      = 50 * time.Millisecond
)

var deliveryBarrier sync.RWMutex
var desiredStateBarrier sync.RWMutex

type SnapshotIdentity struct {
	Authority   string
	Generation  string
	TargetEpoch string
	Revision    uint64
	Digest      string
}

// AcquireDeliveryBarrier serializes secret-bearing snapshot delivery with node
// revoke/delete. The returned release function must be called exactly once.
func AcquireDeliveryBarrier() func() {
	deliveryBarrier.Lock()
	return deliveryBarrier.Unlock
}

// AcquireSnapshotDeliveryBarrier marks an active secret-bearing delivery.
// A waiting revoke/delete blocks subsequent deliveries until it commits.
func AcquireSnapshotDeliveryBarrier() func() {
	deliveryBarrier.RLock()
	return deliveryBarrier.RUnlock
}

// AcquireDesiredStateMutation blocks backup execution guards while a public
// account mutation and its revision are committed atomically.
func AcquireDesiredStateMutation() func() {
	desiredStateBarrier.Lock()
	return desiredStateBarrier.Unlock
}

// AcquireDesiredStateExecution keeps the verified desired revision stable
// until the guarded Agent request has been handed off.
func AcquireDesiredStateExecution() func() {
	desiredStateBarrier.RLock()
	return desiredStateBarrier.RUnlock
}

func NodeTargetKey(nodeID uint) string {
	return fmt.Sprintf("node:%d", nodeID)
}

func InitializeTx(tx *gorm.DB) error {
	if err := ensureSequenceTx(tx); err != nil {
		return err
	}
	sequence, err := CurrentSequenceTx(tx)
	if err != nil {
		return err
	}
	if _, err := ensureTargetTx(tx, model.BackupSyncTargetLocal, 0, sequence.Generation, sequence.Revision); err != nil {
		return err
	}
	if err := ensureTargetEpochsTx(tx); err != nil {
		return err
	}
	var outboxCount int64
	if err := tx.Model(&model.BackupSyncOutbox{}).Count(&outboxCount).Error; err != nil {
		return err
	}
	if outboxCount != 0 {
		return nil
	}
	_, err = EnqueueTx(tx, "", model.BackupSyncOperationBootstrap)
	return err
}

func EnqueueTx(tx *gorm.DB, accountName, operation string) (uint64, error) {
	if err := ensureSequenceTx(tx); err != nil {
		return 0, err
	}
	if err := tx.Model(&model.BackupSyncSequence{}).
		Where("id = ?", model.BackupSyncSequenceID).
		Updates(map[string]interface{}{
			"revision":        gorm.Expr("revision + 1"),
			"snapshot_digest": "",
		}).Error; err != nil {
		return 0, err
	}
	sequence, err := CurrentSequenceTx(tx)
	if err != nil {
		return 0, err
	}
	event := model.BackupSyncOutbox{
		Generation:  sequence.Generation,
		Revision:    sequence.Revision,
		AccountName: strings.TrimSpace(accountName),
		Operation:   operation,
		Status:      model.BackupSyncOutboxStatusPending,
	}
	if err := tx.Create(&event).Error; err != nil {
		return 0, err
	}
	if err := updateTombstoneTx(tx, event.AccountName, operation, sequence.Generation, sequence.Revision); err != nil {
		return 0, err
	}
	if _, err := ensureTargetTx(tx, model.BackupSyncTargetLocal, 0, sequence.Generation, sequence.Revision); err != nil {
		return 0, err
	}
	var nodes []model.Node
	if err := tx.Where("enrolled = ? AND status <> ?", true, constant.NodeStatusRevoked).Find(&nodes).Error; err != nil {
		return 0, err
	}
	for _, node := range nodes {
		if _, err := ensureTargetTx(tx, NodeTargetKey(node.ID), node.ID, sequence.Generation, sequence.Revision); err != nil {
			return 0, err
		}
	}
	return sequence.Revision, nil
}

func CurrentRevisionTx(tx *gorm.DB) (uint64, error) {
	sequence, err := CurrentSequenceTx(tx)
	if err != nil {
		return 0, err
	}
	return sequence.Revision, nil
}

func CurrentSequenceTx(tx *gorm.DB) (model.BackupSyncSequence, error) {
	var sequence model.BackupSyncSequence
	if err := tx.Where("id = ?", model.BackupSyncSequenceID).First(&sequence).Error; err != nil {
		return model.BackupSyncSequence{}, err
	}
	if !validSyncIdentity(sequence.Authority) || !validSyncIdentity(sequence.Generation) ||
		(sequence.SnapshotDigest != "" && !validSyncIdentity(sequence.SnapshotDigest)) {
		return model.BackupSyncSequence{}, errors.New("backup synchronization identity is unavailable")
	}
	return sequence, nil
}

func CurrentRevision() (uint64, error) {
	return CurrentRevisionTx(global.DB)
}

func CurrentSequence() (model.BackupSyncSequence, error) {
	return CurrentSequenceTx(global.DB)
}

func BindSnapshotDigestTx(tx *gorm.DB, identity SnapshotIdentity) error {
	sequence, err := CurrentSequenceTx(tx)
	if err != nil {
		return err
	}
	if sequence.Authority != identity.Authority || sequence.Generation != identity.Generation || sequence.Revision != identity.Revision {
		return errors.New("backup synchronization state changed while building the snapshot")
	}
	if sequence.SnapshotDigest != "" {
		if sequence.SnapshotDigest != identity.Digest {
			return errors.New("backup synchronization snapshot changed without a new revision")
		}
		return nil
	}
	result := tx.Model(&model.BackupSyncSequence{}).
		Where("id = ? AND authority = ? AND generation = ? AND revision = ? AND snapshot_digest = ?",
			model.BackupSyncSequenceID, identity.Authority, identity.Generation, identity.Revision, "").
		Update("snapshot_digest", identity.Digest)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var current model.BackupSyncSequence
	if err := tx.Where("id = ?", model.BackupSyncSequenceID).First(&current).Error; err != nil {
		return err
	}
	if current.Authority == identity.Authority && current.Generation == identity.Generation && current.Revision == identity.Revision && current.SnapshotDigest == identity.Digest {
		return nil
	}
	return errors.New("backup synchronization state changed while binding the snapshot digest")
}

func EnqueueStartupReconciliation() error {
	release := AcquireDesiredStateMutation()
	defer release()
	return global.DB.Transaction(func(tx *gorm.DB) error {
		_, err := EnqueueTx(tx, "", model.BackupSyncOperationBootstrap)
		return err
	})
}

func EnsureNodeTarget(nodeID uint) error {
	release := AcquireDesiredStateMutation()
	defer release()
	return global.DB.Transaction(func(tx *gorm.DB) error {
		return EnsureNodeTargetTx(tx, nodeID)
	})
}

func EnsureNodeTargetTx(tx *gorm.DB, nodeID uint) error {
	var node model.Node
	if err := tx.Where("id = ?", nodeID).First(&node).Error; err != nil {
		return err
	}
	if !node.Enrolled || node.Status == constant.NodeStatusRevoked {
		return DeactivateNodeTargetTx(tx, nodeID)
	}
	var existing model.BackupSyncTarget
	targetErr := tx.Where("target_key = ?", NodeTargetKey(nodeID)).First(&existing).Error
	if errors.Is(targetErr, gorm.ErrRecordNotFound) || (targetErr == nil && !existing.Active) {
		_, err := EnqueueTx(tx, "", model.BackupSyncOperationBootstrap)
		return err
	}
	if targetErr != nil {
		return targetErr
	}
	sequence, err := CurrentSequenceTx(tx)
	if err != nil {
		return err
	}
	if sequence.Revision == 0 {
		_, err = EnqueueTx(tx, "", model.BackupSyncOperationBootstrap)
		return err
	}
	_, err = ensureTargetTx(tx, NodeTargetKey(nodeID), nodeID, sequence.Generation, sequence.Revision)
	return err
}

func DeactivateNodeTarget(nodeID uint) error {
	return global.DB.Transaction(func(tx *gorm.DB) error {
		return DeactivateNodeTargetTx(tx, nodeID)
	})
}

func DeactivateNodeTargetTx(tx *gorm.DB, nodeID uint) error {
	if err := tx.Model(&model.BackupSyncTarget{}).
		Where("target_key = ?", NodeTargetKey(nodeID)).
		Updates(map[string]interface{}{
			"active":        false,
			"status":        model.BackupSyncTargetStatusSynced,
			"last_error":    "",
			"next_retry_at": nil,
		}).Error; err != nil {
		return err
	}
	return refreshCompletedTx(tx)
}

func ListDueTargets(now time.Time, limit int) ([]model.BackupSyncTarget, error) {
	if limit <= 0 {
		limit = 100
	}
	var targets []model.BackupSyncTarget
	err := global.DB.Where(
		"active = ? AND (desired_generation <> applied_generation OR desired_revision > applied_revision OR last_success_at IS NULL OR last_success_at <= ?) AND (next_retry_at IS NULL OR next_retry_at <= ?)",
		true,
		now.Add(-publicBackupSyncRenewAfter),
		now,
	).Order(
		"CASE WHEN last_attempt_at IS NULL THEN 0 WHEN status = 'pending' THEN 1 ELSE 2 END ASC, " +
			"CASE WHEN last_attempt_at IS NULL THEN created_at ELSE last_attempt_at END ASC, id ASC",
	).Limit(limit).Find(&targets).Error
	return targets, err
}

func MarkTargetAttempt(targetKey string, now time.Time) error {
	return retryTransientSQLiteWrite(func() error {
		result := global.DB.Model(&model.BackupSyncTarget{}).
			Where("target_key = ? AND active = ?", targetKey, true).
			Update("last_attempt_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func GetActiveTarget(targetKey string) (model.BackupSyncTarget, error) {
	var target model.BackupSyncTarget
	err := global.DB.Where("target_key = ? AND active = ?", targetKey, true).First(&target).Error
	return target, err
}

func RetryAccount(accountName string) error {
	name := strings.TrimSpace(accountName)
	now := time.Now()
	sequence, err := CurrentSequence()
	if err != nil {
		return err
	}
	var event model.BackupSyncOutbox
	err = global.DB.Where("generation = ? AND account_name = ?", sequence.Generation, name).Order("revision DESC").First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		visible, visibleErr := statusAccountExists(name, sequence.Generation)
		if visibleErr != nil {
			return visibleErr
		}
		if !visible {
			return nil
		}
		err = global.DB.Where("generation = ? AND account_name = ?", sequence.Generation, "").Order("revision DESC").First(&event).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return retryTransientSQLiteWrite(func() error {
		return global.DB.Model(&model.BackupSyncTarget{}).
			Where(
				"active = ? AND (applied_generation <> ? OR applied_revision < ? OR last_success_at IS NULL OR last_success_at <= ? OR status = ?)",
				true,
				event.Generation,
				event.Revision,
				now.Add(-PublicBackupSyncLeaseDuration),
				model.BackupSyncTargetStatusFailed,
			).
			Updates(map[string]interface{}{
				"status":        model.BackupSyncTargetStatusPending,
				"next_retry_at": nil,
				"last_error":    "",
			}).Error
	})
}

func statusAccountExists(accountName, generation string) (bool, error) {
	var count int64
	if err := global.DB.Model(&model.BackupAccount{}).
		Where("is_public = ? AND name = ?", true, accountName).
		Count(&count).Error; err != nil {
		return false, err
	}
	if count != 0 {
		return true, nil
	}
	if err := global.DB.Model(&model.BackupSyncTombstone{}).
		Where("active = ? AND generation = ? AND account_name = ?", true, generation, accountName).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count != 0, nil
}

func RetryTarget(targetKey string) error {
	now := time.Now()
	sequence, err := CurrentSequence()
	if err != nil {
		return err
	}
	return retryTransientSQLiteWrite(func() error {
		return global.DB.Model(&model.BackupSyncTarget{}).
			Where(
				"target_key = ? AND active = ? AND desired_generation = ? AND (desired_generation <> applied_generation OR desired_revision > applied_revision OR last_success_at IS NULL OR last_success_at <= ? OR status = ?)",
				targetKey,
				true,
				sequence.Generation,
				now.Add(-PublicBackupSyncLeaseDuration),
				model.BackupSyncTargetStatusFailed,
			).
			Updates(map[string]interface{}{
				"status":        model.BackupSyncTargetStatusPending,
				"next_retry_at": nil,
				"last_error":    "",
			}).Error
	})
}

func TargetReady(targetKey string) (bool, error) {
	var target model.BackupSyncTarget
	err := global.DB.Where("target_key = ? AND active = ?", targetKey, true).First(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	sequence, err := CurrentSequence()
	if err != nil {
		return false, err
	}
	return targetLeaseFresh(target.LastSuccessAt, time.Now()) &&
		sequence.SnapshotDigest != "" &&
		validSyncIdentity(target.TargetEpoch) &&
		target.DesiredGeneration == sequence.Generation &&
		target.DesiredRevision == sequence.Revision &&
		target.AppliedTargetEpoch == target.TargetEpoch &&
		target.AppliedAuthority == sequence.Authority &&
		target.AppliedGeneration == sequence.Generation &&
		target.AppliedRevision == sequence.Revision &&
		target.AppliedDigest == sequence.SnapshotDigest, nil
}

func MarkTargetSuccess(targetKey string, applied SnapshotIdentity, now time.Time) error {
	return retryTransientSQLiteWrite(func() error {
		return global.DB.Transaction(func(tx *gorm.DB) error {
			var target model.BackupSyncTarget
			if err := tx.Where("target_key = ?", targetKey).First(&target).Error; err != nil {
				return err
			}
			sequence, err := CurrentSequenceTx(tx)
			if err != nil {
				return err
			}
			if applied.Authority != sequence.Authority || applied.Generation != sequence.Generation ||
				!validSyncIdentity(applied.TargetEpoch) || applied.TargetEpoch != target.TargetEpoch ||
				applied.Digest == "" || applied.Revision == 0 || applied.Revision > sequence.Revision {
				return errors.New("invalid backup synchronization acknowledgement")
			}
			if target.DesiredGeneration != applied.Generation {
				return nil
			}
			if target.AppliedGeneration == applied.Generation && target.AppliedRevision > applied.Revision {
				return nil
			}
			if target.AppliedGeneration == applied.Generation && target.AppliedRevision == applied.Revision && target.AppliedDigest != "" && target.AppliedDigest != applied.Digest {
				return errors.New("conflicting backup synchronization acknowledgement")
			}
			target.AppliedTargetEpoch = applied.TargetEpoch
			target.AppliedAuthority = applied.Authority
			target.AppliedGeneration = applied.Generation
			target.AppliedRevision = applied.Revision
			target.AppliedDigest = applied.Digest
			target.Attempts = 0
			target.LastAttemptAt = &now
			target.LastSuccessAt = &now
			target.LastError = ""
			target.NextRetryAt = nil
			if target.AppliedGeneration == target.DesiredGeneration && target.AppliedRevision >= target.DesiredRevision {
				target.Status = model.BackupSyncTargetStatusSynced
			} else {
				target.Status = model.BackupSyncTargetStatusPending
			}
			if err := tx.Save(&target).Error; err != nil {
				return err
			}
			return refreshCompletedTx(tx)
		})
	})
}

func MarkTargetFailure(targetKey string, syncErr error, now time.Time) error {
	return retryTransientSQLiteWrite(func() error {
		return global.DB.Transaction(func(tx *gorm.DB) error {
			var target model.BackupSyncTarget
			if err := tx.Where("target_key = ?", targetKey).First(&target).Error; err != nil {
				return err
			}
			target.Attempts++
			target.Status = model.BackupSyncTargetStatusFailed
			target.LastAttemptAt = &now
			target.LastError = sanitizeError(syncErr)
			nextRetry := now.Add(retryDelay(target.Attempts))
			target.NextRetryAt = &nextRetry
			return tx.Save(&target).Error
		})
	})
}

func retryTransientSQLiteWrite(write func() error) error {
	delay := sqliteWriteRetryInitialDelay
	for attempt := 0; attempt < sqliteWriteRetryAttempts; attempt++ {
		err := write()
		if err == nil || !isTransientSQLiteWriteError(err) || attempt == sqliteWriteRetryAttempts-1 {
			return err
		}
		time.Sleep(delay)
		delay = min(delay*2, sqliteWriteRetryMaxDelay)
	}
	return nil
}

func isTransientSQLiteWriteError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "database is deadlocked")
}

func GetStatus(accountName string) (dto.BackupSyncStatus, error) {
	name := strings.TrimSpace(accountName)
	sequence, err := CurrentSequence()
	if err != nil {
		return dto.BackupSyncStatus{}, err
	}
	var event model.BackupSyncOutbox
	err = global.DB.Where("generation = ? AND account_name = ?", sequence.Generation, name).Order("revision DESC").First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		visible, visibleErr := statusAccountExists(name, sequence.Generation)
		if visibleErr != nil {
			return dto.BackupSyncStatus{}, visibleErr
		}
		if !visible {
			return dto.BackupSyncStatus{
				AccountName: name,
				Status:      model.BackupSyncStatusSynced,
				Targets:     []dto.BackupSyncTargetStatus{},
			}, nil
		}
		err = global.DB.Where("generation = ? AND account_name = ?", sequence.Generation, "").Order("revision DESC").First(&event).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.BackupSyncStatus{
			AccountName: name,
			Status:      model.BackupSyncStatusSynced,
			Targets:     []dto.BackupSyncTargetStatus{},
		}, nil
	}
	if err != nil {
		return dto.BackupSyncStatus{}, err
	}
	return buildStatus(name, event.Generation, event.Revision)
}

func ListStatuses() ([]dto.BackupSyncStatus, error) {
	sequence, err := CurrentSequence()
	if err != nil {
		return nil, err
	}
	names := make(map[string]struct{})
	var accounts []model.BackupAccount
	if err := global.DB.Where("is_public = ?", true).Find(&accounts).Error; err != nil {
		return nil, err
	}
	for _, account := range accounts {
		names[account.Name] = struct{}{}
	}
	var tombstones []model.BackupSyncTombstone
	if err := global.DB.Where("active = ? AND generation = ?", true, sequence.Generation).Find(&tombstones).Error; err != nil {
		return nil, err
	}
	for _, tombstone := range tombstones {
		names[tombstone.AccountName] = struct{}{}
	}
	orderedNames := make([]string, 0, len(names))
	for name := range names {
		orderedNames = append(orderedNames, name)
	}
	sort.Strings(orderedNames)

	result := make([]dto.BackupSyncStatus, 0, len(orderedNames))
	for _, name := range orderedNames {
		status, err := GetStatus(name)
		if err != nil {
			return nil, err
		}
		result = append(result, status)
	}
	return result, nil
}

func ensureSequenceTx(tx *gorm.DB) error {
	authority, err := newSyncIdentity()
	if err != nil {
		return err
	}
	generation, err := newSyncIdentity()
	if err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.BackupSyncSequence{
		ID:         model.BackupSyncSequenceID,
		Authority:  authority,
		Generation: generation,
	}).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.BackupSyncSequence{}).
		Where("id = ? AND authority = ?", model.BackupSyncSequenceID, "").
		Update("authority", authority).Error; err != nil {
		return err
	}
	return tx.Model(&model.BackupSyncSequence{}).
		Where("id = ? AND generation = ?", model.BackupSyncSequenceID, "").
		Update("generation", generation).Error
}

func ensureTargetTx(tx *gorm.DB, targetKey string, nodeID uint, desiredGeneration string, desiredRevision uint64) (model.BackupSyncTarget, error) {
	var target model.BackupSyncTarget
	err := tx.Where("target_key = ?", targetKey).First(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		targetEpoch, epochErr := newSyncIdentity()
		if epochErr != nil {
			return model.BackupSyncTarget{}, epochErr
		}
		status := model.BackupSyncTargetStatusSynced
		if desiredRevision != 0 {
			status = model.BackupSyncTargetStatusPending
		}
		target = model.BackupSyncTarget{
			TargetKey:         targetKey,
			NodeID:            nodeID,
			Active:            true,
			TargetEpoch:       targetEpoch,
			DesiredGeneration: desiredGeneration,
			DesiredRevision:   desiredRevision,
			Status:            status,
		}
		return target, tx.Create(&target).Error
	}
	if err != nil {
		return model.BackupSyncTarget{}, err
	}
	if !validSyncIdentity(target.TargetEpoch) {
		targetEpoch, epochErr := newSyncIdentity()
		if epochErr != nil {
			return model.BackupSyncTarget{}, epochErr
		}
		target.TargetEpoch = targetEpoch
		target.AppliedTargetEpoch = ""
		target.AppliedAuthority = ""
		target.AppliedGeneration = ""
		target.AppliedRevision = 0
		target.AppliedDigest = ""
		target.LastSuccessAt = nil
	}
	target.NodeID = nodeID
	target.Active = true
	if desiredGeneration != "" && desiredGeneration != target.DesiredGeneration {
		target.DesiredGeneration = desiredGeneration
		target.DesiredRevision = desiredRevision
	} else if desiredRevision > target.DesiredRevision {
		target.DesiredRevision = desiredRevision
	}
	if target.AppliedTargetEpoch != target.TargetEpoch || target.AppliedGeneration != target.DesiredGeneration || target.AppliedRevision < target.DesiredRevision {
		if target.Status != model.BackupSyncTargetStatusFailed {
			target.Status = model.BackupSyncTargetStatusPending
			target.NextRetryAt = nil
			target.LastError = ""
		}
	} else {
		target.Status = model.BackupSyncTargetStatusSynced
	}
	return target, tx.Save(&target).Error
}

func RotateNodeTargetEpochTx(tx *gorm.DB, nodeID uint) (string, error) {
	targetEpoch, err := newSyncIdentity()
	if err != nil {
		return "", err
	}
	result := tx.Model(&model.BackupSyncTarget{}).
		Where("target_key = ? AND node_id = ? AND active = ?", NodeTargetKey(nodeID), nodeID, true).
		Updates(map[string]interface{}{
			"target_epoch":         targetEpoch,
			"applied_target_epoch": "",
			"applied_authority":    "",
			"applied_generation":   "",
			"applied_revision":     0,
			"applied_digest":       "",
			"status":               model.BackupSyncTargetStatusPending,
			"attempts":             0,
			"next_retry_at":        nil,
			"last_attempt_at":      nil,
			"last_success_at":      nil,
			"last_error":           "",
		})
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected != 1 {
		return "", gorm.ErrRecordNotFound
	}
	return targetEpoch, nil
}

func ensureTargetEpochsTx(tx *gorm.DB) error {
	var targets []model.BackupSyncTarget
	if err := tx.Find(&targets).Error; err != nil {
		return err
	}
	for _, target := range targets {
		if validSyncIdentity(target.TargetEpoch) {
			continue
		}
		targetEpoch, err := newSyncIdentity()
		if err != nil {
			return err
		}
		if err := tx.Model(&model.BackupSyncTarget{}).
			Where("id = ?", target.ID).
			Updates(map[string]interface{}{
				"target_epoch":         targetEpoch,
				"applied_target_epoch": "",
				"last_success_at":      nil,
				"status":               model.BackupSyncTargetStatusPending,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func updateTombstoneTx(tx *gorm.DB, accountName, operation, generation string, revision uint64) error {
	if accountName == "" {
		return nil
	}
	var tombstone model.BackupSyncTombstone
	err := tx.Where("account_name = ?", accountName).First(&tombstone).Error
	if operation != model.BackupSyncOperationDelete {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return tx.Model(&tombstone).Updates(map[string]interface{}{
			"active":     false,
			"generation": generation,
			"revision":   revision,
		}).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&model.BackupSyncTombstone{
			AccountName: accountName,
			Generation:  generation,
			Revision:    revision,
			Active:      true,
		}).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&tombstone).Updates(map[string]interface{}{
		"active":     true,
		"generation": generation,
		"revision":   revision,
	}).Error
}

func refreshCompletedTx(tx *gorm.DB) error {
	if err := deactivateUnavailableNodeTargetsTx(tx); err != nil {
		return err
	}
	var targets []model.BackupSyncTarget
	if err := tx.Where("active = ?", true).Find(&targets).Error; err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("backup synchronization targets are unavailable")
	}
	sequence, err := CurrentSequenceTx(tx)
	if err != nil {
		return err
	}
	minimum := targets[0].AppliedRevision
	for _, target := range targets {
		if target.AppliedTargetEpoch != target.TargetEpoch || target.AppliedAuthority != sequence.Authority || target.AppliedGeneration != sequence.Generation {
			minimum = 0
			break
		}
		if target.AppliedRevision < minimum {
			minimum = target.AppliedRevision
		}
	}
	if err := tx.Model(&model.BackupSyncOutbox{}).
		Where("generation = ? AND revision <= ?", sequence.Generation, minimum).
		Update("status", model.BackupSyncOutboxStatusCompleted).Error; err != nil {
		return err
	}
	return tx.Model(&model.BackupSyncTombstone{}).
		Where("active = ? AND generation = ? AND revision <= ?", true, sequence.Generation, minimum).
		Update("active", false).Error
}

func deactivateUnavailableNodeTargetsTx(tx *gorm.DB) error {
	var targets []model.BackupSyncTarget
	if err := tx.Where("active = ? AND node_id <> ?", true, 0).Find(&targets).Error; err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}

	nodeIDs := make([]uint, 0, len(targets))
	for _, target := range targets {
		nodeIDs = append(nodeIDs, target.NodeID)
	}
	var nodes []model.Node
	if err := tx.Where("id IN ?", nodeIDs).Find(&nodes).Error; err != nil {
		return err
	}
	availableNodes := make(map[uint]struct{}, len(nodes))
	for _, node := range nodes {
		if node.Status != constant.NodeStatusRevoked {
			availableNodes[node.ID] = struct{}{}
		}
	}

	invalidTargetKeys := make([]string, 0)
	for _, target := range targets {
		if _, exists := availableNodes[target.NodeID]; !exists {
			invalidTargetKeys = append(invalidTargetKeys, target.TargetKey)
		}
	}
	if len(invalidTargetKeys) == 0 {
		return nil
	}
	return tx.Model(&model.BackupSyncTarget{}).
		Where("target_key IN ?", invalidTargetKeys).
		Updates(map[string]interface{}{
			"active":        false,
			"status":        model.BackupSyncTargetStatusSynced,
			"last_error":    "",
			"next_retry_at": nil,
		}).Error
}

func buildStatus(accountName, generation string, revision uint64) (dto.BackupSyncStatus, error) {
	now := time.Now()
	status := dto.BackupSyncStatus{
		AccountName: accountName,
		Revision:    revision,
		Status:      model.BackupSyncStatusSynced,
		Targets:     []dto.BackupSyncTargetStatus{},
	}
	err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := refreshCompletedTx(tx); err != nil {
			return err
		}
		var targets []model.BackupSyncTarget
		if err := tx.Where("active = ?", true).Order("id ASC").Find(&targets).Error; err != nil {
			return err
		}
		var nodes []model.Node
		if err := tx.Find(&nodes).Error; err != nil {
			return err
		}
		nodesByID := make(map[uint]model.Node, len(nodes))
		for _, node := range nodes {
			nodesByID[node.ID] = node
		}
		status.Targets = make([]dto.BackupSyncTargetStatus, 0, len(targets))
		sequence, err := CurrentSequenceTx(tx)
		if err != nil {
			return err
		}
		for _, target := range targets {
			nodeName := "local"
			if target.NodeID != 0 {
				nodeName = nodesByID[target.NodeID].Name
			}
			targetStatus := model.BackupSyncTargetStatusPending
			applied := targetLeaseFresh(target.LastSuccessAt, now) &&
				validSyncIdentity(target.TargetEpoch) &&
				target.AppliedTargetEpoch == target.TargetEpoch &&
				target.DesiredGeneration == sequence.Generation &&
				target.AppliedAuthority == sequence.Authority &&
				target.AppliedGeneration == generation &&
				target.AppliedRevision >= revision &&
				target.AppliedDigest != ""
			if applied {
				targetStatus = model.BackupSyncTargetStatusSynced
			} else if target.Status == model.BackupSyncTargetStatusFailed {
				targetStatus = model.BackupSyncTargetStatusFailed
			}
			item := dto.BackupSyncTargetStatus{
				TargetKey:       target.TargetKey,
				NodeID:          target.NodeID,
				NodeName:        nodeName,
				Status:          targetStatus,
				DesiredRevision: target.DesiredRevision,
				AppliedRevision: target.AppliedRevision,
				Attempts:        target.Attempts,
				LastError:       target.LastError,
			}
			if target.NextRetryAt != nil {
				item.NextRetryAt = target.NextRetryAt.UTC().Format(time.RFC3339)
			}
			if target.LastSuccessAt != nil {
				item.LastSuccessAt = target.LastSuccessAt.UTC().Format(time.RFC3339)
			}
			status.Total++
			if applied {
				status.Succeeded++
			} else {
				status.Pending++
			}
			status.Targets = append(status.Targets, item)
		}
		return nil
	})
	if err != nil {
		return dto.BackupSyncStatus{}, err
	}
	if status.Pending == 0 {
		status.Status = model.BackupSyncStatusSynced
	} else if status.Succeeded == 0 {
		status.Status = model.BackupSyncStatusPending
	} else {
		status.Status = model.BackupSyncStatusPartiallySynced
	}
	return status, nil
}

func targetLeaseFresh(lastSuccessAt *time.Time, now time.Time) bool {
	if lastSuccessAt == nil || lastSuccessAt.After(now.Add(time.Minute)) {
		return false
	}
	return now.Sub(*lastSuccessAt) <= PublicBackupSyncLeaseDuration
}

func validSyncIdentity(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func retryDelay(attempts uint) time.Duration {
	shift := attempts - 1
	if shift > 8 {
		shift = 8
	}
	delay := 5 * time.Second * time.Duration(1<<shift)
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}

func sanitizeError(err error) string {
	if err == nil {
		return "synchronization failed"
	}
	cleaned := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(err.Error()))
	if cleaned == "" {
		return "synchronization failed"
	}
	runes := []rune(cleaned)
	if len(runes) > maxStatusErrorRunes {
		cleaned = string(runes[:maxStatusErrorRunes])
	}
	return cleaned
}

func newSyncIdentity() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", errors.New("generate backup synchronization identity failed")
	}
	return hex.EncodeToString(raw), nil
}
