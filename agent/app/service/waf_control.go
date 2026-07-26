package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
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
	// readyTimeout bounds the wait after a container start/restart.
	readyTimeout time.Duration
	// reloadTimeout bounds the wait for an already-running gateway to pick the
	// new config file up in-process. It only has to cover the gateway's watch
	// interval plus one ruleset compile, so it is much shorter than a restart.
	reloadTimeout time.Duration
}

func NewIWafControlService() IWafControlService {
	return &WafControlService{
		healthClient:  &http.Client{Timeout: 2 * time.Second},
		runtime:       systemWafRuntime{},
		readyTimeout:  15 * time.Second,
		reloadTimeout: 10 * time.Second,
	}
}

func GetWafDir() string {
	return path.Join(global.Dir.DataDir, "waf")
}

func GetWafConfigPath() string {
	return path.Join(GetWafDir(), "config", "gateway.json")
}

// GetWafAdminTokenPath is the shared secret the control plane and the gateway
// use for the loopback management API. It lives in the config directory that is
// already mounted read-only into the container, so no new mount is needed.
func GetWafAdminTokenPath() string {
	return path.Join(GetWafDir(), "config", "admin.token")
}

// ensureWafAdminToken creates the management secret once and returns it. The
// gateway disables its management API when the file is absent, so a failure
// here degrades to "no remote management" rather than breaking traffic.
func ensureWafAdminToken() (string, error) {
	tokenPath := GetWafAdminTokenPath()
	if data, err := os.ReadFile(tokenPath); err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf[:])
	// 0600: only the panel process and root may read it. The container reads it
	// through the same bind mount as the rest of the config.
	if err := wafconfig.WriteAtomic(tokenPath, []byte(token+"\n"), 0600); err != nil {
		return "", err
	}
	return token, nil
}

func GetWafComposePath() string {
	return path.Join(GetWafDir(), "docker-compose.yml")
}

// composeVersionMarker prefixes the version comment carried by the embedded
// compose asset. Deployed files written before the marker existed parse as
// version 0 and are therefore treated as outdated.
const composeVersionMarker = "# 1panel-x-waf-compose-version:"

// EnsureWafRuntimeFiles creates the runtime directories and keeps the deployed
// compose file up to date with the embedded asset. It reports whether the
// compose file was (re)written, because a rewritten compose only takes effect
// once the container is recreated — the caller must then take the restart path
// rather than the in-process reload path.
func EnsureWafRuntimeFiles() (bool, error) {
	if err := os.MkdirAll(path.Join(GetWafDir(), "config"), 0750); err != nil {
		return false, err
	}
	if err := os.MkdirAll(path.Join(GetWafDir(), "audit"), 0750); err != nil {
		return false, err
	}
	if _, err := ensureWafAdminToken(); err != nil {
		return false, err
	}
	composePath := GetWafComposePath()
	current, err := os.ReadFile(composePath)
	if errors.Is(err, os.ErrNotExist) {
		return true, wafconfig.WriteAtomic(composePath, wafasset.Compose, 0640)
	}
	if err != nil {
		return false, err
	}
	// Only move forward. An operator-edited file with an equal or newer marker is
	// left alone; overwriting unconditionally would silently discard local tuning.
	if composeAssetVersion(current) >= composeAssetVersion(wafasset.Compose) {
		return false, nil
	}
	return true, wafconfig.WriteAtomic(composePath, wafasset.Compose, 0640)
}

