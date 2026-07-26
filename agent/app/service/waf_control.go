package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/1Panel-dev/1Panel/agent/app/dto/response"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	wafasset "github.com/1Panel-dev/1Panel/agent/cmd/server/waf"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	composeutil "github.com/1Panel-dev/1Panel/agent/utils/compose"
	"github.com/1Panel-dev/1Panel/agent/utils/files"
	"github.com/1Panel-dev/1Panel/agent/utils/wafconfig"
	"gorm.io/gorm"
)

const (
	wafGatewayProxyPass = "http://127.0.0.1:9000"
	wafMaxBodySize      = "13m"
	// wafModeInherit is the control-plane-only sentinel for "follow the
	// panel-wide default"; the data plane only ever sees the resolved
	// detection/block value.
	wafModeInherit = "inherit"
)

var wafControlMu sync.Mutex

type wafRuntime interface {
	Up(composePath string) error
	Restart(composePath string) error
	NginxInstall() (model.AppInstall, error)
	NginxCheck(containerName string) error
	NginxReload(containerName string) error
}

type systemWafRuntime struct{}

func (systemWafRuntime) Up(composePath string) error {
	_, err := composeutil.Up(composePath)
	return err
}

func (systemWafRuntime) Restart(composePath string) error {
	_, err := composeutil.Restart(composePath)
	return err
}

func (systemWafRuntime) NginxInstall() (model.AppInstall, error) {
	return getAppInstallByKey(constant.AppOpenresty)
}

func (systemWafRuntime) NginxCheck(containerName string) error {
	return opNginx(containerName, constant.NginxCheck)
}

func (systemWafRuntime) NginxReload(containerName string) error {
	return opNginx(containerName, constant.NginxReload)
}

type IWafControlService interface {
	GetStatus(websiteID uint) (response.WafSiteStatus, error)
	Update(websiteID uint, req request.WafSiteUpdate) (response.WafSiteStatus, error)
	Remove(websiteID uint) error
	GetGlobal() (response.WafGlobalConfig, error)
	UpdateGlobal(req request.WafGlobalUpdate) (response.WafGlobalConfig, error)
}

type WafControlService struct {
	healthClient *http.Client
	runtime      wafRuntime
	readyTimeout time.Duration
}

func NewIWafControlService() IWafControlService {
	return &WafControlService{
		healthClient: &http.Client{Timeout: 2 * time.Second},
		runtime:      systemWafRuntime{},
		readyTimeout: 15 * time.Second,
	}
}

func GetWafDir() string {
	return path.Join(global.Dir.DataDir, "waf")
}

func GetWafConfigPath() string {
	return path.Join(GetWafDir(), "config", "gateway.json")
}

func GetWafComposePath() string {
	return path.Join(GetWafDir(), "docker-compose.yml")
}

func EnsureWafRuntimeFiles() error {
	if err := os.MkdirAll(path.Join(GetWafDir(), "config"), 0750); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(GetWafDir(), "audit"), 0750); err != nil {
		return err
	}
	composePath := GetWafComposePath()
	if _, err := os.Stat(composePath); errors.Is(err, os.ErrNotExist) {
		return wafconfig.WriteAtomic(composePath, wafasset.Compose, 0640)
	} else if err != nil {
		return err
	}
	return nil
}

func (s *WafControlService) GetStatus(websiteID uint) (response.WafSiteStatus, error) {
	website, err := repo.NewIWebsiteRepo().GetFirst(repo.WithByID(websiteID))
	if err != nil {
		return response.WafSiteStatus{}, err
	}
	status := response.WafSiteStatus{
		WebsiteID:     websiteID,
		Supported:     website.Type == constant.Proxy,
		Mode:          string(wafconfig.ModeDetection),
		EffectiveMode: string(wafconfig.ModeDetection),
	}
	if global.WafDB == nil {
		status.LastError = "WAF database is not available"
		return status, nil
	}
	wafRepo := repo.NewIWafRepo()
	globalPolicy, err := loadGlobalPolicy(wafRepo)
	if err != nil {
		return status, err
	}
	// A site without a stored policy follows the panel-wide default.
	status.Mode = wafModeInherit
	policy, err := wafRepo.GetPolicy(websiteID)
	if err == nil {
		status.Enabled = policy.Enabled
		status.Mode = normalizedPolicyMode(policy.Mode)
		status.AllowList = splitIPLines(policy.AllowIPs)
		status.DenyList = splitIPLines(policy.DenyIPs)
		status.LastError = policy.LastError
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return status, err
	}
	status.EffectiveMode = effectivePolicyMode(status.Mode, globalPolicy.DefaultMode)
	status.Installed = files.NewFileOp().Stat(GetWafConfigPath())
	generation := ""
	if status.Installed {
		if data, readErr := os.ReadFile(GetWafConfigPath()); readErr == nil {
			var cfg wafconfig.GatewayConfig
			if json.Unmarshal(data, &cfg) == nil {
				generation = cfg.Generation
			}
		}
	}
	status.Ready = generation != "" && s.gatewayReadyGeneration(generation)
	status.Routed = isWafRouted(rootProxyPath(website))
	status.Protected = status.Supported && status.Enabled && status.Installed && status.Ready && status.Routed
	return status, nil
}

