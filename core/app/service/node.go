package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/backupsync"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/nodepki"
	"gorm.io/gorm"
)

type NodeService struct{}

type INodeService interface {
	List(req dto.NodeSearch) ([]dto.NodeInfo, error)
	SimpleAll() ([]dto.SimpleNodeInfo, error)
	Create(req dto.NodeCreate) (*dto.NodeEnrollToken, error)
	Delete(id uint) error
	Revoke(id uint) error
	Enroll(req dto.NodeEnrollRequest) (*dto.NodeEnrollResponse, error)
}

func NewINodeService() INodeService {
	return &NodeService{}
}

// pkiMu serialises the lazy one-time PKI bootstrap so two concurrent admin
// "add node" calls cannot each generate a competing CA.
var pkiMu sync.Mutex

type nodePKI struct {
	ca             *nodepki.CA
	coreClientCert []byte
	coreClientKey  []byte
	coreClientFP   string
	enrollSecret   []byte
}

func (s *NodeService) List(req dto.NodeSearch) ([]dto.NodeInfo, error) {
	nodes, err := nodeRepo.GetList(repo.WithOrderDesc("created_at"))
	if err != nil {
		return nil, err
	}
	items := make([]dto.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, dto.NodeInfo{
			ID:      n.ID,
			GroupID: n.GroupID,
			Addr:    n.Addr,
			Status:  n.Status,
			Version: n.Version,
			IsXpack: false,
			IsBound: n.Status == constant.NodeStatusOnline,
			Name:    n.Name,
		})
	}
	return items, nil
}

func (s *NodeService) SimpleAll() ([]dto.SimpleNodeInfo, error) {
	nodes, err := nodeRepo.GetList(repo.WithOrderDesc("created_at"))
	if err != nil {
		return nil, err
	}
	items := make([]dto.SimpleNodeInfo, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, dto.SimpleNodeInfo{
			ID:            n.ID,
			Name:          n.Name,
			Addr:          n.Addr,
			SystemVersion: n.Version,
		})
	}
	return items, nil
}

func (s *NodeService) Create(req dto.NodeCreate) (*dto.NodeEnrollToken, error) {
	if exist, _ := nodeRepo.Get(repo.WithByName(req.Name)); exist.ID != 0 {
		return nil, buserr.New("ErrRecordExist")
	}
	if err := validateNodeAddr(req.Addr); err != nil {
		return nil, err
	}
	if err := validateNodePort(req.Port); err != nil {
		return nil, err
	}
	pki, err := s.ensurePKI()
	if err != nil {
		return nil, err
	}

	proxyID, err := randomHex(24)
	if err != nil {
		return nil, err
	}
	nonce, err := nodepki.NewNonce()
	if err != nil {
		return nil, err
	}
	expireAt := time.Now().Add(nodepki.MaxTokenTTL).Unix()
	node := &model.Node{
		Name:         req.Name,
		Addr:         req.Addr,
		Port:         req.Port,
		GroupID:      req.GroupID,
		Scope:        "node",
		Status:       constant.NodeStatusPending,
		ProxyID:      proxyID,
		EnrollNonce:  nonce,
		EnrollExpire: expireAt,
	}
	if err := nodeRepo.Create(node); err != nil {
		return nil, err
	}

	claims := nodepki.TokenClaims{
		NodeID:            node.ID,
		Nonce:             nonce,
		Exp:               expireAt,
		MasterFingerprint: coreServerFingerprint(),
	}
	token, err := nodepki.IssueToken(pki.enrollSecret, claims)
	if err != nil {
		return nil, err
	}
	return &dto.NodeEnrollToken{
		NodeID:   node.ID,
		Token:    token,
		ExpireAt: expireAt,
		Addr:     req.Addr,
		Port:     req.Port,
	}, nil
}

func (s *NodeService) Delete(id uint) error {
	if _, err := nodeRepo.Get(repo.WithByID(id)); err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	releaseDeliveryBarrier := backupsync.AcquireDeliveryBarrier()
	defer releaseDeliveryBarrier()
	return global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Delete(&model.Node{}).Error; err != nil {
			return err
		}
		return backupsync.DeactivateNodeTargetTx(tx, id)
	})
}

// Revoke marks a node revoked without deleting its registry row (N10). A revoked
// node is refused by the proxy dial path (buildNodeTransport), so a suspected
// node is cut off immediately while its record is preserved for audit.
func (s *NodeService) Revoke(id uint) error {
	if _, err := nodeRepo.Get(repo.WithByID(id)); err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	releaseDeliveryBarrier := backupsync.AcquireDeliveryBarrier()
	defer releaseDeliveryBarrier()
	return global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Node{}).Where("id = ?", id).Update("status", constant.NodeStatusRevoked).Error; err != nil {
			return err
		}
		return backupsync.DeactivateNodeTargetTx(tx, id)
	})
}

