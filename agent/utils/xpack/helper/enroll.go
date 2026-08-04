package helper

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/global"
	backupcoord "github.com/1Panel-dev/1Panel/agent/utils/backupsync"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/utils/nodepki"
	"gorm.io/gorm"
)

// DefaultProxyIDPath is where the node writes its Proxy-Id (the agent middleware
// reads this file to bind incoming master calls to this node).
const DefaultProxyIDPath = "/etc/1panel/.nodeProxyID"

// EnrollResponse mirrors core's dto.NodeEnrollResponse.
type EnrollResponse struct {
	ServerCert            string `json:"serverCert"`
	CACert                string `json:"caCert"`
	ProxyID               string `json:"proxyID"`
	CoreClientFingerprint string `json:"coreClientFingerprint"`
	BackupSyncAuthority   string `json:"backupSyncAuthority"`
	BackupSyncGeneration  string `json:"backupSyncGeneration"`
	BackupSyncTargetEpoch string `json:"backupSyncTargetEpoch"`
}

// ApplyEnrollment persists the enrollment result so the agent boots into node
// mode on next start: it stores the signed server cert + the node's own key +
// the CA (all encrypted), the pinned master client fingerprint, the node port,
// flips NodeScope to "node", and writes the Proxy-Id file (0600). The node's
// private key is generated locally and never leaves the host (N9).
func ApplyEnrollment(resp EnrollResponse, nodeKeyPEM []byte, port uint, proxyIDFile string) error {
	if resp.ServerCert == "" || resp.CACert == "" || resp.ProxyID == "" || resp.CoreClientFingerprint == "" ||
		!backupcoord.ValidIdentity(resp.BackupSyncAuthority) || !backupcoord.ValidIdentity(resp.BackupSyncGeneration) ||
		!backupcoord.ValidIdentity(resp.BackupSyncTargetEpoch) {
		return fmt.Errorf("incomplete enrollment response")
	}
	releaseMutation := backupcoord.AcquireMutation()
	defer releaseMutation()
	serverCrtEnc, err := encrypt.StringEncrypt(resp.ServerCert)
	if err != nil {
		return err
	}
	serverKeyEnc, err := encrypt.StringEncrypt(string(nodeKeyPEM))
	if err != nil {
		return err
	}
	rootCrtEnc, err := encrypt.StringEncrypt(resp.CACert)
	if err != nil {
		return err
	}
	updates := map[string]string{
		"ServerCrt":               serverCrtEnc,
		"ServerKey":               serverKeyEnc,
		"RootCrt":                 rootCrtEnc,
		"MasterClientFingerprint": strings.ToLower(strings.TrimSpace(resp.CoreClientFingerprint)),
		"NodePort":                fmt.Sprintf("%d", port),
		"NodeScope":               "node",
	}
	if proxyIDFile == "" {
		proxyIDFile = DefaultProxyIDPath
	}
	tempProxyID, err := prepareEnrollmentProxyID(proxyIDFile, resp.ProxyID)
	if err != nil {
		return err
	}
	defer os.Remove(tempProxyID)

	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		for key, value := range updates {
			if err := upsertEnrollmentSettingTx(tx, key, value); err != nil {
				return err
			}
		}
		var syncState model.BackupPublicSyncState
		stateErr := tx.Where("id = ?", model.BackupPublicSyncStateID).First(&syncState).Error
		if errors.Is(stateErr, gorm.ErrRecordNotFound) {
			syncState = model.BackupPublicSyncState{ID: model.BackupPublicSyncStateID}
			if err := tx.Create(&syncState).Error; err != nil {
				return err
			}
		} else if stateErr != nil {
			return stateErr
		}
		return tx.Model(&syncState).Updates(map[string]interface{}{
			"authority":        resp.BackupSyncAuthority,
			"generation":       resp.BackupSyncGeneration,
			"target_epoch":     resp.BackupSyncTargetEpoch,
			"applied_revision": 0,
			"applied_digest":   "",
			"applied_at":       nil,
		}).Error
	}); err != nil {
		return err
	}

	// The database already carries revision zero for the new target epoch. If
	// the atomic replacement fails, public operations remain fail closed and a
	// repeated enrollment application can finish the filesystem side safely.
	return os.Rename(tempProxyID, proxyIDFile)
}

func prepareEnrollmentProxyID(proxyIDFile, proxyID string) (string, error) {
	dir := filepath.Dir(proxyIDFile)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(proxyIDFile)+".*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return "", err
	}
	if _, err := temp.WriteString(proxyID); err != nil {
		cleanup()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	return tempPath, nil
}

func upsertEnrollmentSettingTx(tx *gorm.DB, key, value string) error {
	var setting model.Setting
	err := tx.Where("key = ?", key).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&model.Setting{Key: key, Value: value}).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&setting).UpdateColumn("value", value).Error
}

// Enroll performs the node-join handshake against core: it reads the master
// fingerprint from the (HMAC-authenticated) token to pin core (N4), generates a
// local keypair + CSR, posts them to core's enrollment endpoint, and applies the
// signed result. Core imposes the certificate subject/SANs from its registry, so
// the CSR carries only the public key. The live call is exercised in the
// cross-network acceptance (Slice C); the persistence half is unit-tested.
func Enroll(coreBaseURL, token string, port uint, proxyIDFile string) error {
	claims, err := nodepki.ParseTokenClaims(token)
	if err != nil {
		return err
	}
	keyPEM, csrPEM, err := nodepki.GenerateKeyAndCSR("node", nil, nil)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"token": token, "csr": string(csrPEM)})
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: nodepki.EnrollTLSConfig(claims.MasterFingerprint)},
	}
	url := strings.TrimRight(coreBaseURL, "/") + "/api/v2/core/nodes/enroll"
	httpResp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	body, _ := io.ReadAll(httpResp.Body)

	var wrapper struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    EnrollResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return fmt.Errorf("unexpected enrollment response: %s", string(body))
	}
	if httpResp.StatusCode != http.StatusOK || wrapper.Code != http.StatusOK && wrapper.Code != 0 {
		return fmt.Errorf("enrollment rejected: %s", wrapper.Message)
	}
	return ApplyEnrollment(wrapper.Data, keyPEM, port, proxyIDFile)
}
