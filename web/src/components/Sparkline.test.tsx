import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { Sparkline, latencySummary } from './Sparkline';
import type { Beat } from '../monitor';

afterEach(cleanup);

const beat = (i: number, latency_ms: number, state: Beat['state'] = 'ok'): Beat => ({ at: `2026-10-31T10:00:${String(i).padStart(2, '0')}Z`, state, latency_ms });

describe('Sparkline', () => {
  it('names the range and median of answered polls', () => {
    expect(latencySummary([])).toBe('No latency recorded yet');
    expect(latencySummary([30])).toBe('Latency over the last 1 answered check: 30 ms, median 30 ms');
    expect(latencySummary([40, 10, 25, 30])).toBe('Latency over the last 4 answered checks: 10 to 40 ms, median 25 ms');
  });

  it('breaks the line at down polls and leaves them out of the summary', () => {
    render(<Sparkline beats={[beat(1, 20), beat(2, 40), beat(3, 900, 'down'), beat(4, 10)]} />);
    const svg = screen.getByRole('img', { name: 'Latency over the last 3 answered checks: 10 to 40 ms, median 20 ms' });
    const d = svg.querySelector('path')!.getAttribute('d')!;
    expect(d.match(/M/g)).toHaveLength(2);
    expect(svg.querySelector('circle')).toBeTruthy();
  });

  it('keeps its size with no history and draws no end dot after a down poll', () => {
    const { container } = render(<Sparkline beats={[beat(1, 20), beat(2, 0, 'down')]} />);
    const svg = container.querySelector('svg')!;
    expect(svg.getAttribute('width')).toBe('80');
    expect(svg.querySelector('circle')).toBeNull();
    cleanup();
    render(<Sparkline beats={null} />);
    expect(screen.getByRole('img', { name: 'No latency recorded yet' }).getAttribute('height')).toBe('24');
  });
});
