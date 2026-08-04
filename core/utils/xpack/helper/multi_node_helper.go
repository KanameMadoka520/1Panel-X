package helper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/init/proxy"
	"github.com/1Panel-dev/1Panel/core/utils/backupsync"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/nodepki"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type multiNodeHelper struct{}

const publicBackupSyncConcurrency = 4

func NewIMultiNodeProvider() *multiNodeHelper {
	return &multiNodeHelper{}
}

// repos used to resolve a node + its dialing material. These packages do not
// import xpack, so there is no import cycle (service -> xpack -> helper).
var (
	nodeRepo          = repo.NewINodeRepo()
	nodeSettingRepo   = repo.NewISettingRepo()
	backupSyncBatch   = make(chan struct{}, 1)
	backupSyncLocksMu sync.Mutex
	backupSyncLocks   = make(map[string]*backupSyncTargetLock)
)

type backupSyncTargetLock struct {
	mu   sync.Mutex
	refs int
}

func acquireBackupSyncTargetLock(targetKey string) func() {
	backupSyncLocksMu.Lock()
	entry := backupSyncLocks[targetKey]
	if entry == nil {
		entry = &backupSyncTargetLock{}
		backupSyncLocks[targetKey] = entry
	}
	entry.refs++
	backupSyncLocksMu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		backupSyncLocksMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(backupSyncLocks, targetKey)
		}
		backupSyncLocksMu.Unlock()
	}
}

// Proxy forwards a browser request to the target agent. For the local ("local"
// or "") node it uses the existing unix-socket reverse proxy; for a remote node
// it dials the node's mTLS listener with core's client cert, verifying the CA
// chain AND pinning the node's exact server-cert fingerprint (N5/N8/N12/N14).
func (m *multiNodeHelper) Proxy(c *gin.Context, currentNode string) {
	if currentNode == "local" || currentNode == "" {
		m.proxyLocal(c)
		return
	}

	node, err := resolveNode(currentNode)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"code": http.StatusBadGateway, "message": err.Error()})
		return
	}
	transport, err := buildNodeTransport(node)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"code": http.StatusBadGateway, "message": err.Error()})
		return
	}

	// target is derived solely from the registry row, never from client input (N14).
	target := &url.URL{Scheme: "https", Host: net.JoinHostPort(node.Addr, node.Port)}
	reverse := httputil.NewSingleHostReverseProxy(target)
	reverse.Transport = transport
	reverse.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, e error) {
		global.LOG.Warnf("node proxy error node=%s: %v", node.Name, e)
		w.WriteHeader(http.StatusBadGateway)
	}
	// bind this call to the enrolled node (agent verifies Proxy-Id).
	c.Request.Header.Set("Proxy-Id", node.ProxyID)

	defer func() {
		if err := recover(); err != nil && err != http.ErrAbortHandler {
			global.LOG.Debug(err)
		}
	}()
	reverse.ServeHTTP(c.Writer, c.Request)
	c.Abort()
}

func (m *multiNodeHelper) proxyLocal(c *gin.Context) {
	defer func() {
		if err := recover(); err != nil && err != http.ErrAbortHandler {
			global.LOG.Debug(err)
		}
	}()
	proxy.LocalAgentProxy.ServeHTTP(c.Writer, c.Request)
	c.Abort()
}

// resolveNode looks up a node by address, then name, then numeric id.
func resolveNode(currentNode string) (model.Node, error) {
	if n, err := nodeRepo.Get(repo.WithByAddr(currentNode)); err == nil && n.ID != 0 {
		return n, nil
	}
	if n, err := nodeRepo.Get(repo.WithByName(currentNode)); err == nil && n.ID != 0 {
		return n, nil
	}
	if id, err := strconv.ParseUint(currentNode, 10, 64); err == nil {
		if n, err := nodeRepo.Get(repo.WithByID(uint(id))); err == nil && n.ID != 0 {
			return n, nil
		}
	}
	return model.Node{}, fmt.Errorf("unknown node %q", currentNode)
}

// buildNodeTransport builds the pinned mTLS transport for one node.
func buildNodeTransport(node model.Node) (*http.Transport, error) {
	return buildNodeTransportWithOffline(node, false)
}

