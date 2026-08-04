import { describe, expect, it, vi } from 'vitest';

import {
    buildOAuthBeginRequest,
    getBackupSyncStatusMeta,
    getUnlistedPendingBackupSyncStatuses,
    getOAuthStatusMeta,
    isOAuthSessionExpired,
    mergePublicBackupSyncStatuses,
    mergeOAuthCredentialInfo,
    navigateOAuthAuthorization,
    openOAuthAuthorization,
    openOAuthAuthorizationPlaceholder,
    requiresOAuthCredentialInput,
    resolveOAuthStatus,
    resolveBackupSyncOperationFeedback,
    resolveBackupSyncStatusLoad,
    sanitizeBackupSyncOperationResult,
    sanitizeBackupSyncStatuses,
    sanitizeOAuthVars,
    selectMoreSevereBackupSyncResult,
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

    it('hides missing and fully synchronized backup status', () => {
        expect(getBackupSyncStatusMeta()).toBeUndefined();
        expect(
            getBackupSyncStatusMeta({
                accountName: 'shared-drive',
                status: 'synced',
                succeeded: 2,
                pending: 0,
                total: 2,
            }),
        ).toBeUndefined();
    });

    it('maps pending and partial backup synchronization to safe progress metadata', () => {
        expect(
            getBackupSyncStatusMeta({
                accountName: 'shared-drive',
                status: 'sync_pending',
                succeeded: 0,
                pending: 3,
                total: 3,
            }),
        ).toEqual({
            labelKey: 'setting.backupSyncPending',
            descriptionKey: 'setting.backupSyncPendingHelp',
            tagType: 'warning',
            succeeded: 0,
            pending: 3,
            total: 3,
        });

        const partial = getBackupSyncStatusMeta({
            accountName: 'shared-drive',
            status: 'partially_synced',
            succeeded: 2,
            pending: 1,
            total: 3,
        });

        expect(partial).toEqual({
            labelKey: 'setting.backupSyncPartiallySynced',
            descriptionKey: 'setting.backupSyncPartiallySyncedHelp',
            tagType: 'danger',
            succeeded: 2,
            pending: 1,
            total: 3,
        });
        expect(JSON.stringify(partial)).not.toContain('internal-target-key');
        expect(partial).not.toHaveProperty('revision');
        expect(partial).not.toHaveProperty('targets');
    });

    it('fails closed with a visible warning for an unknown backup synchronization status', () => {
        expect(
            getBackupSyncStatusMeta({
                accountName: 'shared-drive',
                status: 'unexpected' as never,
                succeeded: 2,
                pending: 1,
                total: 3,
            }),
        ).toEqual({
            labelKey: 'setting.backupSyncPending',
            descriptionKey: 'setting.backupSyncStatusUnavailable',
            tagType: 'warning',
            succeeded: 2,
            pending: 1,
            total: 3,
        });
    });

    it('sanitizes list synchronization responses and degrades unknown non-empty states to pending', () => {
        const statuses = sanitizeBackupSyncStatuses([
            {
                accountName: 'shared-drive',
                status: 'partially_synced',
                succeeded: 2.8,
                pending: 1,
                total: 3,
                revision: 9,
                targets: [{ nodeId: 7 }],
                clientSecret: 'browser-must-discard-secret',
            },
            {
                accountName: 'future-drive',
                status: 'future_status',
                succeeded: 1,
                pending: 2,
                total: 3,
                refreshToken: 'browser-must-discard-token',
            },
            { accountName: 'missing-status' },
            { status: 'sync_pending' },
            null,
        ]);

        expect(statuses).toEqual([
            {
                accountName: 'shared-drive',
                status: 'partially_synced',
                succeeded: 2,
                pending: 1,
                total: 3,
            },
            {
                accountName: 'future-drive',
                status: 'sync_pending',
                succeeded: 1,
                pending: 2,
                total: 3,
            },
        ]);
        expect(JSON.stringify(statuses)).not.toContain('revision');
        expect(JSON.stringify(statuses)).not.toContain('targets');
        expect(JSON.stringify(statuses)).not.toContain('browser-must-discard');
        expect(sanitizeBackupSyncStatuses({ statuses: [] })).toEqual([]);
    });

    it('keeps synchronization status load failures visibly fail closed', () => {
        expect(resolveBackupSyncStatusLoad(undefined, false)).toEqual({
            statuses: [],
            unavailable: true,
        });
        expect(resolveBackupSyncStatusLoad({ statuses: [] }, true)).toEqual({
            statuses: [],
            unavailable: true,
        });
        expect(
            resolveBackupSyncStatusLoad(
                [
                    {
                        accountName: 'shared-drive',
                        status: 'sync_pending',
                        pending: 1,
                        total: 1,
                        clientSecret: 'browser-must-discard-secret',
                    },
                ],
                true,
            ),
        ).toEqual({
            statuses: [
                {
                    accountName: 'shared-drive',
                    status: 'sync_pending',
                    succeeded: 0,
                    pending: 1,
                    total: 1,
                },
            ],
            unavailable: false,
        });
    });

    it('attaches named synchronization state only to public accounts and keeps private names out of visibility', () => {
        const statuses = sanitizeBackupSyncStatuses([
            {
                accountName: 'shared-drive',
                status: 'sync_pending',
                succeeded: 0,
                pending: 2,
                total: 2,
            },
            {
                accountName: 'private-shadow',
                status: 'partially_synced',
                succeeded: 1,
                pending: 1,
                total: 2,
            },
        ]);
        const publicAccount = { name: 'shared-drive', isPublic: true };
        const privateAccount = { name: 'shared-drive', isPublic: false, sync: statuses[0] };
        const privateOnlyAccount = { name: 'private-shadow', isPublic: false, sync: statuses[1] };

        const merged = mergePublicBackupSyncStatuses([publicAccount, privateAccount, privateOnlyAccount], statuses);

        expect(merged[0].sync).toEqual(statuses[0]);
        expect(merged[1].sync).toBeUndefined();
        expect(merged[2].sync).toBeUndefined();
        expect(getUnlistedPendingBackupSyncStatuses(merged, statuses)).toEqual([statuses[1]]);
    });

    it('sanitizes operation feedback and rejects malformed synchronization responses', () => {
        expect(
            sanitizeBackupSyncOperationResult({
                applied: true,
                sync: {
                    accountName: 'shared-drive',
                    revision: 5,
                    status: 'partially_synced',
                    succeeded: 2,
                    pending: 1,
                    total: 3,
                    targets: [
                        {
                            targetKey: 'internal-target-key',
                            nodeId: 8,
                            nodeName: 'node-b',
                            desiredRevision: 5,
                            appliedRevision: 4,
                            attempts: 2,
                            nextRetryAt: '2026-08-04T12:10:00Z',
                            lastSuccessAt: '2026-08-04T11:00:00Z',
                            lastError: 'sanitized transport diagnostic',
                        },
                    ],
                    clientSecret: 'browser-must-discard-secret',
                    refreshToken: 'browser-must-discard-token',
                    snapshotDigest: 'browser-must-discard-digest',
                },
            }),
        ).toEqual({
            applied: true,
            sync: {
                accountName: 'shared-drive',
                status: 'partially_synced',
                succeeded: 2,
                pending: 1,
                total: 3,
            },
        });
        expect(sanitizeBackupSyncOperationResult({ applied: true, sync: { status: 'unexpected' } })).toBeUndefined();
        expect(sanitizeBackupSyncOperationResult({ applied: true })).toBeUndefined();
        expect(sanitizeBackupSyncOperationResult('invalid')).toBeUndefined();
    });

    it('classifies operation feedback fail closed and preserves the most severe batch result', () => {
        const synced = sanitizeBackupSyncOperationResult({
            applied: true,
            sync: {
                accountName: 'synced-drive',
                revision: 5,
                status: 'synced',
                succeeded: 3,
                pending: 0,
                total: 3,
            },
        });
        const pending = sanitizeBackupSyncOperationResult({
            applied: true,
            sync: {
                accountName: 'pending-drive',
                revision: 5,
                status: 'sync_pending',
                succeeded: 0,
                pending: 3,
                total: 3,
            },
        });
        const partial = sanitizeBackupSyncOperationResult({
            applied: true,
            sync: {
                accountName: 'partial-drive',
                revision: 5,
                status: 'partially_synced',
                succeeded: 2,
                pending: 1,
                total: 3,
            },
        });

        expect(resolveBackupSyncOperationFeedback(synced)).toEqual({ kind: 'success' });
        expect(resolveBackupSyncOperationFeedback(pending)).toMatchObject({
            kind: 'degraded',
            meta: { tagType: 'warning', succeeded: 0, pending: 3, total: 3 },
        });
        expect(resolveBackupSyncOperationFeedback({ applied: false, sync: partial?.sync })).toEqual({
            kind: 'unconfirmed',
        });
        expect(resolveBackupSyncOperationFeedback({ applied: true, sync: { status: 'unexpected' } })).toEqual({
            kind: 'unconfirmed',
        });
        expect(selectMoreSevereBackupSyncResult(synced, pending)).toEqual(pending);
        expect(selectMoreSevereBackupSyncResult(pending, partial)).toEqual(partial);
        expect(selectMoreSevereBackupSyncResult(partial, pending)).toEqual(partial);
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
