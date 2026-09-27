import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WebhookForm } from './WebhookForm';

const saved = { configured: true, preset: 'ntfy', url: 'https://ntfy.sh/kypulse', has_token: true, last: { at: '2026-09-27T10:00:00Z', ok: false, error: 'receiver_500' } };

function stub(extra: Array<[RegExp, unknown, number?]> = [], info: unknown = saved) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${String(input)}`;
    for (const [re, body, status] of extra) if (re.test(key)) return new Response(status === 204 ? null : JSON.stringify(body), { status: status ?? 200 });
    if (key === 'GET /api/alerts/webhook') return new Response(JSON.stringify(info));
    throw new Error(key);
  });
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe('WebhookForm', () => {
  it('never renders the token and shows the last delivery', async () => {
    stub();
    const { container } = render(<WebhookForm onChanged={() => {}} />);
    expect(await screen.findByDisplayValue('https://ntfy.sh/kypulse')).toBeTruthy();
    const token = screen.getByLabelText(/Token/) as HTMLInputElement;
    expect(token.value).toBe('');
    expect(token.type).toBe('password');
    expect(token.placeholder).toMatch(/saved/i);
    expect(container.textContent).toContain('receiver_500');
  });

  it('keeps the saved token when the field is left empty', async () => {
    const fetchMock = stub([[/^PUT \/api\/alerts\/webhook$/, { ...saved, last: null }]]);
    const onChanged = vi.fn();
    render(<WebhookForm onChanged={onChanged} />);
    await screen.findByDisplayValue('https://ntfy.sh/kypulse');
    fireEvent.click(screen.getByRole('button', { name: 'Save webhook' }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const put = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'PUT')!;
    expect(JSON.parse((put[1] as RequestInit).body as string)).toEqual({ preset: 'ntfy', url: 'https://ntfy.sh/kypulse', token: '', clear_token: false });
  });

  it('sends clear_token when asked', async () => {
    const fetchMock = stub([[/^PUT \/api\/alerts\/webhook$/, { ...saved, has_token: false }]]);
    render(<WebhookForm onChanged={() => {}} />);
    await screen.findByDisplayValue('https://ntfy.sh/kypulse');
    fireEvent.click(screen.getByLabelText(/Remove the saved token/));
    fireEvent.click(screen.getByRole('button', { name: 'Save webhook' }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => (init as RequestInit)?.method === 'PUT')).toBe(true));
    const put = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'PUT')!;
    expect(JSON.parse((put[1] as RequestInit).body as string).clear_token).toBe(true);
  });

  it('shows the reason when a test send fails', async () => {
    stub([[/^POST \/api\/alerts\/webhook\/test$/, { error: 'receiver_500' }, 502]]);
    render(<WebhookForm onChanged={() => {}} />);
    await screen.findByDisplayValue('https://ntfy.sh/kypulse');
    fireEvent.click(screen.getByRole('button', { name: 'Send test' }));
    expect(await screen.findByText(/Test failed: receiver_500/)).toBeTruthy();
  });

  it('starts empty when nothing is configured', async () => {
    stub([], { configured: false });
    render(<WebhookForm onChanged={() => {}} />);
    expect(await screen.findByText(/No webhook configured/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Remove webhook' })).toBeNull();
  });

  it('clears the form after the webhook is removed', async () => {
    stub([[/^DELETE \/api\/alerts\/webhook$/, null, 204]]);
    vi.stubGlobal('confirm', vi.fn(() => true));
    render(<WebhookForm onChanged={() => {}} />);
    await screen.findByDisplayValue('https://ntfy.sh/kypulse');
    fireEvent.click(screen.getByRole('button', { name: 'Remove webhook' }));
    expect(await screen.findByText(/No webhook configured/)).toBeTruthy();
    expect((screen.getByLabelText('URL') as HTMLInputElement).value).toBe('');
    expect(screen.queryByRole('button', { name: 'Remove webhook' })).toBeNull();
  });
});
