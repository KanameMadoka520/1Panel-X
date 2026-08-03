import { beforeEach, describe, expect, it, vi } from 'vitest';

const httpMocks = vi.hoisted(() => ({
    post: vi.fn(),
    postLocalNode: vi.fn(),
}));

vi.mock('@/api', () => ({
    default: {
        post: httpMocks.post,
        postLocalNode: httpMocks.postLocalNode,
    },
}));

vi.mock('@/store', () => ({
    GlobalStore: vi.fn(() => ({ isProductPro: false })),
}));

import { checkBackup } from './backup';

describe('backup account node routing', () => {
    beforeEach(() => {
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
});