// composeAssetVersion reads the embedded version marker, returning 0 when the
// file predates the marker or carries an unparsable one.
func composeAssetVersion(data []byte) int {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, composeVersionMarker) {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, composeVersionMarker)))
		if err != nil {
			return 0
		}
		return v
	}
	return 0
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
	var siteLimits []wafconfig.RateLimit
	policy, err := wafRepo.GetPolicy(websiteID)
	if err == nil {
		status.Enabled = policy.Enabled
		status.Mode = normalizedPolicyMode(policy.Mode)
		status.AllowList = splitIPLines(policy.AllowIPs)
		status.DenyList = splitIPLines(policy.DenyIPs)
		status.LastError = policy.LastError
		if siteLimits, err = parseRateLimits(policy.RateLimits); err != nil {
			// Report the unreadable row instead of quietly showing no limits.
			status.LastError = strings.TrimSpace(status.LastError + " " + err.Error())
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return status, err
	}
	status.EffectiveMode = effectivePolicyMode(status.Mode, globalPolicy.DefaultMode)

	globalLimits, err := parseRateLimits(globalPolicy.RateLimits)
	if err != nil {
		status.LastError = strings.TrimSpace(status.LastError + " " + err.Error())
	}
	status.RateLimits = rateLimitsToResponse(siteLimits)
	// The effective set is what the gateway actually enforces, so the UI can
	// report the running policy rather than the site's overrides alone.
	effective, _ := splitAttackLimit(mergeRateLimits(globalLimits, siteLimits))
	status.EffectiveRateLimits = rateLimitsToResponse(effective)

	siteRules, err := parseRulePolicy(policy.RuleOptions)
	if err != nil {
		status.LastError = strings.TrimSpace(status.LastError + " " + err.Error())
	}
	globalRules, err := parseRulePolicy(globalPolicy.RuleOptions)
	if err != nil {
		status.LastError = strings.TrimSpace(status.LastError + " " + err.Error())
	}
	status.Rules = rulePolicyToResponse(siteRules)
	status.EffectiveRules = rulePolicyValue(wafconfig.MergeRulePolicy(globalRules, siteRules))

	siteRegion, err := parseRegionPolicy(policy.RegionRules)
	if err != nil {
		status.LastError = strings.TrimSpace(status.LastError + " " + err.Error())
	}
	globalRegion, err := parseRegionPolicy(globalPolicy.RegionRules)
	if err != nil {
		status.LastError = strings.TrimSpace(status.LastError + " " + err.Error())
	}
	status.Region = regionPolicyToResponse(siteRegion)
	status.EffectiveRegion = regionPolicyValue(wafconfig.MergeRegionPolicy(globalRegion, siteRegion))
	status.GeoAvailable = wafGeoAvailable()
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
	composeChanged, err := EnsureWafRuntimeFiles()
	if err != nil {
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
	siteLimits, err := wafconfig.NormalizeRateLimits(rateLimitsFromRequest(req.RateLimits))
	if err != nil {
		return response.WafSiteStatus{}, fmt.Errorf("invalid WAF rate limits: %w", err)
	}
	// The attack limit is enforced gateway-wide; accepting it here would store a
	// per-site threshold the data plane can never apply.
	if _, attack := splitAttackLimit(siteLimits); attack != nil {
		return response.WafSiteStatus{}, errors.New("the attack frequency limit is applied gateway-wide and can only be set in the global WAF settings")
	}
	siteLimitsJSON, err := marshalRateLimits(siteLimits)
	if err != nil {
		return response.WafSiteStatus{}, err
	}
	var siteRules *wafconfig.RulePolicy
	if raw := rulePolicyFromRequest(req.Rules); raw != nil {
		normalized, err := wafconfig.NormalizeRulePolicy(*raw)
		if err != nil {
			return response.WafSiteStatus{}, fmt.Errorf("invalid WAF rule options: %w", err)
		}
		siteRules = &normalized
	}
	siteRulesJSON, err := marshalRulePolicy(siteRules)
	if err != nil {
		return response.WafSiteStatus{}, err
	}
	siteRegion, err := normalizeRegionRequest(req.Region)
	if err != nil {
		return response.WafSiteStatus{}, err
	}
	siteRegionJSON, err := marshalRegionPolicy(siteRegion)
	if err != nil {
		return response.WafSiteStatus{}, err
	}

	wafRepo := repo.NewIWafRepo()
	oldPolicy, oldPolicyErr := wafRepo.GetPolicy(websiteID)
	oldConfig, configExisted := readOptional(GetWafConfigPath())
	proxyPath := rootProxyPath(website)
	oldProxy, proxyExisted := readOptional(proxyPath)

	candidate := model.WafSitePolicy{
		WebsiteID:   websiteID,
		Enabled:     req.Enabled,
		Mode:        mode,
		AllowIPs:    strings.Join(allowList, "\n"),
		DenyIPs:     strings.Join(denyList, "\n"),
		RateLimits:  siteLimitsJSON,
		RuleOptions: siteRulesJSON,
		RegionRules: siteRegionJSON,
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
	if err := s.applyGatewayConfig(generation, composeChanged); err != nil {
		return fail(err)
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
	composeChanged, err := EnsureWafRuntimeFiles()
	if err != nil {
		return err
	}
	generation, err := s.writeGatewayConfig()
	if err != nil {
		return err
	}
	if err := s.applyGatewayConfig(generation, composeChanged); err != nil {
		return fmt.Errorf("apply WAF gateway config after website removal: %w", err)
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
	limits, err := parseRateLimits(policy.RateLimits)
	if err != nil {
		return response.WafGlobalConfig{}, err
	}
	rules, err := parseRulePolicy(policy.RuleOptions)
	if err != nil {
		return response.WafGlobalConfig{}, err
	}
	region, err := parseRegionPolicy(policy.RegionRules)
	if err != nil {
		return response.WafGlobalConfig{}, err
	}
	return response.WafGlobalConfig{
		DefaultMode:  normalizedPolicyMode(policy.DefaultMode),
		AllowList:    splitIPLines(policy.AllowIPs),
		DenyList:     splitIPLines(policy.DenyIPs),
		RateLimits:   rateLimitsToResponse(limits),
		Rules:        rulePolicyValue(rules),
		Region:       regionPolicyValue(region),
		GeoAvailable: wafGeoAvailable(),
	}, nil
}

func (s *WafControlService) UpdateGlobal(req request.WafGlobalUpdate) (response.WafGlobalConfig, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return response.WafGlobalConfig{}, errors.New("WAF database is not available")
	}
	composeChanged, err := EnsureWafRuntimeFiles()
	if err != nil {
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
	globalLimits, err := wafconfig.NormalizeRateLimits(rateLimitsFromRequest(req.RateLimits))
	if err != nil {
		return response.WafGlobalConfig{}, fmt.Errorf("invalid WAF global rate limits: %w", err)
	}
	globalLimitsJSON, err := marshalRateLimits(globalLimits)
	if err != nil {
		return response.WafGlobalConfig{}, err
	}
	var globalRules *wafconfig.RulePolicy
	if raw := rulePolicyFromRequest(req.Rules); raw != nil {
		normalized, err := wafconfig.NormalizeRulePolicy(*raw)
		if err != nil {
			return response.WafGlobalConfig{}, fmt.Errorf("invalid WAF rule options: %w", err)
		}
		globalRules = &normalized
	}
	globalRulesJSON, err := marshalRulePolicy(globalRules)
	if err != nil {
		return response.WafGlobalConfig{}, err
	}
	globalRegion, err := normalizeRegionRequest(req.Region)
	if err != nil {
		return response.WafGlobalConfig{}, err
	}
	globalRegionJSON, err := marshalRegionPolicy(globalRegion)
	if err != nil {
		return response.WafGlobalConfig{}, err
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
		RateLimits:  globalLimitsJSON,
		RuleOptions: globalRulesJSON,
		RegionRules: globalRegionJSON,
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
	if err := s.applyGatewayConfig(generation, composeChanged); err != nil {
		return fail(err)
	}
	return s.GetGlobal()
}

// applyGatewayConfig makes the freshly written config the one the gateway is
// actually enforcing, confirmed by the generation it echoes on /healthz.
//
// A running gateway watches its config file and swaps the routing table
// in-process, so the normal path is just "wait for the generation to appear".
// Restarting the container is reserved for the cases that genuinely need it —
// it is not free: the gateway holds enforcement state in memory (rate-limit
// counters and temporary IP bans), and restarting on every unrelated policy save
// would silently erase it.
func (s *WafControlService) applyGatewayConfig(generation string, composeChanged bool) error {
	if !composeChanged {
		if health, ok := s.fetchGatewayHealth(); ok && health.Status == "ready" {
			if s.waitGatewayReady(s.reloadTimeout, generation) {
				return nil
			}
			// Still running but not on the requested generation: if it refused the
			// candidate, say why instead of reporting a bare timeout.
			if refreshed, ok := s.fetchGatewayHealth(); ok && refreshed.LastError != "" {
				return fmt.Errorf("WAF gateway refused the configuration: %s", refreshed.LastError)
			}
		}
	}
	if err := s.runtime.Up(GetWafComposePath()); err != nil {
		return fmt.Errorf("apply WAF gateway config: %w", err)
	}
	if err := s.runtime.Restart(GetWafComposePath()); err != nil {
		return fmt.Errorf("reload WAF gateway config: %w", err)
	}
	if !s.waitGatewayReady(s.readyTimeout, generation) {
		if health, ok := s.fetchGatewayHealth(); ok && health.LastError != "" {
			return fmt.Errorf("WAF gateway refused the configuration: %s", health.LastError)
		}
		return errors.New("WAF gateway did not load the requested configuration")
	}
	return nil
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
	globalLimits, err := parseRateLimits(globalPolicy.RateLimits)
	if err != nil {
		return "", err
	}
	// The attack limit is gateway-wide; it is emitted once at the top level, not
	// copied into every site (the gateway refuses a per-site copy outright).
	globalSiteLimits, attackLimit := splitAttackLimit(globalLimits)
	globalRules, err := parseRulePolicy(globalPolicy.RuleOptions)
	if err != nil {
		return "", err
	}
	globalRegion, err := parseRegionPolicy(globalPolicy.RegionRules)
	if err != nil {
		return "", err
	}
	listEntries, err := wafRepo.ListEntries()
	if err != nil {
		return "", err
	}
	ipGroups, err := wafRepo.ListIPGroups()
	if err != nil {
		return "", err
	}
	customRows, err := wafRepo.ListCustomRules()
	if err != nil {
		return "", err
	}
	customRules, err := enabledCustomRules(customRows)
	if err != nil {
		return "", err
	}
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
		siteLimits, err := parseRateLimits(policy.RateLimits)
		if err != nil {
			return "", fmt.Errorf("website %q: %w", website.Alias, err)
		}
		siteRules, err := parseRulePolicy(policy.RuleOptions)
		if err != nil {
			return "", fmt.Errorf("website %q: %w", website.Alias, err)
		}
		siteRegion, err := parseRegionPolicy(policy.RegionRules)
		if err != nil {
			return "", fmt.Errorf("website %q: %w", website.Alias, err)
		}
		mergedLimits, _ := splitAttackLimit(mergeRateLimits(globalSiteLimits, siteLimits))
		inputs = append(inputs, wafconfig.Site{
			Website:    website,
			Domains:    website.Domains,
			Enabled:    true,
			Mode:       wafconfig.Mode(effectivePolicyMode(policy.Mode, globalPolicy.DefaultMode)),
			AllowIPs:   allowIPs,
			DenyIPs:    denyIPs,
			RateLimits: mergedLimits,
			Rules:      wafconfig.MergeRulePolicy(globalRules, siteRules),
			Region:     wafconfig.MergeRegionPolicy(globalRegion, siteRegion),
		})
	}
	cfg, err := wafconfig.BuildWithOptions(inputs, wafconfig.BuildOptions{
		AttackRateLimit: attackLimit,
		Lists:           enabledListRules(listEntries),
		IPGroups:        ipGroupsToConfig(ipGroups),
		CustomRules:     customRules,
	})
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

// gatewayHealth mirrors the gateway's /healthz payload. Generation always
// describes the config the gateway is RUNNING, and LastError is set when a
// candidate config on disk was refused — so "not applied yet" and "refused" are
// distinguishable instead of both surfacing as a readiness timeout.
type gatewayHealth struct {
	Status     string `json:"status"`
	Generation string `json:"generation"`
	LastError  string `json:"lastError"`
}

func (s *WafControlService) fetchGatewayHealth() (gatewayHealth, bool) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:9000/healthz", nil)
	if err != nil {
		return gatewayHealth{}, false
	}
	resp, err := s.healthClient.Do(req)
	if err != nil {
		return gatewayHealth{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return gatewayHealth{}, false
	}
	var health gatewayHealth
	if json.NewDecoder(resp.Body).Decode(&health) != nil {
		return gatewayHealth{}, false
	}
	return health, true
}

func (s *WafControlService) gatewayReadyGeneration(expected string) bool {
	health, ok := s.fetchGatewayHealth()
	if !ok || health.Status != "ready" {
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

// --- rate-limit storage helpers -------------------------------------------
//
// Limits are stored as a JSON array per policy row. They are validated by
// wafconfig before being written, so a parse failure here means the row was
// corrupted or hand-edited; it is surfaced rather than silently dropped, because
// silently dropping a limit would leave the UI showing protection that is not
// being enforced.

func parseRateLimits(raw string) ([]wafconfig.RateLimit, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out []wafconfig.RateLimit
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("stored WAF rate limits are unreadable: %w", err)
	}
	return out, nil
}

func marshalRateLimits(limits []wafconfig.RateLimit) (string, error) {
	if len(limits) == 0 {
		return "", nil
	}
	data, err := json.Marshal(limits)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// mergeRateLimits overlays a site's limits on the panel-wide defaults: a site
// entry REPLACES the global entry of the same kind, and global kinds the site
// does not mention still apply. Replacing rather than combining matters — two
// limits of one kind would share a counter and enforce whichever threshold was
// checked first.
func mergeRateLimits(global, site []wafconfig.RateLimit) []wafconfig.RateLimit {
	byKind := make(map[string]wafconfig.RateLimit, len(global)+len(site))
	order := make([]string, 0, len(global)+len(site))
	remember := func(l wafconfig.RateLimit) {
		if _, seen := byKind[l.Kind]; !seen {
			order = append(order, l.Kind)
		}
		byKind[l.Kind] = l
	}
	for _, l := range global {
		remember(l)
	}
	for _, l := range site {
		remember(l)
	}
	out := make([]wafconfig.RateLimit, 0, len(order))
	for _, kind := range order {
		out = append(out, byKind[kind])
	}
	return out
}

// splitAttackLimit separates the gateway-wide attack limit from the per-site
// ones. The gateway refuses a per-site attack limit outright, so this keeps an
// operator's global attack setting from being emitted into every site.
func splitAttackLimit(limits []wafconfig.RateLimit) (perSite []wafconfig.RateLimit, attack *wafconfig.RateLimit) {
	for _, l := range limits {
		if l.Kind == wafconfig.RateLimitAttack {
			entry := l
			attack = &entry
			continue
		}
		perSite = append(perSite, l)
	}
	return perSite, attack
}

func rateLimitsFromRequest(in []request.WafRateLimit) []wafconfig.RateLimit {
	if len(in) == 0 {
		return nil
	}
	out := make([]wafconfig.RateLimit, 0, len(in))
	for _, l := range in {
		out = append(out, wafconfig.RateLimit{
			Kind:      strings.TrimSpace(l.Kind),
			PeriodSec: l.PeriodSec,
			Threshold: l.Threshold,
			BanSec:    l.BanSec,
			PerURL:    l.PerURL,
		})
	}
	return out
}

// --- region-policy storage helpers ------------------------------------------

// GetWafGeoDBPath is the address database region access control reads. It is the
// panel's own database, mounted read-only into the gateway container, so region
// control needs no separate download.
func GetWafGeoDBPath() string {
	return path.Join(global.Dir.DataDir, "geo", "GeoIP.mmdb")
}

// wafGeoAvailable reports whether region control can actually be enforced. It is
// surfaced to the UI so the control can be shown as unavailable rather than as a
// switch that silently does nothing.
func wafGeoAvailable() bool {
	info, err := os.Stat(GetWafGeoDBPath())
	return err == nil && !info.IsDir() && info.Size() > 0
}

func parseRegionPolicy(raw string) (*wafconfig.RegionPolicy, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var p wafconfig.RegionPolicy
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("stored WAF region policy is unreadable: %w", err)
	}
	return &p, nil
}

func marshalRegionPolicy(p *wafconfig.RegionPolicy) (string, error) {
	if p == nil {
		return "", nil
	}
	data, err := json.Marshal(*p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func regionPolicyFromRequest(in *request.WafRegionPolicy) *wafconfig.RegionPolicy {
	if in == nil {
		return nil
	}
	return &wafconfig.RegionPolicy{Mode: in.Mode, Regions: in.Regions}
}

func regionPolicyToResponse(p *wafconfig.RegionPolicy) *response.WafRegionPolicy {
	if p == nil {
		return nil
	}
	regions := p.Regions
	if regions == nil {
		regions = []string{}
	}
	return &response.WafRegionPolicy{Mode: p.Mode, Regions: regions}
}

func regionPolicyValue(p *wafconfig.RegionPolicy) response.WafRegionPolicy {
	if converted := regionPolicyToResponse(p); converted != nil {
		return *converted
	}
	return response.WafRegionPolicy{Regions: []string{}}
}

// normalizeRegionRequest validates an incoming region policy and refuses one the
// data plane could not enforce. Saying so here, naming the missing database, is
// far better than letting the gateway refuse the whole config later.
func normalizeRegionRequest(in *request.WafRegionPolicy) (*wafconfig.RegionPolicy, error) {
	raw := regionPolicyFromRequest(in)
	if raw == nil {
		return nil, nil
	}
	normalized, err := wafconfig.NormalizeRegionPolicy(*raw)
	if err != nil {
		return nil, fmt.Errorf("invalid WAF region policy: %w", err)
	}
	if !normalized.IsZero() && !wafGeoAvailable() {
		return nil, fmt.Errorf("region access control needs the IP address database at %s, which is not installed", GetWafGeoDBPath())
	}
	return &normalized, nil
}

// --- detection-policy storage helpers --------------------------------------

func parseRulePolicy(raw string) (*wafconfig.RulePolicy, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var p wafconfig.RulePolicy
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("stored WAF rule options are unreadable: %w", err)
	}
	return &p, nil
}

func marshalRulePolicy(p *wafconfig.RulePolicy) (string, error) {
	if p == nil {
		return "", nil
	}
	data, err := json.Marshal(*p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func rulePolicyFromRequest(in *request.WafRulePolicy) *wafconfig.RulePolicy {
	if in == nil {
		return nil
	}
	return &wafconfig.RulePolicy{
		DisableSQLi:      in.DisableSQLi,
		DisableXSS:       in.DisableXSS,
		Strict:           in.Strict,
		AllowedMethods:   in.AllowedMethods,
		BannedUploadExts: in.BannedUploadExts,
	}
}

func rulePolicyToResponse(p *wafconfig.RulePolicy) *response.WafRulePolicy {
	if p == nil {
		return nil
	}
	methods := p.AllowedMethods
	if methods == nil {
		methods = []string{}
	}
	exts := p.BannedUploadExts
	if exts == nil {
		exts = []string{}
	}
	return &response.WafRulePolicy{
		DisableSQLi:      p.DisableSQLi,
		DisableXSS:       p.DisableXSS,
		Strict:           p.Strict,
		AllowedMethods:   methods,
		BannedUploadExts: exts,
	}
}

func rulePolicyValue(p *wafconfig.RulePolicy) response.WafRulePolicy {
	if converted := rulePolicyToResponse(p); converted != nil {
		return *converted
	}
	return response.WafRulePolicy{AllowedMethods: []string{}, BannedUploadExts: []string{}}
}

func rateLimitsToResponse(in []wafconfig.RateLimit) []response.WafRateLimit {
	out := make([]response.WafRateLimit, 0, len(in))
	for _, l := range in {
		out = append(out, response.WafRateLimit{
			Kind:      l.Kind,
			PeriodSec: l.PeriodSec,
			Threshold: l.Threshold,
			BanSec:    l.BanSec,
			PerURL:    l.PerURL,
		})
	}
	return out
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
