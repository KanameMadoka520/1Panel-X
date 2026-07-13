package helper

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/init/proxy"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/nodepki"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
	"github.com/gin-gonic/gin"
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

func (m *multiNodeHelper) Sync(dataType string) error { return nil }

func (m *multiNodeHelper) AutoUpgradeWithMaster() {}
