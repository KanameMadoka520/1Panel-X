import { beforeEach, describe, expect, it, vi } from 'vitest';

const httpMocks = vi.hoisted(() => ({
    get: vi.fn(),
    post: vi.fn(),
    postLocalNode: vi.fn(),
}));

vi.mock('@/api', () => ({
    default: {
        get: httpMocks.get,
        post: httpMocks.post,
        postLocalNode: httpMocks.postLocalNode,
    },
}));

vi.mock('@/store', () => ({
    GlobalStore: vi.fn(() => ({ isProductPro: false })),
}));

import { checkBackup, listBackupSyncStatuses, retryBackupSync } from './backup';

describe('backup account node routing', () => {
    beforeEach(() => {
        httpMocks.get.mockReset();
        httpMocks.post.mockReset();
        httpMocks.postLocalNode.mockReset();
        httpMocks.post.mockResolvedValue({ code: 200 });
    });

    it('keeps private OAuth connection checks on the currently selected node', async () => {
        await checkBackup({
            id: 7,
            type: 'OneDrive',
            name: 'remote-private-drive',
            isPublic: false,
            accessKey: '',
            bucket: '',
            credential: '',
            backupPath: '/',
            vars: '{}',
        });

        expect(httpMocks.post).toHaveBeenCalledOnce();
        expect(httpMocks.post).toHaveBeenCalledWith('/backups/conn/check', expect.any(Object));
        expect(httpMocks.postLocalNode).not.toHaveBeenCalled();
    });

    it('loads all public backup synchronization summaries in one core request', async () => {
        httpMocks.get.mockResolvedValue({ data: [] });

        await listBackupSyncStatuses();

        expect(httpMocks.get).toHaveBeenCalledOnce();
        expect(httpMocks.get).toHaveBeenCalledWith('/core/backups/sync/status');
    });

    it('retries synchronization through the core account endpoint', async () => {
        await retryBackupSync('shared-drive');

        expect(httpMocks.post).toHaveBeenCalledOnce();
        expect(httpMocks.post).toHaveBeenCalledWith('/core/backups/sync/retry', { name: 'shared-drive' });
        expect(httpMocks.postLocalNode).not.toHaveBeenCalled();
    });
});