func (s *WafControlService) Update(websiteID uint, req request.WafSiteUpdate) (response.WafSiteStatus, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	return s.update(websiteID, req)
}

func (s *WafControlService) update(websiteID uint, req request.WafSiteUpdate) (response.WafSiteStatus, error) {
	if global.WafDB == nil {
		return response.WafSiteStatus{}, errors.New("WAF database is not available")
	}
	if err := EnsureWafRuntimeFiles(); err != nil {
		return response.WafSiteStatus{}, err
	}
	websiteRepo := repo.NewIWebsiteRepo()
	website, err := websiteRepo.GetFirst(repo.WithByID(websiteID))
	if err != nil {
		return response.WafSiteStatus{}, err
	}
	if website.Type != constant.Proxy {
		return response.WafSiteStatus{}, fmt.Errorf("WAF currently supports reverse-proxy websites only")
	}
	mode := strings.TrimSpace(req.Mode)
	switch mode {
	case string(wafconfig.ModeDetection), string(wafconfig.ModeBlock), wafModeInherit:
	default:
		return response.WafSiteStatus{}, fmt.Errorf("invalid WAF mode %q", req.Mode)
	}
	allowList, err := wafconfig.NormalizeIPList(req.AllowList)
	if err != nil {
		return response.WafSiteStatus{}, fmt.Errorf("invalid WAF allow list: %w", err)
	}
	denyList, err := wafconfig.NormalizeIPList(req.DenyList)
	if err != nil {
		return response.WafSiteStatus{}, fmt.Errorf("invalid WAF deny list: %w", err)
	}

	wafRepo := repo.NewIWafRepo()
	oldPolicy, oldPolicyErr := wafRepo.GetPolicy(websiteID)
	oldConfig, configExisted := readOptional(GetWafConfigPath())
	proxyPath := rootProxyPath(website)
	oldProxy, proxyExisted := readOptional(proxyPath)

	candidate := model.WafSitePolicy{
		WebsiteID: websiteID,
		Enabled:   req.Enabled,
		Mode:      mode,
		AllowIPs:  strings.Join(allowList, "\n"),
		DenyIPs:   strings.Join(denyList, "\n"),
	}
	if err := wafRepo.SavePolicy(candidate); err != nil {
		return response.WafSiteStatus{}, err
	}
	rollback := func() error {
		var rollbackErrs []error
		if err := restoreOptional(GetWafConfigPath(), oldConfig, configExisted); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore gateway config: %w", err))
		}
		if err := restoreOptional(proxyPath, oldProxy, proxyExisted); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore nginx route: %w", err))
		}
		if oldPolicyErr == nil {
			if err := wafRepo.SavePolicy(oldPolicy); err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("restore WAF policy: %w", err))
			}
		} else if err := wafRepo.DeletePolicy(websiteID); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("remove WAF policy: %w", err))
		}
		if err := s.runtime.Restart(GetWafComposePath()); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restart previous WAF gateway: %w", err))
		}
		return errors.Join(rollbackErrs...)
	}
	fail := func(operationErr error) (response.WafSiteStatus, error) {
		if rollbackErr := rollback(); rollbackErr != nil {
			operationErr = errors.Join(operationErr, fmt.Errorf("WAF rollback incomplete: %w", rollbackErr))
		}
		_ = s.saveLastError(websiteID, operationErr)
		return response.WafSiteStatus{}, operationErr
	}

	generation, err := s.writeGatewayConfig()
	if err != nil {
		return fail(err)
	}
	if err := s.runtime.Up(GetWafComposePath()); err != nil {
		return fail(fmt.Errorf("apply WAF gateway config: %w", err))
	}
	if err := s.runtime.Restart(GetWafComposePath()); err != nil {
		return fail(fmt.Errorf("reload WAF gateway config: %w", err))
	}
	if !s.waitGatewayReady(s.readyTimeout, generation) {
		return fail(errors.New("WAF gateway did not load the requested configuration"))
	}
	if req.Enabled {
		if isWafRouted(proxyPath) {
			// The origin remains authoritative in Website.Proxy; only nginx routing
			// is already applied, so no file mutation is needed.
		} else if err := switchRootProxy(proxyPath, wafGatewayProxyPass); err != nil {
			return fail(err)
		}
	} else if proxyExisted && isWafRouted(proxyPath) {
		origin := normalizedWebsiteOrigin(website)
		if err := restoreRootProxy(proxyPath, origin); err != nil {
			return fail(err)
		}
	}

	nginxInstall, err := s.runtime.NginxInstall()
	if err != nil {
		return fail(err)
	}
	if err := s.applyNginx(proxyPath, oldProxy, proxyExisted, nginxInstall.ContainerName); err != nil {
		return fail(err)
	}
	status, err := s.GetStatus(websiteID)
	if err != nil {
		return fail(err)
	}
	if req.Enabled && !status.Protected {
		return fail(errors.New("WAF routing verification failed after nginx reload"))
	}
	return status, nil
}

