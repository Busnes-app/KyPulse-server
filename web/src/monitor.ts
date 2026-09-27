// Types and calls for the monitoring routes (internal/api/AGENTS.md → Monitoring), plus the
// pure display rules the Status, Alerts and detail screens share.
import { secureFetch } from './api';

export type TargetState = 'pending' | 'ok' | 'degraded' | 'down';

export interface Target {
  id: string;
  name: string;
  url: string;
  interval_sec: number;
  enabled: boolean;
  container?: string;
  state: TargetState;
  state_since: string;
  cause?: string;
  silenced_until?: string | null;
  until_fixed: boolean;
  last_result: string;
  last_polled_at?: string | null;
  last_latency_ms: number;
  created_at: string;
  updated_at: string;
  basic: boolean;
}

export interface HealthCheck {
  name: string;
  status: 'ok' | 'degraded' | 'down';
  reason?: string;
}

export interface LastResult {
  state: TargetState;
  basic?: boolean;
  cause?: string;
  service?: string;
  checks?: HealthCheck[];
}

export interface TargetEvent {
  id: number;
  target_id: string;
  at: string;
  from: string;
  to: string;
  cause?: string;
  reminder: boolean;
  notified: boolean;
  notify_error?: string;
}

export interface Problem {
  id: string;
  name: string;
  state: TargetState;
  since: string;
  cause?: string;
}

export interface DeliveryStatus {
  at: string;
  ok: boolean;
  error?: string;
}

export interface StatusSummary {
  checked_at: string | null;
  total: number;
  ok: number;
  degraded: number;
  down: number;
  pending: number;
  paused: number;
  problems: Problem[] | null;
  webhook: { configured: boolean; last: DeliveryStatus | null };
  kyyard?: KyYardStatus;
}

export interface WebhookInfo {
  configured: boolean;
  preset?: string;
  url?: string;
  has_token?: boolean;
  last?: DeliveryStatus | null;
}

export interface TargetInput {
  name: string;
  url: string;
  interval_sec: number;
  enabled: boolean;
  container?: string;
}

export interface KyYardStatus {
  paired: boolean;
  url?: string;
  organization?: string;
  fetched_at?: string | null;
  stale: boolean;
  error?: string;
}

export interface ContainerFacts {
  link: string;
  endpoint_id: string;
  endpoint_name: string;
  container_id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  health: string;
  exit_code?: number;
  observed_at: string;
  memory_bytes: number;
  memory_limit: number;
  restart_count: number;
  restarts_last_hour: number;
  stale: boolean;
}

export interface Suggestion {
  link: string;
  endpoint_name: string;
  name: string;
  image: string;
  state: string;
}

export type SilenceFor = '1h' | '8h' | 'until_fixed' | 'off';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function readJSON<T>(resp: Response): Promise<T> {
  if (resp.status === 204) return undefined as T;
  if (resp.ok) {
    try {
      return (await resp.json()) as T;
    } catch {
      throw new ApiError(resp.status, 'Unexpected response (not JSON)');
    }
  }
  const body = await resp.json().catch(() => ({}));
  throw new ApiError(resp.status, (body as { error?: string }).error || `Request failed (${resp.status})`);
}

const json = (body: unknown): RequestInit => ({
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body),
});

export const getStatus = () => fetch('/api/status').then((r) => readJSON<StatusSummary>(r));
export const listTargets = () => fetch('/api/targets').then((r) => readJSON<{ targets: Target[] }>(r)).then((b) => b.targets ?? []);
export const getTarget = (id: string) =>
  fetch(`/api/targets/${encodeURIComponent(id)}`).then((r) =>
    readJSON<{ target: Target; last_result: LastResult | null; events: TargetEvent[] | null; kyyard: ContainerFacts | null }>(r));
export const listAlerts = (limit = 100) =>
  fetch(`/api/alerts?limit=${limit}`).then((r) => readJSON<{ events: TargetEvent[] | null; total: number }>(r));
