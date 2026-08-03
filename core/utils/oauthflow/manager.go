package oauthflow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultFlowTTL          = 10 * time.Minute
	defaultHTTPTimeout      = 15 * time.Second
	defaultMaxResponseBytes = int64(1 << 20)
	randomTokenBytes        = 32
)

type Provider string

const (
	ProviderOneDrive    Provider = "OneDrive"
	ProviderGoogleDrive Provider = "GoogleDrive"
)

var (
	ErrInvalidInput        = errors.New("invalid OAuth flow input")
	ErrUnsupportedProvider = errors.New("unsupported OAuth provider")
	ErrRandomSource        = errors.New("OAuth flow random generation failed")
	ErrInvalidState        = errors.New("invalid or already used OAuth state")
	ErrFlowExpired         = errors.New("OAuth flow expired")
	ErrInvalidCallback     = errors.New("invalid OAuth callback")
	ErrProviderDenied      = errors.New("OAuth authorization was denied")
	ErrTokenExchange       = errors.New("OAuth token exchange failed")
	ErrDriveValidation     = errors.New("OAuth drive validation failed")
	ErrFlowNotFound        = errors.New("OAuth flow not found")
	ErrFlowNotReady        = errors.New("OAuth flow is not ready")
	errRedirectRefused     = errors.New("OAuth HTTP redirect refused")
)

type BeginInput struct {
	Provider        Provider
	ClientID        string
	ClientSecret    string
	RedirectURI     string
	IsCN            bool
	AccountIdentity string
}

// BeginResult is safe to serialize to an API response.
type BeginResult struct {
	AuthorizationURL string    `json:"authorization_url"`
	FlowID           string    `json:"flow_id"`
	ExpiresAt        time.Time `json:"expires_at"`
	MaskedClientID   string    `json:"masked_client_id"`
}