func buildNodeTransportWithOffline(node model.Node, allowOffline bool) (*http.Transport, error) {
	// N10: a revoked (or offline) node is refused even though its registry row
	// persists for audit — core will not dial it.
	if node.Status == constant.NodeStatusRevoked || (!allowOffline && node.Status == constant.NodeStatusOffline) {
		return nil, fmt.Errorf("node %q is not available (%s)", node.Name, node.Status)
	}
	if node.ServerFingerprint == "" {
		return nil, fmt.Errorf("node %q is not enrolled", node.Name)
	}
	caCert, _ := nodeSettingRepo.GetValueByKey(constant.NodeCACertKey)
	clientCert, _ := nodeSettingRepo.GetValueByKey(constant.NodeCoreCertKey)
	clientKeyEnc, _ := nodeSettingRepo.GetValueByKey(constant.NodeCoreKeyKey)
	if caCert == "" || clientCert == "" || clientKeyEnc == "" {
		return nil, fmt.Errorf("node PKI material not initialised")
	}
	clientKey, err := encrypt.StringDecrypt(clientKeyEnc)
	if err != nil {
		return nil, err
	}
	cfg, err := nodepki.ClientTLSConfig([]byte(caCert), []byte(clientCert), []byte(clientKey), node.ServerFingerprint)
	if err != nil {
		return nil, err
	}
	return &http.Transport{
		TLSClientConfig: cfg,
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       15 * time.Second,
	}, nil
}

func (m *multiNodeHelper) ProxyDocker(proxyURL string) error { return nil }

func (m *multiNodeHelper) UpdateGroup(name string, group, newGroup uint) error { return nil }

func (m *multiNodeHelper) CheckBackupUsed(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("backup account name is required")
	}

	var nodes []model.Node
	if err := global.DB.
		Where("enrolled = ? AND status <> ?", true, constant.NodeStatusRevoked).
		Order("id ASC").
		Find(&nodes).Error; err != nil {
		return errors.New("load registered nodes for backup usage check failed")
	}
	for _, node := range nodes {
		// An explicitly offline node cannot answer a live usage query. Allow the
		// authoritative delete transaction to persist its tombstone so the node
		// removes the account when it reconnects.
		if node.Status == constant.NodeStatusOffline {
			continue
		}
		if err := checkBackupUsedOnNode(node, name); err != nil {
			return fmt.Errorf("verify backup account usage on node %q failed: %w", node.Name, err)
		}
	}
	return nil
}

func checkBackupUsedOnNode(node model.Node, name string) error {
	transport, err := buildNodeTransport(node)
	if err != nil {
		return errors.New("node transport is unavailable")
	}
	defer transport.CloseIdleConnections()

	target := "https://" + net.JoinHostPort(node.Addr, node.Port) + "/api/v2/backups/check/" + url.PathEscape(name)
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return errors.New("create backup usage check request failed")
	}
	req.Header.Set("Proxy-Id", node.ProxyID)
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("backup usage check redirect refused")
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("backup usage check request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return errors.New("node returned a non-success backup usage check status")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errors.New("read backup usage check response failed")
	}
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return errors.New("node returned an invalid backup usage check response")
	}
	if envelope.Code != http.StatusOK {
		return errors.New("backup account may still be used by a node schedule")
	}
	return nil
}

func (m *multiNodeHelper) EnsureBackupSyncReady(currentNode string) error {
	targetKey := model.BackupSyncTargetLocal
	if currentNode != "" && currentNode != "local" {
		node, err := resolveNode(currentNode)
		if err != nil {
			return err
		}
		if !node.Enrolled || node.Status == constant.NodeStatusRevoked {
			return fmt.Errorf("node %q is not available for public backup reconciliation", node.Name)
		}
		// Target creation belongs to migration, startup reconciliation, and the
		// enrollment transaction. This execution guard must stay read-only so it
		// never attempts to upgrade the desired-state read lock to a write lock.
		targetKey = backupsync.NodeTargetKey(node.ID)
	}
	ready, err := backupsync.TargetReady(targetKey)
	if err != nil {
		return errors.New("public backup account synchronization state is unavailable")
	}
	if ready {
		return nil
	}
	if err := backupsync.RetryTarget(targetKey); err != nil {
		return errors.New("public backup account synchronization is pending; retry after reconciliation")
	}
	_ = m.syncTarget(targetKey)
	ready, err = backupsync.TargetReady(targetKey)
	if err != nil || !ready {
		return errors.New("public backup account synchronization is pending; retry after reconciliation")
	}
	return nil
}

