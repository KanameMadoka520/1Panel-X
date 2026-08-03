package helper

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/init/proxy"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/nodepki"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type multiNodeHelper struct{}

func NewIMultiNodeProvider() *multiNodeHelper {
	return &multiNodeHelper{}
}

// repos used to resolve a node + its dialing material. These packages do not
// import xpack, so there is no import cycle (service -> xpack -> helper).
var (
	nodeRepo        = repo.NewINodeRepo()
	nodeSettingRepo = repo.NewISettingRepo()
)

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
	// N10: a revoked (or offline) node is refused even though its registry row
	// persists for audit — core will not dial it.
	if node.Status == constant.NodeStatusRevoked || node.Status == constant.NodeStatusOffline {
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

func (m *multiNodeHelper) CheckBackupUsed(name string) error { return nil }

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
	payload, err := buildPublicBackupSyncPayload()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return errors.New("encode public backup account sync failed")
	}
	var syncErrors []error
	if err := sendPublicBackupSyncLocal(encoded); err != nil {
		syncErrors = append(syncErrors, fmt.Errorf("sync public backup accounts to local agent: %w", err))
	}
	nodes, err := nodeRepo.GetList()
	if err != nil {
		syncErrors = append(syncErrors, err)
		return errors.Join(syncErrors...)
	}
	if err := syncPublicBackupAccountsToNodes(nodes, encoded, sendPublicBackupSyncNode); err != nil {
		syncErrors = append(syncErrors, err)
	}
	return errors.Join(syncErrors...)
}

func syncPublicBackupAccountsToNodes(
	nodes []model.Node,
	payload []byte,
	send func(model.Node, []byte) error,
) error {
	var syncErrors []error
	for _, node := range nodes {
		if !node.Enrolled {
			continue
		}
		if node.Status != constant.NodeStatusOnline {
			syncErrors = append(syncErrors, fmt.Errorf(
				"public backup account sync is pending for node %q (%s)",
				node.Name,
				node.Status,
			))
			continue
		}
		if err := send(node, payload); err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("sync public backup accounts to node %q: %w", node.Name, err))
		}
	}
	return errors.Join(syncErrors...)
}

func buildPublicBackupSyncPayload() (dto.BackupPublicSync, error) {
	var accounts []model.BackupAccount
	if err := global.DB.Where("is_public = ?", true).Order("id ASC").Find(&accounts).Error; err != nil {
		return dto.BackupPublicSync{}, err
	}
	payload := dto.BackupPublicSync{Accounts: make([]dto.BackupPublicSyncAccount, 0, len(accounts))}
	for _, account := range accounts {
		accessKey, err := encrypt.StringDecrypt(account.AccessKey)
		if err != nil {
			return dto.BackupPublicSync{}, fmt.Errorf("decrypt access key for public backup account %d failed", account.ID)
		}
		credential, err := encrypt.StringDecrypt(account.Credential)
		if err != nil {
			return dto.BackupPublicSync{}, fmt.Errorf("decrypt credential for public backup account %d failed", account.ID)
		}
		vars := account.Vars
		if isPublicBackupOAuthType(account.Type) {
			vars, err = sanitizePublicBackupOAuthVars(vars)
			if err != nil {
				return dto.BackupPublicSync{}, fmt.Errorf("sanitize OAuth metadata for public backup account %d failed", account.ID)
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
			err := global.DB.Where("backup_account_id = ?", account.ID).First(&stored).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return dto.BackupPublicSync{}, err
			}
			if err == nil {
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
						return dto.BackupPublicSync{}, fmt.Errorf("decrypt OAuth client secret for public backup account %d failed", account.ID)
					}
					oauth.RefreshToken, err = encrypt.StringDecryptGCM(stored.RefreshToken, model.BackupOAuthRefreshTokenEncryptionDomain)
					if err != nil {
						return dto.BackupPublicSync{}, fmt.Errorf("decrypt OAuth refresh token for public backup account %d failed", account.ID)
					}
				}
				item.OAuth = oauth
			}
		}
		payload.Accounts = append(payload.Accounts, item)
	}
	return payload, nil
}

func sendPublicBackupSyncLocal(payload []byte) error {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", proxy.SockPath)
		},
	}
	defer transport.CloseIdleConnections()
	return sendPublicBackupSync(&http.Client{Transport: transport, Timeout: 30 * time.Second}, "http://unix/api/v2/backups/public/sync", "", payload)
}

func sendPublicBackupSyncNode(node model.Node, payload []byte) error {
	transport, err := buildNodeTransport(node)
	if err != nil {
		return err
	}
	defer transport.CloseIdleConnections()
	target := "https://" + net.JoinHostPort(node.Addr, node.Port) + "/api/v2/backups/public/sync"
	return sendPublicBackupSync(&http.Client{Transport: transport, Timeout: 30 * time.Second}, target, node.ProxyID, payload)
}

func sendPublicBackupSync(client *http.Client, target, proxyID string, payload []byte) error {
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errors.New("public backup account sync redirect refused")
	}
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return errors.New("create public backup account sync request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	if proxyID != "" {
		req.Header.Set("Proxy-Id", proxyID)
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("public backup account sync request failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errors.New("read public backup account sync response failed")
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New("agent rejected public backup account sync")
	}
	var result dto.Response
	if err := json.Unmarshal(body, &result); err != nil || result.Code != http.StatusOK {
		return errors.New("agent rejected public backup account sync")
	}
	return nil
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
