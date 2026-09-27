import { secureFetch } from './api';
import { ApiError } from './monitor';

export interface LogUser { role: string; id?: string }
export interface LogLine {
  id: number; time: string; received_at: string; source_id: string; source: string;
  target_id: string; app: string; level: string; event: string; message: string; raw: string; truncated: boolean;
}
export interface AuditEvent {
  id: number; time: string; received_at: string; source_id: string; target_id: string;
  app: string; actor: string; action: string; target: string; outcome: string; ip: string;
}
export interface Burst { app: string; actor: string; ip: string; count: number; from: string; to: string }
export interface LogSource { id: string; name: string; target_id: string; created_at: string; revoked_at: string | null }
export interface Pairing { code: string; expires_at: string }
export interface Page<T> { items: T[]; next_before_id: number; bursts: Burst[]; has_app_events: boolean }

function object(value: unknown): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) throw new Error('Unexpected response');
  return value as Record<string, unknown>;
}
function text(value: unknown): string {
  if (typeof value !== 'string') throw new Error('Unexpected text in response');
  return value;
}
function number(value: unknown): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) throw new Error('Unexpected number in response');
  return value;
}
function date(value: unknown): string {
  const s = text(value);
  if (!Number.isFinite(Date.parse(s))) throw new Error('Unexpected date in response');
  return s;
}
function list<T>(value: unknown, decode: (v: unknown) => T): T[] {
  if (!Array.isArray(value)) throw new Error('Unexpected list in response');
  return value.map(decode);
}
function common(v: Record<string, unknown>) {
  return { id: number(v.id), time: date(v.time), received_at: date(v.received_at), source_id: text(v.source_id), target_id: text(v.target_id), app: text(v.app) };
}
function decodeLine(value: unknown): LogLine {
  const v = object(value);
  if (typeof v.truncated !== 'boolean') throw new Error('Unexpected truncation flag');
  return { ...common(v), source: text(v.source), level: text(v.level), event: text(v.event), message: text(v.message), raw: text(v.raw), truncated: v.truncated };
}
export function decodeLogs(value: unknown): Page<LogLine> {
  const v = object(value);
  return { items: list(v.items, decodeLine), next_before_id: number(v.next_before_id), bursts: [], has_app_events: false };
}
export function decodeActivity(value: unknown): Page<AuditEvent> {
  const v = object(value);
  if (typeof v.has_app_events !== 'boolean') throw new Error('Unexpected activity coverage');
  return {
    has_app_events: v.has_app_events,
    items: list(v.items, value => {
      const e = object(value);
      return { ...common(e), actor: text(e.actor), action: text(e.action), target: text(e.target), outcome: text(e.outcome), ip: text(e.ip) };
    }),
    next_before_id: number(v.next_before_id),
    bursts: list(v.bursts, value => {
      const b = object(value);
      return { app: text(b.app), actor: text(b.actor), ip: text(b.ip), count: number(b.count), from: date(b.from), to: date(b.to) };
    }),
  };
}
async function read(response: Response): Promise<unknown> {
  if (response.status === 401 || response.status === 403) window.dispatchEvent(new Event('logs-auth-denied'));
  const body: unknown = await response.json();
  if (!response.ok) {
    const v = object(body);
    throw new ApiError(response.status, typeof v.error === 'string' ? v.error : `Request failed (${response.status})`);
  }
  return body;
}
export const getLogs = (query: string, signal: AbortSignal) => fetch(`/api/logs?${query}`, { signal }).then(read).then(decodeLogs);
export const getActivity = (query: string, signal: AbortSignal) => fetch(`/api/activity?${query}`, { signal }).then(read).then(decodeActivity);
export async function getSources(signal: AbortSignal): Promise<LogSource[]> {
  const v = object(await read(await fetch('/api/log-sources', { signal })));
  return list(v.sources, value => {
    const s = object(value);
    return { id: text(s.id), name: text(s.name), target_id: text(s.target_id), created_at: date(s.created_at), revoked_at: s.revoked_at === null ? null : date(s.revoked_at) };
  });
}
export async function getSourceTargets(signal: AbortSignal): Promise<{ id: string; name: string }[]> {
  const v = object(await read(await fetch('/api/targets', { signal })));
  return list(v.targets, value => { const t = object(value); return { id: text(t.id), name: text(t.name) }; });
}
export async function createPairing(target_id: string, signal: AbortSignal): Promise<Pairing> {
  const v = object(await read(await secureFetch('/api/log-sources/pairing', { method: 'POST', signal, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ target_id }) })));
  const code = text(v.code);
  if (!/^\d{6}$/.test(code)) throw new Error('Unexpected pairing code');
  return { code, expires_at: date(v.expires_at) };
}
export async function revokeSource(id: string, signal: AbortSignal): Promise<void> {
  const v = object(await read(await secureFetch(`/api/log-sources/${encodeURIComponent(id)}`, { method: 'DELETE', signal })));
  if (v.revoked !== true) throw new Error('Unexpected revoke response');
}
export const logError = (error: unknown) => error instanceof Error ? error.message : 'Request failed';