// LoadRequestTransport is a generic outbound transport used elsewhere; the node
// proxy path builds its own pinned transport via buildNodeTransport.
func (m *multiNodeHelper) LoadRequestTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       15 * time.Second,
	}
}

// LoadNodeInfo returns SSH connection info for a node. SSH-based remote access
// is out of scope for the token+mTLS enrollment model (no SSH creds collected),
// so this stays a no-op; HTTP federation goes through Proxy above.
func (m *multiNodeHelper) LoadNodeInfo(currentNode string) (*ssh.ConnInfo, string, error) {
	return nil, "", nil
}

func (m *multiNodeHelper) Sync(dataType string) error {
	if dataType != constant.SyncBackupAccounts {
		return nil
	}
	select {
	case backupSyncBatch <- struct{}{}:
		defer func() { <-backupSyncBatch }()
	default:
		return nil
	}

	targets, err := backupsync.ListDueTargets(time.Now(), 100)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	return syncPublicBackupTargets(targets)
}

func (m *multiNodeHelper) syncTarget(targetKey string) error {
	return syncPublicBackupTargets([]model.BackupSyncTarget{{TargetKey: targetKey}})
}

func syncPublicBackupTargets(targets []model.BackupSyncTarget) error {
	payload, err := buildPublicBackupSyncPayload()
	if err != nil {
		for _, target := range targets {
			_ = backupsync.MarkTargetFailure(target.TargetKey, errors.New("build public backup snapshot failed"), time.Now())
		}
		return err
	}
	workerCount := min(len(targets), publicBackupSyncConcurrency)
	jobs := make(chan model.BackupSyncTarget)
	syncErrors := make(chan error, len(targets))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for target := range jobs {
				if syncErr := syncPublicBackupTarget(target.TargetKey, payload); syncErr != nil {
					syncErrors <- syncErr
				}
			}
		}()
	}
	for _, target := range targets {
		jobs <- target
	}
	close(jobs)
	workers.Wait()
	close(syncErrors)

	var joined []error
	for syncErr := range syncErrors {
		joined = append(joined, syncErr)
	}
	return errors.Join(joined...)
}

func syncPublicBackupTarget(targetKey string, basePayload dto.BackupPublicSync) error {
	releaseTarget := acquireBackupSyncTargetLock(targetKey)
	defer releaseTarget()
	releaseDeliveryBarrier := backupsync.AcquireSnapshotDeliveryBarrier()
	defer releaseDeliveryBarrier()

	target, err := backupsync.GetActiveTarget(targetKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !validPublicBackupSyncHex(target.TargetEpoch) {
		syncErr := errors.New("backup synchronization target epoch is unavailable")
		_ = backupsync.MarkTargetFailure(targetKey, syncErr, time.Now())
		return syncErr
	}
	if err := backupsync.MarkTargetAttempt(targetKey, time.Now()); err != nil {
		return err
	}

	payload := basePayload
	payload.TargetEpoch = target.TargetEpoch
	encoded, err := json.Marshal(payload)
	if err != nil {
		syncErr := errors.New("encode public backup account sync failed")
		_ = backupsync.MarkTargetFailure(targetKey, syncErr, time.Now())
		return syncErr
	}

	var result dto.BackupPublicSyncResult
	if target.TargetKey == model.BackupSyncTargetLocal {
		result, err = sendPublicBackupSyncLocal(encoded)
	} else {
		var node model.Node
		node, err = nodeRepo.Get(repo.WithByID(target.NodeID))
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && (!node.Enrolled || node.Status == constant.NodeStatusRevoked)) {
			return backupsync.DeactivateNodeTarget(target.NodeID)
		}
		if err == nil {
			result, err = sendPublicBackupSyncNode(node, encoded)
		}
		if err == nil && node.Status == constant.NodeStatusOffline {
			_ = markReconciledNodeOnline(node.ID)
		}
	}
	if err != nil {
		if markErr := backupsync.MarkTargetFailure(targetKey, err, time.Now()); markErr != nil {
			return errors.Join(fmt.Errorf("sync public backup accounts to %s: %w", targetKey, err), markErr)
		}
		return fmt.Errorf("sync public backup accounts to %s: %w", targetKey, err)
	}
	if !validPublicBackupSyncAcknowledgement(payload, result) {
		ackErr := errors.New("agent acknowledged an invalid public backup revision")
		if markErr := backupsync.MarkTargetFailure(targetKey, ackErr, time.Now()); markErr != nil {
			return errors.Join(ackErr, markErr)
		}
		return ackErr
	}
	return backupsync.MarkTargetSuccess(targetKey, backupsync.SnapshotIdentity{
		Authority:   result.Authority,
		Generation:  result.Generation,
		TargetEpoch: result.TargetEpoch,
		Revision:    result.AppliedRevision,
		Digest:      result.SnapshotDigest,
	}, time.Now())
}

