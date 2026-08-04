import { Backup } from '@/api/interface/backup';

export type OAuthTagType = 'success' | 'info' | 'warning' | 'danger';

export interface OAuthStatusMeta {
    labelKey: string;
    descriptionKey?: string;
    tagType: OAuthTagType;
    requiresCredentials: boolean;
    requiresAuthorization: boolean;
}

export interface BackupSyncStatusMeta {
    labelKey: string;
    descriptionKey: string;
    tagType: Extract<OAuthTagType, 'warning' | 'danger'>;
    succeeded: number;
    pending: number;
    total: number;
}

export type BackupSyncOperationFeedback =
    | { kind: 'success' }
    | { kind: 'degraded'; meta: BackupSyncStatusMeta }
    | { kind: 'unconfirmed' };

export interface BackupSyncStatusLoadResult {
    statuses: Backup.BackupSyncStatus[];
    unavailable: boolean;
}

export interface OAuthBeginForm {
    provider: Backup.OAuthProvider;
    accountId: number;
    accountName: string;
    isPublic: boolean;
    isCN: boolean;
    clientId?: string;
    clientSecret?: string;
    redirectUri?: string;
}

export type OAuthWindowOpen = (url?: string | URL, target?: string, features?: string) => WindowProxy | null | void;

const statusMeta: Record<Backup.OAuthStatus, OAuthStatusMeta> = {
    configured: {
        labelKey: 'setting.oauthConfigured',
        tagType: 'success',
        requiresCredentials: false,
        requiresAuthorization: false,
    },
    unconfigured: {
        labelKey: 'setting.oauthUnconfigured',
        tagType: 'info',
        requiresCredentials: true,
        requiresAuthorization: true,
    },
    legacy_reconfiguration_required: {
        labelKey: 'setting.oauthLegacyReconfigurationRequired',
        descriptionKey: 'setting.oauthLegacyReconfigurationHelp',
        tagType: 'warning',
        requiresCredentials: true,
        requiresAuthorization: true,
    },
    reauthorization_required: {
        labelKey: 'setting.oauthReauthorizationRequired',
        descriptionKey: 'setting.oauthReauthorizationHelp',
        tagType: 'danger',
        requiresCredentials: false,
        requiresAuthorization: true,
    },
};

const sensitiveOAuthKeys = new Set([
    'access_token',
    'accesstoken',
    'authorization_response',
    'authorization_url',
    'authorization_code',
    'authorizationcode',
    'authorizationresponse',
    'authorizationurl',
    'client_id',
    'client_secret',
    'code_challenge',
    'codechallenge',
    'code_verifier',
    'codeverifier',
    'clientid',
    'clientsecret',
    'code',
    'flow_id',
    'flowid',
    'id_token',
    'idtoken',
    'oauth_session',
    'oauthsession',
    'pkce_verifier',
    'pkceverifier',
    'refresh_token',
    'refreshtoken',
    'session_id',
    'sessionid',
    'state',
    'token',
]);

const asBoolean = (value: unknown) => value === true || value === 'true';
const asSafeCount = (value: unknown) =>
    typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
const asSafeSyncCount = (value: unknown) =>
    typeof value === 'number' && Number.isFinite(value) && value >= 0 ? Math.floor(value) : 0;
const isRecord = (value: unknown): value is Record<string, unknown> => {
    return typeof value === 'object' && value !== null && !Array.isArray(value);
};

const sanitizeBackupSyncStatus = (
    value: unknown,
    unknownStatusFallback?: Backup.BackupSyncStatusValue,
): Backup.BackupSyncStatus | undefined => {
    if (!isRecord(value) || typeof value.accountName !== 'string' || !value.accountName.trim()) return undefined;

    let status: Backup.BackupSyncStatusValue;
    if (value.status === 'synced' || value.status === 'sync_pending' || value.status === 'partially_synced') {
        status = value.status;
    } else if (typeof value.status === 'string' && value.status.trim() && unknownStatusFallback) {
        status = unknownStatusFallback;
    } else {
        return undefined;
    }

    return {
        accountName: value.accountName.trim(),
        status,
        succeeded: asSafeSyncCount(value.succeeded),
        pending: asSafeSyncCount(value.pending),
        total: asSafeSyncCount(value.total),
    };
};

