import { ReqPage } from '.';

export namespace Backup {
    export interface SearchWithType extends ReqPage {
        type: string;
        name: string;
    }
    export interface BackupOption {
        id: number;
        name: string;
        type: string;
    }
    export interface BackupInfo {
        id: number;
        name: string;
        type: string;
        isPublic: boolean;
        accessKey: string;
        bucket: string;
        credential: string;
        rememberAuth: boolean;
        backupPath: string;
        bucketInput: boolean;
        vars: string;
        varsJson: Record<string, any>;
        oauthSession?: string;
        oauth?: OAuthCredentialInfo;
        sync?: BackupSyncStatus;
        createdAt: Date;
    }
    export interface CheckResult {
        isOk: boolean;
        msg: string;
    }
    export interface BackupOperate {
        id: number;
        type: string;
        name: string;
        isPublic: boolean;
        accessKey: string;
        bucket: string;
        credential: string;
        backupPath: string;
        vars: string;
        oauthSession?: string;
    }
    export type OAuthProvider = 'OneDrive' | 'GoogleDrive';
    export type OAuthStatus =
        | 'configured'
        | 'unconfigured'
        | 'legacy_reconfiguration_required'
        | 'reauthorization_required';
    export interface OAuthBegin {
        provider: OAuthProvider;
        accountId: number;
        accountName: string;
        isPublic: boolean;
        clientId?: string;
        clientSecret?: string;
        redirectUri?: string;
        isCN: boolean;
    }
    export interface OAuthBeginResponse {
        flowId: string;
        authorizationUrl: string;
        expiresAt: string;
        clientIdDisplay: string;
    }
    export interface OAuthComplete {
        flowId: string;
        authorizationResponse: string;
    }
    export interface OAuthCompleteResponse {
        sessionId: string;
        provider: OAuthProvider;
        clientIdDisplay: string;
        expiresAt: string;
    }
    export interface OAuthCredentialInfo {
        provider: string;
        configured: boolean;
        authorized: boolean;
        clientIdDisplay: string;
        redirectUri: string;
        status: OAuthStatus;
        requiresReauthorization: boolean;
        updatedAt: string;
        sync?: BackupSyncStatus;
    }
    export type BackupSyncStatusValue = 'synced' | 'sync_pending' | 'partially_synced';
    export interface BackupSyncStatus {
        accountName: string;
        status: BackupSyncStatusValue;
        succeeded: number;
        pending: number;
        total: number;
    }
    export interface BackupSyncOperationResult {
        applied: boolean;
        sync: BackupSyncStatus;
    }
    export interface RecordDownload {
        downloadAccountID: number;
        fileDir: string;
        fileName: string;
    }
    export interface RecordInfo {
        id: number;
        createdAt: Date;
        accountType: string;
        accountName: string;
        downloadAccountID: number;
        fileDir: string;
        fileName: string;
        size: number;
    }
    export interface ForBucket {
        type: string;
        isPublic: boolean;
        accessKey: string;
        credential: string;
        vars: string;
    }
    export interface SearchBackupRecord extends ReqPage {
        type: string;
        name: string;
        detailName: string;
    }
    export interface SearchForSize extends ReqPage {
        type: string;
        name: string;
        detailName: string;
        info: string;
        cronjobID: number;
    }
    export interface RecordFileSize extends ReqPage {
        id: number;
        size: number;
    }
    export interface SearchBackupRecordByCronjob extends ReqPage {
        cronjobID: number;
    }
    export interface Backup {
        type: string;
        name: string;
        detailName: string;
        secret: string;
        taskID: string;
        stopBefore?: boolean;
    }
    export interface Recover {
        downloadAccountID: number;
        type: string;
        name: string;
        detailName: string;
        file: string;
        secret: string;
        taskID: string;
        dropAllCollections?: boolean;
    }
}