// Enroll is called BY a joining node (token-gated, no session). It verifies the
// enrollment token (HMAC + expiry), atomically consumes it (single-use), signs
// the node's CSR into a per-node server leaf, pins the node's fingerprint, and
// returns the certs + core client fingerprint the node needs to run node mode.
func (s *NodeService) Enroll(req dto.NodeEnrollRequest) (*dto.NodeEnrollResponse, error) {
	pki, err := s.ensurePKI()
	if err != nil {
		return nil, err
	}
	claims, err := nodepki.VerifyToken(pki.enrollSecret, strings.TrimSpace(req.Token))
	if err != nil {
		return nil, err // N2/N3: bad signature or expired
	}
	node, err := nodeRepo.Get(repo.WithByID(claims.NodeID))
	if err != nil {
		return nil, buserr.New("ErrRecordNotFound")
	}
	if node.Status == constant.NodeStatusRevoked || node.EnrollNonce != claims.Nonce {
		return nil, fmt.Errorf("enrollment not permitted for this node")
	}

	// Sign the CSR first (validate the request) so a bad CSR does not waste the
	// single-use token. CN is imposed by core, not taken from the CSR (N13).
	leaf, err := pki.ca.SignLeaf([]byte(req.CSR), nodepki.LeafOptions{
		CommonName:  fmt.Sprintf("node-%d", node.ID),
		DNSNames:    dnsSANs(node.Addr),
		IPAddresses: ipSANs(node.Addr),
		ForServer:   true,
	})
	if err != nil {
		return nil, err
	}

	fp, err := nodepki.FingerprintPEM(leaf)
	if err != nil {
		return nil, err
	}
	releaseDesiredState := backupsync.AcquireDesiredStateMutation()
	defer releaseDesiredState()
	var targetEpoch string
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		var finalizeErr error
		targetEpoch, finalizeErr = finalizeNodeEnrollmentTx(tx, node.ID, claims.Nonce, fp)
		return finalizeErr
	}); err != nil {
		return nil, err
	}
	sequence, err := backupsync.CurrentSequence()
	if err != nil {
		return nil, err
	}
	// N13: audit the highest-value security event — a node was signed and bound.
	global.LOG.Infof("node enrolled: id=%d name=%s addr=%s fingerprint=%s", node.ID, node.Name, node.Addr, fp)
	return &dto.NodeEnrollResponse{
		ServerCert:            string(leaf),
		CACert:                string(pki.ca.CertPEM),
		ProxyID:               node.ProxyID,
		CoreClientFingerprint: pki.coreClientFP,
		BackupSyncAuthority:   sequence.Authority,
		BackupSyncGeneration:  sequence.Generation,
		BackupSyncTargetEpoch: targetEpoch,
	}, nil
}

func finalizeNodeEnrollmentTx(tx *gorm.DB, nodeID uint, nonce, fingerprint string) (string, error) {
	result := tx.Model(&model.Node{}).
		Where(
			"id = ? AND enroll_nonce = ? AND enrolled = ? AND status <> ?",
			nodeID,
			nonce,
			false,
			constant.NodeStatusRevoked,
		).
		Updates(map[string]interface{}{
			"enrolled":           true,
			"server_fingerprint": fingerprint,
			"status":             constant.NodeStatusOnline,
		})
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected != 1 {
		return "", fmt.Errorf("enrollment token already used or node revoked")
	}
	if err := backupsync.EnsureNodeTargetTx(tx, nodeID); err != nil {
		return "", err
	}
	return backupsync.RotateNodeTargetEpochTx(tx, nodeID)
}

// ensurePKI loads the core-owned node PKI, generating it once on first use.
func (s *NodeService) ensurePKI() (*nodePKI, error) {
	caCert, _ := settingRepo.GetValueByKey(constant.NodeCACertKey)
	caKeyEnc, _ := settingRepo.GetValueByKey(constant.NodeCAKeyKey)
	if caCert == "" || caKeyEnc == "" {
		return s.initPKI()
	}
	caKeyPEM, err := encrypt.StringDecrypt(caKeyEnc)
	if err != nil {
		return nil, err
	}
	ca, err := nodepki.LoadCA([]byte(caCert), []byte(caKeyPEM))
	if err != nil {
		return nil, err
	}
	clientCert, _ := settingRepo.GetValueByKey(constant.NodeCoreCertKey)
	clientKeyEnc, _ := settingRepo.GetValueByKey(constant.NodeCoreKeyKey)
	clientKeyPEM, err := encrypt.StringDecrypt(clientKeyEnc)
	if err != nil {
		return nil, err
	}
	secretEnc, _ := settingRepo.GetValueByKey(constant.NodeEnrollSecretKey)
	secretB64, err := encrypt.StringDecrypt(secretEnc)
	if err != nil {
		return nil, err
	}
	secret, err := base64.StdEncoding.DecodeString(secretB64)
	if err != nil {
		return nil, err
	}
	fp, err := nodepki.FingerprintPEM([]byte(clientCert))
	if err != nil {
		return nil, err
	}
	return &nodePKI{
		ca:             ca,
		coreClientCert: []byte(clientCert),
		coreClientKey:  []byte(clientKeyPEM),
		coreClientFP:   fp,
		enrollSecret:   secret,
	}, nil
}

