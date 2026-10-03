import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { KyYardFacts } from './KyYardFacts';
import type { ContainerFacts } from '../monitor';

const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString();

function facts(over: Partial<ContainerFacts>): ContainerFacts {
  return {
    link: 'ep_1/kyvault', endpoint_id: 'ep_1', endpoint_name: 'host-1', container_id: 'c1', name: 'kyvault', image: 'kyvault:1',
    state: 'running', status: 'Up 3 hours', health: 'none', observed_at: ago(1), endpoint_offline: false,
    has_sample: true, sample_at: ago(3), memory_bytes: 104857600, memory_limit: 0, restart_count: 5, restarts_last_hour: 2,
    history_minutes: 60, stale: false, ...over,
  };
}

const show = (f: ContainerFacts) => render(<KyYardFacts facts={f} link={f.link} paired isAdmin={false} />);

afterEach(cleanup);

describe('KyYardFacts', () => {
  it('shows memory and restarts with the sample age', () => {
    show(facts({}));
    expect(screen.getByText('100 MiB')).toBeTruthy();
    expect(screen.getByText('2 in the last hour')).toBeTruthy();
    expect(screen.getByText('5 total · sampled 3m ago')).toBeTruthy();
    expect(screen.queryByText(/no sample yet/)).toBeNull();
    expect(screen.queryByText(/endpoint offline/)).toBeNull();
  });

  it('says how far back restarts go when history is under an hour', () => {
    show(facts({ history_minutes: 20 }));
    expect(screen.getByText('2 since 20 min ago')).toBeTruthy();
  });

  it('explains the initial restart history instead of saying zero minutes ago', () => {
    show(facts({ history_minutes: 0, restarts_last_hour: 0 }));
    expect(screen.getByText('0 since monitoring began (under 1 min)')).toBeTruthy();
    expect(screen.getByText('5 total · sampled 3m ago')).toBeTruthy();
  });

  it('hides memory and restarts without a sample', () => {
    show(facts({ has_sample: false, sample_at: undefined, state: 'exited', memory_bytes: 0, restarts_last_hour: 0 }));
    expect(screen.getByText('no sample yet (only running containers are sampled)')).toBeTruthy();
    expect(screen.queryByText('Memory')).toBeNull();
    expect(screen.queryByText('Restarts')).toBeNull();
  });

  it('marks a container on an offline endpoint', () => {
    show(facts({ endpoint_offline: true, stale: true }));
    expect(screen.getByText(/running · endpoint offline/)).toBeTruthy();
    expect(screen.getByText(/· stale/)).toBeTruthy();
  });
});
