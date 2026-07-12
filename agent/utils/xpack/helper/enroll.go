package helper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/utils/nodepki"
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
}

// ApplyEnrollment persists the enrollment result so the agent boots into node
// mode on next start: it stores the signed server cert + the node's own key +
// the CA (all encrypted), the pinned master client fingerprint, the node port,
// flips NodeScope to "node", and writes the Proxy-Id file (0600). The node's
// private key is generated locally and never leaves the host (N9).
func ApplyEnrollment(resp EnrollResponse, nodeKeyPEM []byte, port uint, proxyIDFile string) error {
	if resp.ServerCert == "" || resp.CACert == "" || resp.ProxyID == "" || resp.CoreClientFingerprint == "" {
		return fmt.Errorf("incomplete enrollment response")
	}
	settingRepo := repo.NewISettingRepo()

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
	}
	for k, v := range updates {
		if err := settingRepo.UpdateOrCreate(k, v); err != nil {
			return err
		}
	}

	if proxyIDFile == "" {
		proxyIDFile = DefaultProxyIDPath
	}
	if err := os.WriteFile(proxyIDFile, []byte(resp.ProxyID), 0o600); err != nil {
		return err
	}

	// Flip to node mode LAST, only after every credential is safely stored, so a
	// partial failure never leaves a half-provisioned node in node mode.
	if err := settingRepo.UpdateOrCreate("NodeScope", "node"); err != nil {
		return err
	}
	return nil
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
