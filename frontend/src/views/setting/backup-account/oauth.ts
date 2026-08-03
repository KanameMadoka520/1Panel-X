import { Backup } from '@/api/interface/backup';

export type OAuthTagType = 'success' | 'info' | 'warning' | 'danger';

export interface OAuthStatusMeta {
    labelKey: string;
    descriptionKey?: string;
    tagType: OAuthTagType;
    requiresCredentials: boolean;
    requiresAuthorization: boolean;
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
