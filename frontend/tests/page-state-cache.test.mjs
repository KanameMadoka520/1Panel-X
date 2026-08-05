import { expect, test } from 'vitest';
import { clearPageStateCache, getPageState } from '../src/utils/page-state-cache.ts';

test('reuses state for the same page key', () => {
    clearPageStateCache();
    const first = getPageState('local:Website', () => ({ name: '' }));
    first.name = 'example.com';

    const second = getPageState('local:Website', () => ({ name: 'ignored' }));

    expect(second.name).toBe('example.com');
    expect(second).toBe(first);
});

test('isolates state by page key', () => {
    clearPageStateCache();
    const local = getPageState('local:Website', () => ({ name: 'local' }));
    const remote = getPageState('remote:Website', () => ({ name: 'remote' }));

    expect(remote).not.toBe(local);
    expect(remote.name).toBe('remote');
});

test('creates fresh state after clearing the cache', () => {
    const first = getPageState('local:Website', () => ({ page: 2 }));
    clearPageStateCache();

    const second = getPageState('local:Website', () => ({ page: 1 }));

    expect(second).not.toBe(first);
    expect(second.page).toBe(1);
});