// CompleteResult deliberately carries no credential or token material.
type CompleteResult struct {
	FlowID    string    `json:"flow_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StoredResult is available only to server-side callers. Sensitive fields are
// excluded from JSON so an accidental response serialization cannot expose them.
type StoredResult struct {
	FlowID          string    `json:"flow_id"`
	Provider        Provider  `json:"provider"`
	IsCN            bool      `json:"is_cn"`
	AccountIdentity string    `json:"account_identity"`
	ExpiresAt       time.Time `json:"expires_at"`
	ClientID        string    `json:"-"`
	ClientSecret    string    `json:"-"`
	RedirectURI     string    `json:"-"`
	RefreshToken    string    `json:"-"`
}

type EndpointSet struct {
	AuthorizationURL string
	TokenURL         string
	DriveURL         string
}

type Endpoints struct {
	OneDrive      EndpointSet
	OneDriveChina EndpointSet
	GoogleDrive   EndpointSet
}

type Options struct {
	HTTPClient       *http.Client
	Endpoints        Endpoints
	Now              func() time.Time
	Random           io.Reader
	FlowTTL          time.Duration
	MaxResponseBytes int64
}

type flowStatus uint8

const (
	flowPending flowStatus = iota
	flowCompleting
	flowReady
)

type flow struct {
	id              string
	state           string
	provider        Provider
	isCN            bool
	accountIdentity string
	clientID        string
	clientSecret    string
	redirectURI     string
	pkceVerifier    string
	expiresAt       time.Time
	status          flowStatus
	refreshToken    string
	expiryTimer     *time.Timer
}

type Manager struct {
	mu               sync.Mutex
	flows            map[string]*flow
	stateToFlow      map[string]string
	httpClient       *http.Client
	endpoints        Endpoints
	now              func() time.Time
	random           io.Reader
	flowTTL          time.Duration
	maxResponseBytes int64
}

func DefaultEndpoints() Endpoints {
	return Endpoints{
		OneDrive: EndpointSet{
			AuthorizationURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL:         "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			DriveURL:         "https://graph.microsoft.com/v1.0/me/drive?$select=id",
		},
		OneDriveChina: EndpointSet{
			AuthorizationURL: "https://login.chinacloudapi.cn/common/oauth2/v2.0/authorize",
			TokenURL:         "https://login.chinacloudapi.cn/common/oauth2/v2.0/token",
			DriveURL:         "https://microsoftgraph.chinacloudapi.cn/v1.0/me/drive?$select=id",
		},
		GoogleDrive: EndpointSet{
			AuthorizationURL: "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:         "https://oauth2.googleapis.com/token",
			DriveURL:         "https://www.googleapis.com/drive/v3/about?fields=user",
		},
	}
}

func NewManager(options Options) *Manager {
	client := &http.Client{}
	if options.HTTPClient != nil {
		clientCopy := *options.HTTPClient
		client = &clientCopy
	}
	if client.Timeout <= 0 {
		client.Timeout = defaultHTTPTimeout
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errRedirectRefused
	}

	flowTTL := options.FlowTTL
	if flowTTL <= 0 {
		flowTTL = DefaultFlowTTL
	}
	maxResponseBytes := options.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	randomSource := options.Random
	if randomSource == nil {
		randomSource = rand.Reader
	}

	return &Manager{
		flows:            make(map[string]*flow),
		stateToFlow:      make(map[string]string),
		httpClient:       client,
		endpoints:        mergeEndpoints(options.Endpoints, DefaultEndpoints()),
		now:              now,
		random:           randomSource,
		flowTTL:          flowTTL,
		maxResponseBytes: maxResponseBytes,
	}
}

func (m *Manager) Begin(input BeginInput) (BeginResult, error) {
	provider, err := normalizeProvider(input.Provider)
	if err != nil {
		return BeginResult{}, err
	}
	clientID := strings.TrimSpace(input.ClientID)
	redirectURI := strings.TrimSpace(input.RedirectURI)
	accountIdentity := strings.TrimSpace(input.AccountIdentity)
	if clientID == "" || strings.TrimSpace(input.ClientSecret) == "" || accountIdentity == "" {
		return BeginResult{}, ErrInvalidInput
	}
	if provider == ProviderGoogleDrive && input.IsCN {
		return BeginResult{}, ErrInvalidInput
	}
	if err := validateRedirectURI(redirectURI); err != nil {
		return BeginResult{}, err
	}
	endpointSet, err := m.endpointSet(provider, input.IsCN)
	if err != nil {
		return BeginResult{}, err
	}

	for attempts := 0; attempts < 8; attempts++ {
		flowID, err := m.randomToken()
		if err != nil {
			return BeginResult{}, err
		}
		state, err := m.randomToken()
		if err != nil {
			return BeginResult{}, err
		}
		verifier, err := m.randomToken()
		if err != nil {
			return BeginResult{}, err
		}
		challengeSum := sha256.Sum256([]byte(verifier))
		challenge := base64.RawURLEncoding.EncodeToString(challengeSum[:])
		authorizationURL, err := buildAuthorizationURL(endpointSet.AuthorizationURL, provider, clientID, redirectURI, state, challenge)
		if err != nil {
			return BeginResult{}, err
		}

		now := m.now()
		item := &flow{
			id:              flowID,
			state:           state,
			provider:        provider,
			isCN:            input.IsCN,
			accountIdentity: accountIdentity,
			clientID:        clientID,
			clientSecret:    input.ClientSecret,
			redirectURI:     redirectURI,
			pkceVerifier:    verifier,
			expiresAt:       now.Add(m.flowTTL),
			status:          flowPending,
		}

		m.mu.Lock()
		m.cleanupExpiredLocked(now)
		_, flowCollision := m.flows[flowID]
		_, stateCollision := m.stateToFlow[state]
		if !flowCollision && !stateCollision {
			m.flows[flowID] = item
			m.stateToFlow[state] = flowID
			item.expiryTimer = time.AfterFunc(m.flowTTL, func() {
				m.Delete(flowID)
			})
			m.mu.Unlock()
			return BeginResult{
				AuthorizationURL: authorizationURL,
				FlowID:           flowID,
				ExpiresAt:        item.expiresAt,
				MaskedClientID:   MaskClientID(clientID),
			}, nil
		}
		m.mu.Unlock()
	}

	return BeginResult{}, ErrRandomSource
}

func (m *Manager) Complete(callbackURL string) (CompleteResult, error) {
	callback, err := parseCallbackURL(callbackURL)
	if err != nil {
		return CompleteResult{}, err
	}
	claimed, err := m.claimFlow(callback.state, callback.url)
	if err != nil {
		return CompleteResult{}, err
	}
	fail := func(flowErr error) (CompleteResult, error) {
		m.Delete(claimed.id)
		return CompleteResult{}, flowErr
	}
	if callback.providerDenied {
		return fail(ErrProviderDenied)
	}
	if callback.code == "" || callback.invalidCode {
		return fail(ErrInvalidCallback)
	}

	refreshToken, accessToken, err := m.exchangeCode(claimed, callback.code)
	if err != nil {
		return fail(err)
	}
	if err := m.validateDrive(claimed, accessToken); err != nil {
		return fail(err)
	}

	now := m.now()
	m.mu.Lock()
	current, ok := m.flows[claimed.id]
	if !ok || current.status != flowCompleting {
		m.mu.Unlock()
		return CompleteResult{}, ErrFlowNotFound
	}
	if !now.Before(current.expiresAt) {
		m.deleteFlowLocked(current.id)
		m.mu.Unlock()
		return CompleteResult{}, ErrFlowExpired
	}
	current.refreshToken = refreshToken
	current.pkceVerifier = ""
	current.state = ""
	current.status = flowReady
	expiresAt := current.expiresAt
	m.mu.Unlock()

	return CompleteResult{FlowID: claimed.id, Status: "ready", ExpiresAt: expiresAt}, nil
}

func (m *Manager) Peek(flowID string) (StoredResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, err := m.readyFlowLocked(strings.TrimSpace(flowID), m.now())
	if err != nil {
		return StoredResult{}, err
	}
	return storedResult(item), nil
}

func (m *Manager) Consume(flowID string) (StoredResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, err := m.readyFlowLocked(strings.TrimSpace(flowID), m.now())
	if err != nil {
		return StoredResult{}, err
	}
	result := storedResult(item)
	m.deleteFlowLocked(item.id)
	return result, nil
}

func (m *Manager) Delete(flowID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteFlowLocked(strings.TrimSpace(flowID))
}

func MaskClientID(clientID string) string {
	clientID = strings.TrimSpace(clientID)
	if len(clientID) <= 8 {
		return "********"
	}
	return clientID[:4] + "..." + clientID[len(clientID)-4:]
}

type callbackData struct {
	url            *url.URL
	state          string
	code           string
	invalidCode    bool
	providerDenied bool
}

func parseCallbackURL(raw string) (callbackData, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Fragment != "" || parsed.User != nil {
		return callbackData{}, ErrInvalidCallback
	}
	query := parsed.Query()
	states := query["state"]
	if len(states) != 1 || strings.TrimSpace(states[0]) == "" {
		return callbackData{}, ErrInvalidState
	}
	codes := query["code"]
	data := callbackData{
		url:            parsed,
		state:          states[0],
		providerDenied: strings.TrimSpace(query.Get("error")) != "",
	}
	if len(codes) != 1 || strings.TrimSpace(codes[0]) == "" {
		data.invalidCode = true
	} else {
		data.code = codes[0]
	}
	return data, nil
}

func (m *Manager) claimFlow(state string, callbackURL *url.URL) (*flow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	flowID, ok := m.stateToFlow[state]
	if !ok {
		return nil, ErrInvalidState
	}
	item, ok := m.flows[flowID]
	if !ok || item.status != flowPending || item.state != state {
		delete(m.stateToFlow, state)
		return nil, ErrInvalidState
	}
	if !m.now().Before(item.expiresAt) {
		m.deleteFlowLocked(item.id)
		return nil, ErrFlowExpired
	}
	if !callbackMatchesRedirect(callbackURL, item.redirectURI) {
		m.deleteFlowLocked(item.id)
		return nil, ErrInvalidCallback
	}
	delete(m.stateToFlow, state)
	item.status = flowCompleting
	copyItem := *item
	return &copyItem, nil
}

func (m *Manager) exchangeCode(item *flow, code string) (string, string, error) {
	endpointSet, err := m.endpointSet(item.provider, item.isCN)
	if err != nil {
		return "", "", err
	}
	form := url.Values{}
	form.Set("client_id", item.clientID)
	form.Set("client_secret", item.clientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", item.redirectURI)
	form.Set("code_verifier", item.pkceVerifier)

	request, err := http.NewRequest(http.MethodPost, endpointSet.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", ErrTokenExchange
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := m.httpClient.Do(request)
	if err != nil {
		return "", "", ErrTokenExchange
	}
	defer response.Body.Close()
	body, err := readBounded(response.Body, m.maxResponseBytes)
	if err != nil || response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", "", ErrTokenExchange
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
	}
	if err := json.Unmarshal(body, &token); err != nil || token.Error != "" || token.AccessToken == "" || token.RefreshToken == "" {
		return "", "", ErrTokenExchange
	}
	return token.RefreshToken, token.AccessToken, nil
}

func (m *Manager) validateDrive(item *flow, accessToken string) error {
	endpointSet, err := m.endpointSet(item.provider, item.isCN)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodGet, endpointSet.DriveURL, nil)
	if err != nil {
		return ErrDriveValidation
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	response, err := m.httpClient.Do(request)
	if err != nil {
		return ErrDriveValidation
	}
	defer response.Body.Close()
	if _, err := readBounded(response.Body, m.maxResponseBytes); err != nil {
		return ErrDriveValidation
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ErrDriveValidation
	}
	return nil
}

func (m *Manager) endpointSet(provider Provider, isCN bool) (EndpointSet, error) {
	var endpoints EndpointSet
	switch provider {
	case ProviderOneDrive:
		if isCN {
			endpoints = m.endpoints.OneDriveChina
		} else {
			endpoints = m.endpoints.OneDrive
		}
	case ProviderGoogleDrive:
		if isCN {
			return EndpointSet{}, ErrInvalidInput
		}
		endpoints = m.endpoints.GoogleDrive
	default:
		return EndpointSet{}, ErrUnsupportedProvider
	}
	if !validAbsoluteURL(endpoints.AuthorizationURL) || !validAbsoluteURL(endpoints.TokenURL) || !validAbsoluteURL(endpoints.DriveURL) {
		return EndpointSet{}, ErrInvalidInput
	}
	return endpoints, nil
}

func (m *Manager) readyFlowLocked(flowID string, now time.Time) (*flow, error) {
	item, ok := m.flows[flowID]
	if !ok {
		return nil, ErrFlowNotFound
	}
	if !now.Before(item.expiresAt) {
		m.deleteFlowLocked(item.id)
		return nil, ErrFlowExpired
	}
	if item.status != flowReady || item.refreshToken == "" {
		return nil, ErrFlowNotReady
	}
	return item, nil
}

func (m *Manager) cleanupExpiredLocked(now time.Time) {
	for flowID, item := range m.flows {
		if !now.Before(item.expiresAt) {
			m.deleteFlowLocked(flowID)
		}
	}
}

func (m *Manager) deleteFlowLocked(flowID string) {
	item, ok := m.flows[flowID]
	if !ok {
		return
	}
	if item.state != "" {
		delete(m.stateToFlow, item.state)
	}
	if item.expiryTimer != nil {
		item.expiryTimer.Stop()
		item.expiryTimer = nil
	}
	item.clientSecret = ""
	item.pkceVerifier = ""
	item.refreshToken = ""
	item.state = ""
	delete(m.flows, flowID)
}

func (m *Manager) randomToken() (string, error) {
	raw := make([]byte, randomTokenBytes)
	if _, err := io.ReadFull(m.random, raw); err != nil {
		return "", ErrRandomSource
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func storedResult(item *flow) StoredResult {
	return StoredResult{
		FlowID:          item.id,
		Provider:        item.provider,
		IsCN:            item.isCN,
		AccountIdentity: item.accountIdentity,
		ExpiresAt:       item.expiresAt,
		ClientID:        item.clientID,
		ClientSecret:    item.clientSecret,
		RedirectURI:     item.redirectURI,
		RefreshToken:    item.refreshToken,
	}
}

func buildAuthorizationURL(raw string, provider Provider, clientID, redirectURI, state, challenge string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", ErrInvalidInput
	}
	query := parsed.Query()
	query.Set("client_id", clientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	switch provider {
	case ProviderOneDrive:
		query.Set("response_mode", "query")
		query.Set("scope", "offline_access Files.ReadWrite.All User.Read")
	case ProviderGoogleDrive:
		query.Set("scope", "https://www.googleapis.com/auth/drive")
		query.Set("access_type", "offline")
		query.Set("prompt", "consent")
		query.Set("include_granted_scopes", "true")
	default:
		return "", ErrUnsupportedProvider
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func validateRedirectURI(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ErrInvalidInput
	}
	if !strings.EqualFold(parsed.Scheme, "https") && !(strings.EqualFold(parsed.Scheme, "http") && isLoopbackHost(parsed.Hostname())) {
		return ErrInvalidInput
	}
	query := parsed.Query()
	for _, reserved := range []string{"state", "code", "error", "error_description", "error_uri"} {
		if _, exists := query[reserved]; exists {
			return ErrInvalidInput
		}
	}
	return nil
}

func callbackMatchesRedirect(callback *url.URL, redirectURI string) bool {
	redirect, err := url.Parse(redirectURI)
	if err != nil {
		return false
	}
	if !strings.EqualFold(callback.Scheme, redirect.Scheme) || !strings.EqualFold(callback.Host, redirect.Host) || callback.Path != redirect.Path {
		return false
	}
	callbackQuery := callback.Query()
	for key, expected := range redirect.Query() {
		actual, ok := callbackQuery[key]
		if !ok || !equalStrings(actual, expected) {
			return false
		}
	}
	return true
}

func normalizeProvider(provider Provider) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(string(provider))) {
	case "onedrive", "microsoft":
		return ProviderOneDrive, nil
	case "googledrive", "google":
		return ProviderGoogleDrive, nil
	default:
		return "", ErrUnsupportedProvider
	}
}

func mergeEndpoints(configured, defaults Endpoints) Endpoints {
	configured.OneDrive = mergeEndpointSet(configured.OneDrive, defaults.OneDrive)
	configured.OneDriveChina = mergeEndpointSet(configured.OneDriveChina, defaults.OneDriveChina)
	configured.GoogleDrive = mergeEndpointSet(configured.GoogleDrive, defaults.GoogleDrive)
	return configured
}

func mergeEndpointSet(configured, defaults EndpointSet) EndpointSet {
	if configured.AuthorizationURL == "" {
		configured.AuthorizationURL = defaults.AuthorizationURL
	}
	if configured.TokenURL == "" {
		configured.TokenURL = defaults.TokenURL
	}
	if configured.DriveURL == "" {
		configured.DriveURL = defaults.DriveURL
	}
	return configured
}

func validAbsoluteURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") || (strings.EqualFold(parsed.Scheme, "http") && isLoopbackHost(parsed.Hostname()))
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func readBounded(reader io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		return nil, errors.New("OAuth response exceeds limit")
	}
	return data, nil
}