func (s *WafControlService) Remove(websiteID uint) error {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return nil
	}
	wafRepo := repo.NewIWafRepo()
	if err := wafRepo.DeletePolicy(websiteID); err != nil {
		return err
	}
	if err := EnsureWafRuntimeFiles(); err != nil {
		return err
	}
	if _, err := s.writeGatewayConfig(); err != nil {
		return err
	}
	if err := s.runtime.Up(GetWafComposePath()); err != nil {
		return fmt.Errorf("apply WAF gateway config after website removal: %w", err)
	}
	if err := s.runtime.Restart(GetWafComposePath()); err != nil {
		return fmt.Errorf("reload WAF gateway after website removal: %w", err)
	}
	return nil
}

func (s *WafControlService) GetGlobal() (response.WafGlobalConfig, error) {
	if global.WafDB == nil {
		return response.WafGlobalConfig{}, errors.New("WAF database is not available")
	}
	policy, err := loadGlobalPolicy(repo.NewIWafRepo())
	if err != nil {
		return response.WafGlobalConfig{}, err
	}
	return response.WafGlobalConfig{
		DefaultMode: normalizedPolicyMode(policy.DefaultMode),
		AllowList:   splitIPLines(policy.AllowIPs),
		DenyList:    splitIPLines(policy.DenyIPs),
	}, nil
}