export const isOAuthProvider = (provider: string): provider is Backup.OAuthProvider => {
    return provider === 'OneDrive' || provider === 'GoogleDrive';
};

export const resolveOAuthStatus = (vars: Record<string, any> = {}): Backup.OAuthStatus => {
    const status = vars.oauth_status as Backup.OAuthStatus;
    if (statusMeta[status]) {
        return status;
    }
    if (asBoolean(vars.oauth_configured) && asBoolean(vars.oauth_authorized)) {
        return 'configured';
    }
    if (asBoolean(vars.oauth_configured)) {
        return 'reauthorization_required';
    }
    return 'unconfigured';
};

export const getOAuthStatusMeta = (vars: Record<string, any> = {}): OAuthStatusMeta => {
    return statusMeta[resolveOAuthStatus(vars)];
};

export const getBackupSyncStatusMeta = (sync?: Backup.BackupSyncStatus): BackupSyncStatusMeta | undefined => {
    if (!sync) return undefined;
    const status = sync.status as string;
    if (status === 'synced') return undefined;

    const progress = {
        succeeded: asSafeCount(sync.succeeded),
        pending: asSafeCount(sync.pending),
        total: asSafeCount(sync.total),
    };
    if (status === 'sync_pending') {
        return {
            labelKey: 'setting.backupSyncPending',
            descriptionKey: 'setting.backupSyncPendingHelp',
            tagType: 'warning',
            ...progress,
        };
    }
    if (status === 'partially_synced') {
        return {
            labelKey: 'setting.backupSyncPartiallySynced',
            descriptionKey: 'setting.backupSyncPartiallySyncedHelp',
            tagType: 'danger',
            ...progress,
        };
    }
    if (status.trim()) {
        return {
            labelKey: 'setting.backupSyncPending',
            descriptionKey: 'setting.backupSyncStatusUnavailable',
            tagType: 'warning',
            ...progress,
        };
    }
    return undefined;
};

export const sanitizeBackupSyncStatuses = (value: unknown): Backup.BackupSyncStatus[] => {
    if (!Array.isArray(value)) return [];
    return value
        .map((status) => sanitizeBackupSyncStatus(status, 'sync_pending'))
        .filter((status): status is Backup.BackupSyncStatus => !!status);
};

export const resolveBackupSyncStatusLoad = (value: unknown, requestSucceeded: boolean): BackupSyncStatusLoadResult => {
    const available = requestSucceeded && Array.isArray(value);
    return {
        statuses: available ? sanitizeBackupSyncStatuses(value) : [],
        unavailable: !available,
    };
};

type BackupSyncAccount = Pick<Backup.BackupInfo, 'name' | 'isPublic' | 'oauth' | 'sync'>;
type BackupSyncMergedAccount<T extends BackupSyncAccount> = T & { sync?: Backup.BackupSyncStatus };

export const mergePublicBackupSyncStatuses = <T extends BackupSyncAccount>(
    accounts: T[],
    statuses: Backup.BackupSyncStatus[],
): BackupSyncMergedAccount<T>[] => {
    const syncByAccountName = new Map(
        sanitizeBackupSyncStatuses(statuses).map((status) => [status.accountName, status]),
    );
    return accounts.map((account) => {
        if (!account.isPublic) return { ...account, sync: undefined };
        return {
            ...account,
            sync: syncByAccountName.get(account.name) || sanitizeBackupSyncStatus(account.oauth?.sync, 'sync_pending'),
        };
    });
};

export const getUnlistedPendingBackupSyncStatuses = (
    accounts: Array<Pick<Backup.BackupInfo, 'name' | 'isPublic'>>,
    statuses: Backup.BackupSyncStatus[],
): Backup.BackupSyncStatus[] => {
    const visiblePublicAccountNames = new Set(
        accounts.filter((account) => account.isPublic).map((account) => account.name),
    );
    return statuses.filter(
        (status) => !!getBackupSyncStatusMeta(status) && !visiblePublicAccountNames.has(status.accountName),
    );
};

export const sanitizeBackupSyncOperationResult = (value: unknown): Backup.BackupSyncOperationResult | undefined => {
    if (!isRecord(value) || typeof value.applied !== 'boolean' || !isRecord(value.sync)) return undefined;

    const sync = sanitizeBackupSyncStatus(value.sync);
    if (!sync) return undefined;

    return {
        applied: value.applied,
        sync,
    };
};