func markReconciledNodeOnline(nodeID uint) error {
	return global.DB.Model(&model.Node{}).
		Where("id = ? AND enrolled = ? AND status = ?", nodeID, true, constant.NodeStatusOffline).
		Update("status", constant.NodeStatusOnline).Error
}

func buildPublicBackupSyncPayload() (dto.BackupPublicSync, error) {
	var payload dto.BackupPublicSync
	err := global.DB.Transaction(func(tx *gorm.DB) error {
		sequence, err := backupsync.CurrentSequenceTx(tx)
		if err != nil {
			return err
		}
		payload.Authority = sequence.Authority
		payload.Generation = sequence.Generation
		payload.Revision = sequence.Revision
		var accounts []model.BackupAccount
		if err := tx.Where("is_public = ?", true).Order("id ASC").Find(&accounts).Error; err != nil {
			return err
		}
		payload.Accounts = make([]dto.BackupPublicSyncAccount, 0, len(accounts))
		for _, account := range accounts {
			accessKey, err := encrypt.StringDecrypt(account.AccessKey)
			if err != nil {
				return fmt.Errorf("decrypt access key for public backup account %d failed", account.ID)
			}
			credential, err := encrypt.StringDecrypt(account.Credential)
			if err != nil {
				return fmt.Errorf("decrypt credential for public backup account %d failed", account.ID)
			}
			vars := account.Vars
			if isPublicBackupOAuthType(account.Type) {
				vars, err = sanitizePublicBackupOAuthVars(vars)
				if err != nil {
					return fmt.Errorf("sanitize OAuth metadata for public backup account %d failed", account.ID)
				}
			}
			item := dto.BackupPublicSyncAccount{
				Name:         account.Name,
				Type:         account.Type,
				IsPublic:     true,
				Bucket:       account.Bucket,
				AccessKey:    accessKey,
				Credential:   credential,
				BackupPath:   account.BackupPath,
				Vars:         vars,
				RememberAuth: account.RememberAuth,
			}
			if isPublicBackupOAuthType(account.Type) {
				var stored model.BackupOAuthCredential
				storedErr := tx.Where("backup_account_id = ?", account.ID).First(&stored).Error
				if storedErr != nil && !errors.Is(storedErr, gorm.ErrRecordNotFound) {
					return storedErr
				}
				if storedErr == nil {
					oauth := &dto.BackupOAuthSecretSync{
						Provider:     stored.Provider,
						ClientID:     stored.ClientID,
						RedirectURI:  stored.RedirectURI,
						IsCN:         stored.IsCN,
						Status:       stored.Status,
						AuthorizedAt: stored.AuthorizedAt,
					}
					if stored.Status != model.BackupOAuthStatusLegacyReconfigurationRequired {
						oauth.ClientSecret, err = encrypt.StringDecryptGCM(stored.ClientSecret, model.BackupOAuthClientSecretEncryptionDomain)
						if err != nil {
							return fmt.Errorf("decrypt OAuth client secret for public backup account %d failed", account.ID)
						}
						oauth.RefreshToken, err = encrypt.StringDecryptGCM(stored.RefreshToken, model.BackupOAuthRefreshTokenEncryptionDomain)
						if err != nil {
							return fmt.Errorf("decrypt OAuth refresh token for public backup account %d failed", account.ID)
						}
					}
					item.OAuth = oauth
				}
			}
			payload.Accounts = append(payload.Accounts, item)
		}
		var tombstones []model.BackupSyncTombstone
		if err := tx.Where("active = ? AND generation = ? AND revision <= ?", true, sequence.Generation, sequence.Revision).Order("revision ASC").Find(&tombstones).Error; err != nil {
			return err
		}
		payload.Tombstones = make([]dto.BackupPublicSyncTombstone, 0, len(tombstones))
		for _, tombstone := range tombstones {
			payload.Tombstones = append(payload.Tombstones, dto.BackupPublicSyncTombstone{
				Name:     tombstone.AccountName,
				Revision: tombstone.Revision,
			})
		}
		sealed, err := sealPublicBackupSyncPayload(payload)
		if err != nil {
			return err
		}
		payload = sealed
		return backupsync.BindSnapshotDigestTx(tx, backupsync.SnapshotIdentity{
			Authority:  payload.Authority,
			Generation: payload.Generation,
			Revision:   payload.Revision,
			Digest:     payload.SnapshotDigest,
		})
	})
	return payload, err
}