func (s *WafControlService) UpdateGlobal(req request.WafGlobalUpdate) (response.WafGlobalConfig, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return response.WafGlobalConfig{}, errors.New("WAF database is not available")
	}
	if err := EnsureWafRuntimeFiles(); err != nil {
		return response.WafGlobalConfig{}, err
	}
	mode := wafconfig.Mode(req.DefaultMode)
	if mode != wafconfig.ModeDetection && mode != wafconfig.ModeBlock {
		return response.WafGlobalConfig{}, fmt.Errorf("invalid WAF default mode %q", req.DefaultMode)
	}
	allowList, err := wafconfig.NormalizeIPList(req.AllowList)
	if err != nil {
		return response.WafGlobalConfig{}, fmt.Errorf("invalid WAF global allow list: %w", err)
	}
	denyList, err := wafconfig.NormalizeIPList(req.DenyList)
	if err != nil {
		return response.WafGlobalConfig{}, fmt.Errorf("invalid WAF global deny list: %w", err)
	}

	wafRepo := repo.NewIWafRepo()
	oldGlobal, oldGlobalErr := wafRepo.GetGlobalPolicy()
	if oldGlobalErr != nil && !errors.Is(oldGlobalErr, gorm.ErrRecordNotFound) {
		return response.WafGlobalConfig{}, oldGlobalErr
	}
	oldConfig, configExisted := readOptional(GetWafConfigPath())

	if err := wafRepo.SaveGlobalPolicy(model.WafGlobalPolicy{
		DefaultMode: string(mode),
		AllowIPs:    strings.Join(allowList, "\n"),
		DenyIPs:     strings.Join(denyList, "\n"),
	}); err != nil {
		return response.WafGlobalConfig{}, err
	}
	fail := func(operationErr error) (response.WafGlobalConfig, error) {
		var rollbackErrs []error
		if err := restoreOptional(GetWafConfigPath(), oldConfig, configExisted); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore gateway config: %w", err))
		}
		restored := oldGlobal
		if oldGlobalErr != nil {
			// No stored row before this update: writing the built-in defaults is
			// semantically identical to removing the row again.
			restored = model.WafGlobalPolicy{DefaultMode: string(wafconfig.ModeDetection)}
		}
		if err := wafRepo.SaveGlobalPolicy(restored); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore global WAF policy: %w", err))
		}
		if err := s.runtime.Restart(GetWafComposePath()); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restart previous WAF gateway: %w", err))
		}
		if len(rollbackErrs) > 0 {
			operationErr = errors.Join(operationErr, fmt.Errorf("WAF rollback incomplete: %w", errors.Join(rollbackErrs...)))
		}
		return response.WafGlobalConfig{}, operationErr
	}

	generation, err := s.writeGatewayConfig()
	if err != nil {
		return fail(err)
	}
	if err := s.runtime.Up(GetWafComposePath()); err != nil {
		return fail(fmt.Errorf("apply WAF gateway config: %w", err))
	}
	if err := s.runtime.Restart(GetWafComposePath()); err != nil {
		return fail(fmt.Errorf("reload WAF gateway config: %w", err))
	}
	if !s.waitGatewayReady(s.readyTimeout, generation) {
		return fail(errors.New("WAF gateway did not load the requested configuration"))
	}
	return s.GetGlobal()
}

func (s *WafControlService) saveLastError(websiteID uint, operationErr error) error {
	if operationErr == nil {
		return nil
	}
	wafRepo := repo.NewIWafRepo()
	policy, err := wafRepo.GetPolicy(websiteID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		policy = model.WafSitePolicy{
			WebsiteID: websiteID,
			Mode:      string(wafconfig.ModeDetection),
		}
	} else if err != nil {
		return err
	}
	const maxLastErrorBytes = 2048
	message := operationErr.Error()
	if len(message) > maxLastErrorBytes {
		message = message[:maxLastErrorBytes]
	}
	policy.LastError = message
	return wafRepo.SavePolicy(policy)
}

func (s *WafControlService) applyNginx(filePath string, oldContent []byte, oldExisted bool, containerName string) error {
	if err := s.runtime.NginxCheck(containerName); err != nil {
		if restoreErr := restoreOptional(filePath, oldContent, oldExisted); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore nginx config: %w", restoreErr))
		}
		return err
	}
	if err := s.runtime.NginxReload(containerName); err != nil {
		if restoreErr := restoreOptional(filePath, oldContent, true); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore nginx config: %w", restoreErr))
		}
		_ = s.runtime.NginxCheck(containerName)
		_ = s.runtime.NginxReload(containerName)
		return err
	}
	return nil
}

func (s *WafControlService) writeGatewayConfig() (string, error) {
	wafRepo := repo.NewIWafRepo()
	globalPolicy, err := loadGlobalPolicy(wafRepo)
	if err != nil {
		return "", err
	}
	globalAllow := splitIPLines(globalPolicy.AllowIPs)
	globalDeny := splitIPLines(globalPolicy.DenyIPs)
	policies, err := wafRepo.ListPolicies()
	if err != nil {
		return "", err
	}
	byID := make(map[uint]model.WafSitePolicy, len(policies))
	for _, policy := range policies {
		byID[policy.WebsiteID] = policy
	}
	websites, err := repo.NewIWebsiteRepo().List()
	if err != nil {
		return "", err
	}
	inputs := make([]wafconfig.Site, 0, len(websites))
	for _, website := range websites {
		policy, ok := byID[website.ID]
		if !ok || !policy.Enabled {
			continue
		}
		allowIPs, err := wafconfig.MergeIPLists(globalAllow, splitIPLines(policy.AllowIPs))
		if err != nil {
			return "", fmt.Errorf("website %q merged allow list: %w", website.Alias, err)
		}
		denyIPs, err := wafconfig.MergeIPLists(globalDeny, splitIPLines(policy.DenyIPs))
		if err != nil {
			return "", fmt.Errorf("website %q merged deny list: %w", website.Alias, err)
		}
		inputs = append(inputs, wafconfig.Site{
			Website:  website,
			Domains:  website.Domains,
			Enabled:  true,
			Mode:     wafconfig.Mode(effectivePolicyMode(policy.Mode, globalPolicy.DefaultMode)),
			AllowIPs: allowIPs,
			DenyIPs:  denyIPs,
		})
	}
	cfg, err := wafconfig.Build(inputs)
	if err != nil {
		return "", err
	}
	data, err := wafconfig.Marshal(cfg)
	if err != nil {
		return "", err
	}
	if err := wafconfig.WriteAtomic(GetWafConfigPath(), data, 0640); err != nil {
		return "", err
	}
	return cfg.Generation, nil
}

