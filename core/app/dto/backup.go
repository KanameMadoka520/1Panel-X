package dto

import "time"

type SyncToAgent struct {
	Name      string `json:"name" validate:"required"`
	Operation string `json:"operation" validate:"required,oneof=create delere update"`
	Data      string `json:"data"`
}

type BackupOperate struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type" validate:"required"`
	IsPublic   bool   `json:"isPublic"`
	Bucket     string `json:"bucket"`
	AccessKey  string `json:"accessKey"`
	Credential string `json:"credential"`
	BackupPath string `json:"backupPath"`
	Vars       string `json:"vars" validate:"required"`

	RememberAuth bool   `json:"rememberAuth"`
	OAuthSession string `json:"oauthSession"`
}

type BackupInfo struct {
	ID         uint      `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	IsPublic   bool      `json:"isPublic"`
	Bucket     string    `json:"bucket"`
	AccessKey  string    `json:"accessKey"`
	Credential string    `json:"credential"`
	BackupPath string    `json:"backupPath"`
	Vars       string    `json:"vars"`
	CreatedAt  time.Time `json:"createdAt"`

	RememberAuth bool `json:"rememberAuth"`
}

type BackupClientInfo struct {
	Provider        string `json:"provider"`
	Configured      bool   `json:"configured"`
	ClientIDDisplay string `json:"clientIdDisplay"`
	RedirectURI     string `json:"redirectUri"`
	Status          string `json:"status"`
	UpdatedAt       string `json:"updatedAt"`
}

type OAuthBegin struct {
	Provider     string `json:"provider" validate:"required,oneof=OneDrive GoogleDrive"`
	AccountID    uint   `json:"accountId"`
	AccountName  string `json:"accountName"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	RedirectURI  string `json:"redirectUri"`
	IsCN         bool   `json:"isCN"`
}

type OAuthBeginResponse struct {
	FlowID           string `json:"flowId"`
	AuthorizationURL string `json:"authorizationUrl"`
	ClientIDDisplay  string `json:"clientIdDisplay"`
	ExpiresAt        string `json:"expiresAt"`
}

type OAuthComplete struct {
	FlowID                string `json:"flowId" validate:"required"`
	AuthorizationResponse string `json:"authorizationResponse" validate:"required"`
}

type OAuthCompleteResponse struct {
	SessionID       string `json:"sessionId"`
	Provider        string `json:"provider"`
	ClientIDDisplay string `json:"clientIdDisplay"`
	ExpiresAt       string `json:"expiresAt"`
}

type OAuthCredentialInfo struct {
	Provider                string `json:"provider"`
	Configured              bool   `json:"configured"`
	Authorized              bool   `json:"authorized"`
	ClientIDDisplay         string `json:"clientIdDisplay"`
	RedirectURI             string `json:"redirectUri"`
	Status                  string `json:"status"`
	RequiresReauthorization bool   `json:"requiresReauthorization"`
	UpdatedAt               string `json:"updatedAt"`
}

type OAuthClear struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// BackupPublicSync is an internal core-to-agent payload. Secret fields are
// accepted only by the agent sync endpoint and are never returned by an API.
type BackupPublicSync struct {
	Accounts []BackupPublicSyncAccount `json:"accounts"`
}

type BackupPublicSyncAccount struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	IsPublic     bool                   `json:"isPublic"`
	Bucket       string                 `json:"bucket"`
	AccessKey    string                 `json:"accessKey"`
	Credential   string                 `json:"credential"`
	BackupPath   string                 `json:"backupPath"`
	Vars         string                 `json:"vars"`
	RememberAuth bool                   `json:"rememberAuth"`
	OAuth        *BackupOAuthSecretSync `json:"oauth,omitempty"`
}

type BackupOAuthSecretSync struct {
	Provider     string     `json:"provider"`
	ClientID     string     `json:"clientId"`
	ClientSecret string     `json:"clientSecret"`
	RedirectURI  string     `json:"redirectUri"`
	RefreshToken string     `json:"refreshToken"`
	IsCN         bool       `json:"isCN"`
	Status       string     `json:"status"`
	AuthorizedAt *time.Time `json:"authorizedAt,omitempty"`
}

type ForBuckets struct {
	Type       string `json:"type" validate:"required"`
	AccessKey  string `json:"accessKey"`
	Credential string `json:"credential" validate:"required"`
	Vars       string `json:"vars" validate:"required"`
}