func sealPublicBackupSyncPayload(payload dto.BackupPublicSync) (dto.BackupPublicSync, error) {
	payload.Authority = strings.ToLower(strings.TrimSpace(payload.Authority))
	payload.Generation = strings.ToLower(strings.TrimSpace(payload.Generation))
	if !validPublicBackupSyncHex(payload.Authority) || !validPublicBackupSyncHex(payload.Generation) || payload.Revision == 0 {
		return dto.BackupPublicSync{}, errors.New("invalid public backup synchronization identity")
	}
	if payload.Accounts == nil {
		payload.Accounts = []dto.BackupPublicSyncAccount{}
	}
	if payload.Tombstones == nil {
		payload.Tombstones = []dto.BackupPublicSyncTombstone{}
	}
	accountNames := make(map[string]struct{}, len(payload.Accounts))
	for index := range payload.Accounts {
		name := strings.TrimSpace(payload.Accounts[index].Name)
		if name == "" || !payload.Accounts[index].IsPublic {
			return dto.BackupPublicSync{}, errors.New("invalid public backup account snapshot")
		}
		if _, exists := accountNames[name]; exists {
			return dto.BackupPublicSync{}, fmt.Errorf("duplicate public backup account %q", name)
		}
		accountNames[name] = struct{}{}
		payload.Accounts[index].Name = name
		if payload.Accounts[index].OAuth != nil {
			oauth := *payload.Accounts[index].OAuth
			if oauth.AuthorizedAt != nil {
				authorizedAt := oauth.AuthorizedAt.UTC()
				oauth.AuthorizedAt = &authorizedAt
			}
			payload.Accounts[index].OAuth = &oauth
		}
	}
	tombstoneNames := make(map[string]struct{}, len(payload.Tombstones))
	for index := range payload.Tombstones {
		name := strings.TrimSpace(payload.Tombstones[index].Name)
		if name == "" || payload.Tombstones[index].Revision == 0 || payload.Tombstones[index].Revision > payload.Revision {
			return dto.BackupPublicSync{}, errors.New("invalid public backup account tombstone")
		}
		if _, exists := tombstoneNames[name]; exists {
			return dto.BackupPublicSync{}, fmt.Errorf("duplicate public backup account tombstone %q", name)
		}
		if _, exists := accountNames[name]; exists {
			return dto.BackupPublicSync{}, fmt.Errorf("public backup account %q is both present and deleted", name)
		}
		tombstoneNames[name] = struct{}{}
		payload.Tombstones[index].Name = name
	}
	sort.Slice(payload.Accounts, func(i, j int) bool { return payload.Accounts[i].Name < payload.Accounts[j].Name })
	sort.Slice(payload.Tombstones, func(i, j int) bool {
		if payload.Tombstones[i].Name == payload.Tombstones[j].Name {
			return payload.Tombstones[i].Revision < payload.Tombstones[j].Revision
		}
		return payload.Tombstones[i].Name < payload.Tombstones[j].Name
	})
	digest, err := publicBackupSyncDigest(payload)
	if err != nil {
		return dto.BackupPublicSync{}, errors.New("calculate public backup snapshot digest failed")
	}
	payload.SnapshotDigest = digest
	return payload, nil
}

