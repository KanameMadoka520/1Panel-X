import { describe, expect, it, vi } from 'vitest';

import {
    buildOAuthBeginRequest,
    getOAuthStatusMeta,
    isOAuthSessionExpired,
    mergeOAuthCredentialInfo,
    navigateOAuthAuthorization,
    openOAuthAuthorization,
    openOAuthAuthorizationPlaceholder,
    requiresOAuthCredentialInput,
    resolveOAuthStatus,
    sanitizeOAuthVars,
} from './oauth';

describe('backup account OAuth helpers', () => {
    it('removes every browser-readable OAuth secret from persisted vars', () => {
        const vars = sanitizeOAuthVars({
            oauth_status: 'configured',
            oauth_client_id_display: 'cli...123',
            client_id: 'read-back-client-id',
            client_secret: 'read-back-secret',
            refresh_token: 'read-back-refresh-token',
            accessToken: 'read-back-access-token',
            authorizationCode: 'read-back-authorization-code',
            code: 'read-back-code',
            code_verifier: 'read-back-code-verifier',
            codeVerifier: 'read-back-camel-code-verifier',
            codeChallenge: 'read-back-code-challenge',
            flowId: 'read-back-flow-id',
            pkceVerifier: 'read-back-pkce-verifier',
            state: 'read-back-state',
            redirect_uri: 'https://panel.example/oauth/callback',
            isCN: false,
        });

        expect(vars).toEqual({
            oauth_status: 'configured',
            oauth_client_id_display: 'cli...123',
            redirect_uri: 'https://panel.example/oauth/callback',
            isCN: false,
        });
    });

    it('rejects missing, invalid, and expired OAuth sessions', () => {
        const now = Date.parse('2026-08-03T12:00:00Z');

        expect(isOAuthSessionExpired('', now)).toBe(true);
        expect(isOAuthSessionExpired('not-a-date', now)).toBe(true);
        expect(isOAuthSessionExpired('2026-08-03T11:59:59Z', now)).toBe(true);
        expect(isOAuthSessionExpired('2026-08-03T12:00:00Z', now)).toBe(true);
        expect(isOAuthSessionExpired('2026-08-03T12:00:01Z', now)).toBe(false);
    });

    it('supports independent client ID and secret updates without sending an empty secret', () => {
        const retained = buildOAuthBeginRequest({
            provider: 'OneDrive',
            accountId: 12,
            accountName: 'backup',
            isPublic: false,
            isCN: false,
            clientSecret: '',
        });
        const clientIdOnly = buildOAuthBeginRequest({
            provider: 'OneDrive',
            accountId: 12,
            accountName: 'backup',
            isPublic: false,
            isCN: false,
            clientId: 'replacement-client-id',
            clientSecret: '',
        });
        const secretOnly = buildOAuthBeginRequest({
            provider: 'OneDrive',
            accountId: 12,
            accountName: 'backup',
            isPublic: false,
            isCN: false,
            clientSecret: 'rotated-client-secret',
        });
        const replaced = buildOAuthBeginRequest({
            provider: 'GoogleDrive',
            accountId: 0,
            accountName: 'new-backup',
            isPublic: true,
            isCN: false,
            clientId: 'new-client-id',
            clientSecret: 'new-client-secret',
            redirectUri: 'https://panel.example/oauth/callback',
        });

        expect(retained).not.toHaveProperty('clientSecret');
        expect(retained).not.toHaveProperty('refreshToken');
        expect(clientIdOnly).toMatchObject({ clientId: 'replacement-client-id' });
        expect(clientIdOnly).not.toHaveProperty('clientSecret');
        expect(secretOnly).toMatchObject({ clientSecret: 'rotated-client-secret' });
        expect(secretOnly).not.toHaveProperty('clientId');
        expect(replaced).toMatchObject({
            clientId: 'new-client-id',
            clientSecret: 'new-client-secret',
            redirectUri: 'https://panel.example/oauth/callback',
        });
        expect(replaced).not.toHaveProperty('refreshToken');
    });

    it('maps safe list fields to the four supported UI states', () => {
        expect(resolveOAuthStatus({ oauth_status: 'configured' })).toBe('configured');
        expect(resolveOAuthStatus({ oauth_status: 'legacy_reconfiguration_required' })).toBe(
            'legacy_reconfiguration_required',
        );
        expect(resolveOAuthStatus({ oauth_configured: true, oauth_authorized: false })).toBe(
            'reauthorization_required',
        );
        expect(getOAuthStatusMeta({ oauth_status: 'legacy_reconfiguration_required' })).toMatchObject({
            descriptionKey: 'setting.oauthLegacyReconfigurationHelp',
        });
        expect(getOAuthStatusMeta({ oauth_status: 'unconfigured' })).toMatchObject({
            tagType: 'info',
            requiresCredentials: true,
            requiresAuthorization: true,
        });
        expect(
            requiresOAuthCredentialInput({
                oauth_status: 'reauthorization_required',
                oauth_configured: false,
            }),
        ).toBe(true);
        expect(
            requiresOAuthCredentialInput({
                oauth_status: 'reauthorization_required',
                oauth_configured: true,
            }),
        ).toBe(false);
    });

    it('merges only safe credential status fields into account vars', () => {
        expect(
            mergeOAuthCredentialInfo(
                { isCN: false, client_secret: 'legacy-secret', refresh_token: 'legacy-refresh-token' },
                {
                    provider: 'Microsoft',
                    configured: true,
                    authorized: false,
                    clientIdDisplay: 'cli...123',
                    redirectUri: 'https://panel.example/oauth/callback',
                    status: 'reauthorization_required',
                    requiresReauthorization: true,
                    updatedAt: '2026-08-03T12:00:00Z',
                },
            ),
        ).toEqual({
            isCN: false,
            oauth_status: 'reauthorization_required',
            oauth_configured: true,
            oauth_authorized: false,
            oauth_client_id_display: 'cli...123',
            oauth_updated_at: '2026-08-03T12:00:00Z',
            redirect_uri: 'https://panel.example/oauth/callback',
        });
    });

    it('opens provider authorization with an isolated opener', () => {
        const opener = vi.fn();

        openOAuthAuthorization('https://provider.example/authorize?state=one-time', opener);

        expect(opener).toHaveBeenCalledWith(
            'https://provider.example/authorize?state=one-time',
            '_blank',
            'noopener,noreferrer',
        );
    });

    it('reserves a popup during the click event and isolates it before navigation', () => {
        const replace = vi.fn();
        const close = vi.fn();
        const popup = {
            opener: {} as WindowProxy,
            closed: false,
            close,
            location: { replace },
        } as unknown as WindowProxy;
        const opener = vi.fn(() => popup);

        const reserved = openOAuthAuthorizationPlaceholder(opener);
        const navigated = navigateOAuthAuthorization(reserved, 'https://provider.example/authorize?state=one-time');

        expect(opener).toHaveBeenCalledWith('about:blank', '_blank');
        expect(popup.opener).toBeNull();
        expect(replace).toHaveBeenCalledWith('https://provider.example/authorize?state=one-time');
        expect(navigated).toBe(true);
        expect(close).not.toHaveBeenCalled();
    });
});