export const resolveBackupSyncOperationFeedback = (value: unknown): BackupSyncOperationFeedback => {
    const result = sanitizeBackupSyncOperationResult(value);
    if (!result?.applied) return { kind: 'unconfirmed' };

    const meta = getBackupSyncStatusMeta(result.sync);
    return meta ? { kind: 'degraded', meta } : { kind: 'success' };
};

const backupSyncSeverity: Record<Backup.BackupSyncStatusValue, number> = {
    synced: 0,
    sync_pending: 1,
    partially_synced: 2,
};

export const selectMoreSevereBackupSyncResult = (
    current: Backup.BackupSyncOperationResult | undefined,
    candidate: Backup.BackupSyncOperationResult | undefined,
): Backup.BackupSyncOperationResult | undefined => {
    if (!candidate) return current;
    if (!current) return candidate;
    return backupSyncSeverity[candidate.sync.status] > backupSyncSeverity[current.sync.status] ? candidate : current;
};

export const requiresOAuthCredentialInput = (vars: Record<string, any> = {}): boolean => {
    return getOAuthStatusMeta(vars).requiresCredentials || !asBoolean(vars.oauth_configured);
};

export const isOAuthSessionExpired = (expiresAt: string, now = Date.now()): boolean => {
    const expiresAtTimestamp = Date.parse(expiresAt);
    return !expiresAt || Number.isNaN(expiresAtTimestamp) || expiresAtTimestamp <= now;
};

export const sanitizeOAuthVars = (vars: Record<string, any> = {}): Record<string, any> => {
    const sanitized: Record<string, any> = {};
    for (const [key, value] of Object.entries(vars)) {
        const normalizedKey = key.replaceAll('-', '_').toLowerCase();
        if (!sensitiveOAuthKeys.has(normalizedKey)) {
            sanitized[key] = value;
        }
    }
    return sanitized;
};

export const mergeOAuthCredentialInfo = (
    vars: Record<string, any> = {},
    credential?: Backup.OAuthCredentialInfo,
): Record<string, any> => {
    const sanitized = sanitizeOAuthVars(vars);
    if (!credential) return sanitized;

    return {
        ...sanitized,
        oauth_status: credential.status,
        oauth_configured: credential.configured,
        oauth_authorized: credential.authorized,
        oauth_client_id_display: credential.clientIdDisplay,
        oauth_updated_at: credential.updatedAt,
        redirect_uri: credential.redirectUri,
    };
};

export const buildOAuthBeginRequest = (form: OAuthBeginForm): Backup.OAuthBegin => {
    const request: Backup.OAuthBegin = {
        provider: form.provider,
        accountId: form.accountId,
        accountName: form.accountName.trim(),
        isPublic: form.isPublic,
        isCN: form.isCN,
    };
    const clientId = form.clientId?.trim();
    const redirectUri = form.redirectUri?.trim();
    if (clientId) request.clientId = clientId;
    if (form.clientSecret) request.clientSecret = form.clientSecret;
    if (redirectUri) request.redirectUri = redirectUri;
    return request;
};

export const openOAuthAuthorization = (authorizationUrl: string, opener?: OAuthWindowOpen) => {
    const openWindow = opener || ((...args: Parameters<OAuthWindowOpen>) => window.open(...args));
    return openWindow(authorizationUrl, '_blank', 'noopener,noreferrer');
};

export const openOAuthAuthorizationPlaceholder = (opener?: OAuthWindowOpen): WindowProxy | null => {
    const openWindow = opener || ((...args: Parameters<OAuthWindowOpen>) => window.open(...args));
    const popup = openWindow('about:blank', '_blank');
    if (!popup) return null;

    try {
        popup.opener = null;
    } catch {
        popup.close();
        return null;
    }
    return popup;
};

export const navigateOAuthAuthorization = (popup: WindowProxy | null, authorizationUrl: string): boolean => {
    if (!popup || popup.closed) return false;
    try {
        popup.location.replace(authorizationUrl);
        return true;
    } catch {
        popup.close();
        return false;
    }
};
