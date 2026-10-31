import { afterEach, describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { hrefFor, movedRoute, navigate, settingsSections, useHashRoute } from './router';

afterEach(() => { window.location.hash = ''; });

describe('useHashRoute', () => {
  it('defaults to /status when the hash is empty', () => {
    const { result } = renderHook(() => useHashRoute());
    expect(result.current).toEqual({ path: '/status', parts: ['status'] });
  });

  it('follows hashchange and splits the path', () => {
    const { result } = renderHook(() => useHashRoute());
    act(() => { navigate('/apps/tgt_1'); window.dispatchEvent(new HashChangeEvent('hashchange')); });
    expect(result.current).toEqual({ path: '/apps/tgt_1', parts: ['apps', 'tgt_1'] });
    expect(window.location.hash).toBe('#/apps/tgt_1');
  });

  it('normalises a hash without a leading slash', () => {
    window.location.hash = '#alerts';
    const { result } = renderHook(() => useHashRoute());
    expect(result.current.path).toBe('/alerts');
  });

  it('builds hrefs', () => {
    expect(hrefFor('/alerts')).toBe('#/alerts');
  });
});

describe('Settings routes', () => {
  it('orders the tabs Account, Appearance, then the server', () => {
    expect(settingsSections.map((s) => s.label)).toEqual(['Account', 'Appearance', 'Server']);
  });

  it('sends the old Appearance page and the bare Settings row to their tabs', () => {
    expect(movedRoute('/appearance')).toBe('/settings/appearance');
    expect(movedRoute('/settings')).toBe('/settings/account');
    expect(movedRoute('/settings/server')).toBeNull();
    expect(movedRoute('/status')).toBeNull();
  });
});