func (s *WafControlService) waitGatewayReady(timeout time.Duration, generation string) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.gatewayReadyGeneration(generation) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func (s *WafControlService) gatewayReadyGeneration(expected string) bool {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:9000/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := s.healthClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var health struct {
		Status     string `json:"status"`
		Generation string `json:"generation"`
	}
	if json.NewDecoder(resp.Body).Decode(&health) != nil || health.Status != "ready" {
		return false
	}
	return expected == "" || health.Generation == expected
}

func rootProxyPath(website model.Website) string {
	return path.Join(GetSitePath(website, SiteProxyDir), "root.conf")
}

func switchRootProxy(filePath, target string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	updated, _, err := wafconfig.ReplaceProxyPass(string(content), target)
	if err != nil {
		return err
	}
	if strings.TrimSuffix(target, "/") == wafGatewayProxyPass {
		updated, err = wafconfig.EnsureManagedDirective(updated, "client_max_body_size", wafMaxBodySize)
		if err != nil {
			return err
		}
	}
	return wafconfig.WriteAtomic(filePath, []byte(updated), constant.DirPerm)
}

func restoreRootProxy(filePath, target string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	updated, _, err := wafconfig.ReplaceProxyPass(string(content), target)
	if err != nil {
		return err
	}
	updated, err = wafconfig.RestoreManagedDirective(updated, "client_max_body_size")
	if err != nil {
		return err
	}
	return wafconfig.WriteAtomic(filePath, []byte(updated), constant.DirPerm)
}

func isWafRouted(filePath string) bool {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}
	_, origin, err := wafconfig.ReplaceProxyPass(string(content), wafGatewayProxyPass)
	return err == nil && strings.TrimSuffix(origin, "/") == wafGatewayProxyPass
}

func normalizedWebsiteOrigin(website model.Website) string {
	origin := strings.TrimSpace(website.Proxy)
	if !strings.Contains(origin, "://") {
		origin = "http://" + origin
	}
	return origin
}

// splitIPLines expands the newline-separated IP/CIDR text stored on a policy back
// into a trimmed slice, dropping blank lines. The stored text is already
// canonical (written from wafconfig.NormalizeIPList), so no re-validation here.
func splitIPLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizedPolicyMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return string(wafconfig.ModeDetection)
	}
	return mode
}

// effectivePolicyMode resolves the mode the gateway actually applies: an
// explicit site mode wins, "inherit" (or a legacy blank) falls back to the
// panel-wide default, and a blank default degrades to detection.
func effectivePolicyMode(siteMode, globalDefault string) string {
	siteMode = strings.TrimSpace(siteMode)
	if siteMode == "" || siteMode == wafModeInherit {
		return normalizedPolicyMode(globalDefault)
	}
	return siteMode
}

// loadGlobalPolicy returns the stored panel-wide policy, or the built-in
// defaults (detection mode, empty lists) when none has been saved yet.
func loadGlobalPolicy(wafRepo repo.IWafRepo) (model.WafGlobalPolicy, error) {
	policy, err := wafRepo.GetGlobalPolicy()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.WafGlobalPolicy{DefaultMode: string(wafconfig.ModeDetection)}, nil
	}
	if err != nil {
		return model.WafGlobalPolicy{}, err
	}
	return policy, nil
}

func readOptional(filePath string) ([]byte, bool) {
	data, err := os.ReadFile(filePath)
	return data, err == nil
}

func restoreOptional(filePath string, data []byte, existed bool) error {
	if existed {
		return wafconfig.WriteAtomic(filePath, data, constant.DirPerm)
	}
	if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
