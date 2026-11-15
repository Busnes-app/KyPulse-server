import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { TargetForm } from './TargetForm';

afterEach(cleanup);

it('prefills only a committed exact container link and preserves a supplied name', () => {
  render(<TargetForm submitLabel="Save" onSubmit={vi.fn()} onCancel={vi.fn()} suggestions={[
    { link: 'ep/app', endpoint_name: 'host', name: 'app', image: 'app:1', state: 'running' },
    { link: 'ep/app-worker', endpoint_name: 'host', name: 'app-worker', image: 'app:1', state: 'running' },
  ]} />);
  const container = screen.getByLabelText('KyYard container');
  const name = screen.getByLabelText('Name');
  fireEvent.change(container, { target: { value: 'ep/app' } });
  expect(name).toHaveProperty('value', '');
  fireEvent.change(container, { target: { value: 'ep/app-worker' } });
  fireEvent.blur(container);
  expect(name).toHaveProperty('value', 'app-worker');
  fireEvent.change(name, { target: { value: 'My app' } });
  fireEvent.change(container, { target: { value: 'ep/app' } });
  fireEvent.blur(container);
  expect(name).toHaveProperty('value', 'My app');
  fireEvent.change(name, { target: { value: '' } });
  fireEvent.change(container, { target: { value: 'ep/app-unknown' } });
  fireEvent.blur(container);
  expect(name).toHaveProperty('value', '');
});

it('sends a TLS-only target as tls://host:port with its server name and local permission', async () => {
  const onSubmit = vi.fn(async () => {});
  render(<TargetForm submitLabel="Add" onSubmit={onSubmit} onCancel={vi.fn()} />);
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Edge' } });
  fireEvent.click(screen.getByLabelText('TLS certificate only'));
  expect(screen.queryByLabelText('Health URL', { selector: 'input[type=url]' })).toBeNull();
  fireEvent.change(screen.getByLabelText('Host and port'), { target: { value: ' 127.0.0.1:8443 ' } });
  fireEvent.change(screen.getByLabelText('Server name (optional)'), { target: { value: 'edge.lan' } });
  fireEvent.click(screen.getByLabelText('Allow a loopback or link-local address'));
  fireEvent.submit(screen.getByRole('form', { name: 'Add' }));
  await vi.waitFor(() => expect(onSubmit).toHaveBeenCalled());
  expect(onSubmit.mock.calls[0]).toEqual([expect.objectContaining({ url: 'tls://127.0.0.1:8443', sni: 'edge.lan', allow_local: true })]);
});

it('opens a stored TLS target on the TLS fields', () => {
  render(<TargetForm submitLabel="Save" onSubmit={vi.fn()} onCancel={vi.fn()} initial={{
    id: 'e', name: 'Edge', url: 'tls://edge.lan:443', sni: 'proxy.lan', allow_local: false, interval_sec: 30, enabled: true, state: 'ok',
    state_since: 'x', until_fixed: false, last_result: '', last_latency_ms: 0, created_at: 'x', updated_at: 'x', basic: true,
  }} />);
  expect(screen.getByLabelText('Host and port')).toHaveProperty('value', 'edge.lan:443');
  expect(screen.getByLabelText('Server name (optional)')).toHaveProperty('value', 'proxy.lan');
});
