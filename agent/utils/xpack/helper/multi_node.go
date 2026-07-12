package helper

import (
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/global"
	clamUtil "github.com/1Panel-dev/1Panel/agent/utils/clam"
	"github.com/1Panel-dev/1Panel/agent/utils/common"
	"github.com/1Panel-dev/1Panel/agent/utils/nodepki"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack/providers"
	"github.com/gin-gonic/gin"
)

type multiNodeHelper struct{}

func NewIMultiNodeProvider() providers.MultiNodeProvider {
	return &multiNodeHelper{}
}

func (m *multiNodeHelper) RemoveTamper(website string) {}

func (m *multiNodeHelper) StartClam(startClam *model.Clam, _ bool) (int, error) {
	entryID, err := clamUtil.StartSchedule(startClam)
	return int(entryID), err
}

func (m *multiNodeHelper) LoadNodeInfo(isBase bool) (model.NodeInfo, error) {
	var info model.NodeInfo
	info.BaseDir = common.LoadParams("BASE_DIR")
	info.Version = common.LoadParams("ORIGINAL_VERSION")
	// Default single-host master. A community install never leaves this branch;
	// node mode is entered ONLY after a completed enrollment provisions the DB
	// (scope=node + server/root certs). This preserves the unix-socket security
	// posture for every non-enrolled host.
	info.Scope = "master"
	global.IsMaster = true
	if global.DB == nil {
		return info, nil // pre-DB init (viper time): safe default
	}
	settingRepo := repo.NewISettingRepo()
	scope, _ := settingRepo.GetValueByKey("NodeScope")
	serverCrt, _ := settingRepo.GetValueByKey("ServerCrt")
	rootCrt, _ := settingRepo.GetValueByKey("RootCrt")
	if IsProvisionedNode(scope, serverCrt, rootCrt) {
		info.Scope = "node"
		global.IsMaster = false
		if portStr, _ := settingRepo.GetValueByKey("NodePort"); portStr != "" {
			if p, err := strconv.ParseUint(portStr, 10, 32); err == nil {
				info.NodePort = uint(p)
			}
		}
	}
	return info, nil
}

// IsProvisionedNode reports whether this agent completed node enrollment and
// should run in node mode. Node mode is strictly opt-in: it requires an explicit
// scope=node marker AND provisioned server + CA certificates.
func IsProvisionedNode(scope, serverCrt, rootCrt string) bool {
	return scope == "node" && serverCrt != "" && rootCrt != ""
}

func (m *multiNodeHelper) GetImagePrefix() string {
	return ""
}

func (m *multiNodeHelper) IsUseCustomApp() bool {
	return false
}

func (m *multiNodeHelper) IsXpack() bool {
	return false
}

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

// ValidateCertificate pins the master's client certificate. The TLS layer has
// already verified the peer cert chains to our RootCrt (RequireAndVerifyClientCert);
// this additionally requires the peer to be THE enrolled master by exact
// fingerprint (N6), so a stranger holding any CA-signed client cert is rejected.
// Fail-closed: no TLS peer, no DB, or no pinned fingerprint => reject.
func (m *multiNodeHelper) ValidateCertificate(c *gin.Context) bool {
	if c.Request == nil || c.Request.TLS == nil || len(c.Request.TLS.PeerCertificates) == 0 {
		return false
	}
	if global.DB == nil {
		return false
	}
	settingRepo := repo.NewISettingRepo()
	pinned, _ := settingRepo.GetValueByKey("MasterClientFingerprint")
	if pinned == "" {
		return false
	}
	got := nodepki.FingerprintDER(c.Request.TLS.PeerCertificates[0].Raw)
	return nodepki.FingerprintEqual(pinned, got)
}

func (m *multiNodeHelper) PushSSLToNode(websiteSSL *model.WebsiteSSL) error {
	return nil
}

func (m *multiNodeHelper) GetAgentInfo() (*dto.AgentInfo, error) {
	return nil, nil
}