func publicBackupSyncDigest(payload dto.BackupPublicSync) (string, error) {
	// TargetEpoch is an independently validated per-target replay boundary. The
	// content digest stays identical across targets so Core can bind one desired
	// snapshot digest to the global monotonic revision.
	canonical := struct {
		Authority  string                          `json:"authority"`
		Generation string                          `json:"generation"`
		Revision   uint64                          `json:"revision"`
		Accounts   []dto.BackupPublicSyncAccount   `json:"accounts"`
		Tombstones []dto.BackupPublicSyncTombstone `json:"tombstones"`
	}{
		Authority:  payload.Authority,
		Generation: payload.Generation,
		Revision:   payload.Revision,
		Accounts:   payload.Accounts,
		Tombstones: payload.Tombstones,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validPublicBackupSyncAcknowledgement(sent dto.BackupPublicSync, result dto.BackupPublicSyncResult) bool {
	if result.Result != "applied" && result.Result != "already_applied" {
		return false
	}
	return result.Authority == sent.Authority &&
		result.Generation == sent.Generation &&
		result.TargetEpoch == sent.TargetEpoch &&
		result.AppliedRevision == sent.Revision &&
		validPublicBackupSyncHex(result.SnapshotDigest) &&
		subtle.ConstantTimeCompare([]byte(result.SnapshotDigest), []byte(sent.SnapshotDigest)) == 1
}

func validPublicBackupSyncHex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sendPublicBackupSyncLocal(payload []byte) (dto.BackupPublicSyncResult, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", proxy.SockPath)
		},
	}
	defer transport.CloseIdleConnections()
	return sendPublicBackupSync(&http.Client{Transport: transport, Timeout: 30 * time.Second}, "http://unix/api/v2/backups/public/sync", "", payload)
}

func sendPublicBackupSyncNode(node model.Node, payload []byte) (dto.BackupPublicSyncResult, error) {
	transport, err := buildNodeTransportWithOffline(node, true)
	if err != nil {
		return dto.BackupPublicSyncResult{}, err
	}
	defer transport.CloseIdleConnections()
	target := "https://" + net.JoinHostPort(node.Addr, node.Port) + "/api/v2/backups/public/sync"
	return sendPublicBackupSync(&http.Client{Transport: transport, Timeout: 30 * time.Second}, target, node.ProxyID, payload)
}

func sendPublicBackupSync(client *http.Client, target, proxyID string, payload []byte) (dto.BackupPublicSyncResult, error) {
	var result dto.BackupPublicSyncResult
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errors.New("public backup account sync redirect refused")
	}
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return result, errors.New("create public backup account sync request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	if proxyID != "" {
		req.Header.Set("Proxy-Id", proxyID)
	}
	resp, err := client.Do(req)
	if err != nil {
		return result, errors.New("public backup account sync request failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return result, errors.New("read public backup account sync response failed")
	}
	if resp.StatusCode != http.StatusOK {
		return result, errors.New("agent rejected public backup account sync")
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Code != http.StatusOK || len(envelope.Data) == 0 {
		return result, errors.New("agent rejected public backup account sync")
	}
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return result, errors.New("agent returned an invalid public backup sync acknowledgement")
	}
	return result, nil
}

func isPublicBackupOAuthType(backupType string) bool {
	return backupType == constant.OneDrive || backupType == constant.GoogleDrive || backupType == constant.ALIYUN
}

func sanitizePublicBackupOAuthVars(raw string) (string, error) {
	vars := make(map[string]interface{})
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &vars); err != nil {
			return "", err
		}
	}
	if vars == nil {
		vars = make(map[string]interface{})
	}
	for key := range vars {
		if isSensitivePublicBackupOAuthVarKey(key) {
			delete(vars, key)
		}
	}
	encoded, err := json.Marshal(vars)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func isSensitivePublicBackupOAuthVarKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "accesstoken",
		"authorizationcode",
		"authorizationresponse",
		"authorizationurl",
		"clientid",
		"clientsecret",
		"code",
		"codechallenge",
		"codeverifier",
		"flowid",
		"idtoken",
		"oauthsession",
		"pkceverifier",
		"redirecturi",
		"refreshtoken",
		"sessionid",
		"state",
		"token":
		return true
	default:
		return false
	}
}

func (m *multiNodeHelper) AutoUpgradeWithMaster() {}
