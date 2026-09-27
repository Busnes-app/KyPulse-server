import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { AlertBar } from './AlertBar';
import type { StatusSummary } from '../monitor';

const healthy: StatusSummary = {
  checked_at: '2026-09-27T10:00:00Z', total: 3, ok: 3, degraded: 0, down: 0, pending: 0, paused: 0, problems: [],
  webhook: { configured: true, last: { at: '2026-09-27T09:00:00Z', ok: true } },
};

afterEach(cleanup);

describe('AlertBar', () => {
  it('is green with the app count when all are healthy', () => {
    render(<AlertBar status={healthy} loading={false} isAdmin={false} />);
    const bar = screen.getByRole('status');
    expect(bar.className).toContain('alert-bar-ok');
    expect(bar.textContent).toContain('All 3 apps healthy');
  });

  it('is red with one line per problem and a link to Alerts', () => {
    render(<AlertBar status={{ ...healthy, ok: 1, down: 1, degraded: 1, problems: [
      { id: 'tgt_a', name: 'KyVault', state: 'down', since: '2026-09-27T09:30:00Z', cause: 'refused' },
      { id: 'tgt_b', name: 'KyPost', state: 'degraded', since: '2026-09-27T09:40:00Z' },
    ] }} loading={false} isAdmin={false} />);
    const bar = screen.getByRole('alert');
    expect(bar.className).toContain('alert-bar-down');
    expect(screen.getByRole('link', { name: /KyVault down/ }).getAttribute('href')).toBe('#/apps/tgt_a');
    expect(screen.getByRole('link', { name: /KyPost degraded/ })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Alerts' }).getAttribute('href')).toBe('#/alerts');
  });

  it('says alerts are not being delivered when the last delivery failed', () => {
    render(<AlertBar status={{ ...healthy, webhook: { configured: true, last: { at: 'x', ok: false, error: 'receiver_500' } } }} loading={false} isAdmin={true} />);
    expect(screen.getByText(/Alerts not being delivered/).textContent).toContain('receiver_500');
  });

  it('says nothing is watched yet on an empty install', () => {
    render(<AlertBar status={{ ...healthy, total: 0, ok: 0, problems: [] }} loading={false} isAdmin={true} />);
    expect(screen.getByRole('status').textContent).toMatch(/No apps watched yet/);
    expect(screen.queryByText(/All 0 apps/)).toBeNull();
  });

  it('shows checking while loading', () => {
    render(<AlertBar status={null} loading={true} isAdmin={false} />);
    expect(screen.getByRole('status').textContent).toMatch(/Checking/);
  });
});