export const createTarget = (input: TargetInput) =>
  secureFetch('/api/targets', { method: 'POST', ...json(input) }).then((r) => readJSON<{ target: Target }>(r)).then((b) => b.target);
export const updateTarget = (id: string, input: TargetInput) =>
  secureFetch(`/api/targets/${encodeURIComponent(id)}`, { method: 'PUT', ...json(input) }).then((r) => readJSON<{ target: Target }>(r)).then((b) => b.target);
export const deleteTarget = (id: string) =>
  secureFetch(`/api/targets/${encodeURIComponent(id)}`, { method: 'DELETE' }).then((r) => readJSON<void>(r));
export const silenceTarget = (id: string, f: SilenceFor) =>
  secureFetch(`/api/targets/${encodeURIComponent(id)}/silence`, { method: 'POST', ...json({ for: f }) })
    .then((r) => readJSON<{ silenced_until: string | null; until_fixed: boolean }>(r));
export const getWebhook = () => fetch('/api/alerts/webhook').then((r) => readJSON<WebhookInfo>(r));
export const saveWebhook = (input: { preset: string; url: string; token: string; clear_token: boolean }) =>
  secureFetch('/api/alerts/webhook', { method: 'PUT', ...json(input) }).then((r) => readJSON<WebhookInfo>(r));
export const deleteWebhook = () => secureFetch('/api/alerts/webhook', { method: 'DELETE' }).then((r) => readJSON<void>(r));
export const testWebhook = () => secureFetch('/api/alerts/webhook/test', { method: 'POST' }).then((r) => readJSON<{ ok: boolean }>(r));

export const getKyYard = () => fetch('/api/kyyard').then((r) => readJSON<KyYardStatus>(r));
export const pairKyYard = (input: { url: string; pairing_code: string }) =>
  secureFetch('/api/kyyard/pair', { method: 'POST', ...json(input) }).then((r) => readJSON<KyYardStatus>(r));
export const unpairKyYard = () => secureFetch('/api/kyyard', { method: 'DELETE' }).then((r) => readJSON<void>(r));
export const kyYardContainers = () => fetch('/api/kyyard/containers').then((r) => readJSON<Suggestion[]>(r));

// Display rules.

const rank: Record<string, number> = { down: 0, degraded: 1, pending: 2, ok: 3 };

// sortTargets puts broken apps first, paused last, names as the tie-break.
export function sortTargets(list: Target[]): Target[] {
  return [...list].sort((a, b) => {
    const ra = a.enabled ? rank[a.state] ?? 3 : 9;
    const rb = b.enabled ? rank[b.state] ?? 3 : 9;
    return ra - rb || a.name.localeCompare(b.name);
  });
}

export function stateLabel(t: Target): string {
  if (!t.enabled) return 'paused';
  if (t.state === 'ok' && t.basic) return 'ok (basic)';
  return t.state;
}

// stateClass is the CSS suffix for a dot or badge: ok|degraded|down|pending|paused.
export function stateClass(t: Pick<Target, 'state' | 'enabled'>): string {
  return t.enabled ? t.state : 'paused';
}

export function sinceLabel(iso: string, now: Date = new Date()): string {
  const s = Math.max(0, Math.floor((now.getTime() - new Date(iso).getTime()) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ${m % 60}m`;
  return `${Math.floor(h / 24)}d ${h % 24}h`;
}

export function timeLabel(iso: string | null | undefined): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString();
}

export function webhookFailing(s: StatusSummary): boolean {
  return s.webhook.configured && !!s.webhook.last && !s.webhook.last.ok;
}

// kyYardPending is true right after pairing, before the background loop's first pull has
// landed (or failed): paired, no fetched_at yet, and no error reported.
export function kyYardPending(s: Pick<KyYardStatus, 'paired' | 'fetched_at' | 'error'>): boolean {
  return s.paired && !s.fetched_at && !s.error;
}

export function isSilenced(t: Target, now: Date = new Date()): boolean {
  if (t.until_fixed) return true;
  return !!t.silenced_until && new Date(t.silenced_until).getTime() > now.getTime();
}
