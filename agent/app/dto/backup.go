package dto

import (
	"time"
)

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

	RememberAuth bool                 `json:"rememberAuth"`
	OAuth        *OAuthCredentialInfo `json:"oauth,omitempty"`
}

type BackupCheckRes struct {
	IsOk bool   `json:"isOk"`
	Msg  string `json:"msg"`
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

// BackupPublicSync is accepted only from the trusted core-to-agent channel.
// Its secret fields are encrypted with the receiving agent's key before the
// transaction commits and are never serialized in a response.
type BackupPublicSync struct {
	Authority      string                      `json:"authority"`
	Generation     string                      `json:"generation"`
	TargetEpoch    string                      `json:"targetEpoch"`
	Revision       uint64                      `json:"revision"`
	SnapshotDigest string                      `json:"snapshotDigest"`
	Accounts       []BackupPublicSyncAccount   `json:"accounts"`
	Tombstones     []BackupPublicSyncTombstone `json:"tombstones"`
}

type BackupPublicSyncTombstone struct {
	Name     string `json:"name"`
	Revision uint64 `json:"revision"`
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

type BackupPublicSyncResult struct {
	Authority       string `json:"authority"`
	Generation      string `json:"generation"`
	TargetEpoch     string `json:"targetEpoch"`
	AppliedRevision uint64 `json:"appliedRevision"`
	SnapshotDigest  string `json:"snapshotDigest"`
	Result          string `json:"result"`
}

type ForBuckets struct {
	Type       string `json:"type" validate:"required"`
	AccessKey  string `json:"accessKey"`
	Credential string `json:"credential" validate:"required"`
	Vars       string `json:"vars" validate:"required"`
}

type SyncFromMaster struct {
	Name      string `json:"name" validate:"required"`
	Operation string `json:"operation" validate:"required,oneof=create delete update"`
	Data      string `json:"data"`
}

type BackupOption struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	IsPublic bool   `json:"isPublic"`
}

type UploadForRecover struct {
	FilePath  string `json:"filePath"`
	TargetDir string `json:"targetDir"`
}

type CommonBackup struct {
	Type        string   `json:"type" validate:"required,oneof=app mysql mariadb redis website postgresql mongodb mysql-cluster postgresql-cluster redis-cluster container compose"`
	Name        string   `json:"name"`
	DetailName  string   `json:"detailName"`
	Secret      string   `json:"secret"`
	IsImmediate bool     `json:"isImmediate"`
	StopBefore  bool     `json:"stopBefore"`
	TaskID      string   `json:"taskID"`
	FileName    string   `json:"fileName"`
	Args        []string `json:"args"`

	Description string `json:"description"`
}
type CommonRecover struct {
	DownloadAccountID  uint   `json:"downloadAccountID" validate:"required"`
	Type               string `json:"type" validate:"required,oneof=app mysql mariadb redis website postgresql mongodb mysql-cluster postgresql-cluster redis-cluster container compose"`
	Name               string `json:"name"`
	DetailName         string `json:"detailName"`
	File               string `json:"file"`
	Secret             string `json:"secret"`
	DropAllCollections bool   `json:"dropAllCollections"`
	TaskID             string `json:"taskID"`
	BackupRecordID     uint   `json:"backupRecordID"`
	Timeout            int    `json:"timeout"`
}

type RecordSearch struct {
	PageInfo
	Type       string `json:"type" validate:"required"`
	Name       string `json:"name"`
	DetailName string `json:"detailName"`
}

type RecordSearchByCronjob struct {
	PageInfo
	CronjobID uint `json:"cronjobID" validate:"required"`
}

type BackupRecords struct {
	ID                uint      `json:"id"`
	CreatedAt         time.Time `json:"createdAt"`
	AccountType       string    `json:"accountType"`
	AccountName       string    `json:"accountName"`
	DownloadAccountID uint      `json:"downloadAccountID"`
	FileDir           string    `json:"fileDir"`
	FileName          string    `json:"fileName"`
	TaskID            string    `json:"taskID"`
	Status            string    `json:"status"`
	Message           string    `json:"message"`
	Description       string    `json:"description"`
}

type DownloadRecord struct {
	DownloadAccountID uint   `json:"downloadAccountID" validate:"required"`
	FileDir           string `json:"fileDir" validate:"required"`
	FileName          string `json:"fileName" validate:"required"`
}

type SearchForSize struct {
	PageInfo
	Type       string `json:"type" validate:"required"`
	Name       string `json:"name"`
	DetailName string `json:"detailName"`
	Info       string `json:"info"`
	CronjobID  uint   `json:"cronjobID"`
	OrderBy    string `json:"orderBy"`
	Order      string `json:"order"`
}
type RecordFileSize struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}