func (s *NodeService) initPKI() (*nodePKI, error) {
	pkiMu.Lock()
	defer pkiMu.Unlock()
	// re-check under the lock in case another request just initialised it
	if caCert, _ := settingRepo.GetValueByKey(constant.NodeCACertKey); caCert != "" {
		return s.ensurePKI()
	}

	caCertPEM, caKeyPEM, err := nodepki.GenerateCA("1Panel-X Node CA")
	if err != nil {
		return nil, err
	}
	ca, err := nodepki.LoadCA(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, err
	}
	clientKeyPEM, clientCSR, err := nodepki.GenerateKeyAndCSR("1panel-core", nil, nil)
	if err != nil {
		return nil, err
	}
	clientCertPEM, err := ca.SignLeaf(clientCSR, nodepki.LeafOptions{CommonName: "1panel-core", ForClient: true})
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}

	caKeyEnc, err := encrypt.StringEncrypt(string(caKeyPEM))
	if err != nil {
		return nil, err
	}
	clientKeyEnc, err := encrypt.StringEncrypt(string(clientKeyPEM))
	if err != nil {
		return nil, err
	}
	secretEnc, err := encrypt.StringEncrypt(base64.StdEncoding.EncodeToString(secret))
	if err != nil {
		return nil, err
	}
	for k, v := range map[string]string{
		constant.NodeCACertKey:       string(caCertPEM),
		constant.NodeCAKeyKey:        caKeyEnc,
		constant.NodeCoreCertKey:     string(clientCertPEM),
		constant.NodeCoreKeyKey:      clientKeyEnc,
		constant.NodeEnrollSecretKey: secretEnc,
	} {
		if err := settingRepo.UpdateOrCreate(k, v); err != nil {
			return nil, err
		}
	}
	fp, err := nodepki.FingerprintPEM(clientCertPEM)
	if err != nil {
		return nil, err
	}
	return &nodePKI{
		ca:             ca,
		coreClientCert: clientCertPEM,
		coreClientKey:  clientKeyPEM,
		coreClientFP:   fp,
		enrollSecret:   secret,
	}, nil
}

// coreServerFingerprint reads core's browser-facing TLS cert (if TLS is on) so
// the enrollment token can carry the master fingerprint a joining node pins
// before sending its CSR (N4). Best-effort: empty when core serves plain HTTP.
func coreServerFingerprint() string {
	certPath := path.Join(global.CONF.Base.InstallDir, "1panel/secret/server.crt")
	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		return ""
	}
	fp, err := nodepki.FingerprintPEM(pemBytes)
	if err != nil {
		return ""
	}
	return fp
}

// ---- validation + SAN helpers ----

// validateNodeAddr enforces a bare host or IP (N14 SSRF defense): no scheme, no
// credentials, no path, no whitespace.
func validateNodeAddr(addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fmt.Errorf("node address is required")
	}
	if strings.ContainsAny(addr, " \t\r\n/@?#\\") || strings.Contains(addr, "://") {
		return fmt.Errorf("invalid node address")
	}
	if ip := net.ParseIP(addr); ip != nil {
		return nil
	}
	// hostname: labels of [A-Za-z0-9-], dot-separated
	for _, label := range strings.Split(addr, ".") {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("invalid node address")
		}
		for _, r := range label {
			if !(r == '-' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
				return fmt.Errorf("invalid node address")
			}
		}
	}
	return nil
}

func validateNodePort(port string) error {
	port = strings.TrimSpace(port)
	if port == "" {
		return fmt.Errorf("node port is required")
	}
	n := 0
	for _, r := range port {
		if r < '0' || r > '9' {
			return fmt.Errorf("invalid node port")
		}
		n = n*10 + int(r-'0')
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("invalid node port")
	}
	return nil
}

func ipSANs(addr string) []net.IP {
	if ip := net.ParseIP(strings.TrimSpace(addr)); ip != nil {
		return []net.IP{ip}
	}
	return nil
}

func dnsSANs(addr string) []string {
	addr = strings.TrimSpace(addr)
	if net.ParseIP(addr) == nil && addr != "" {
		return []string{addr}
	}
	return nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
