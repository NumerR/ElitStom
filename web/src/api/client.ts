import type { ApiErrorBody, AuthResponse, User } from './types';

const API_BASE = '/api/v1';
const REFRESH_KEY = 'elitstom.refresh_token';
const LOCK_NAME = 'elitstom-refresh';
const CHANNEL_NAME = 'elitstom-auth';

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: string[];

  constructor(status: number, code: string, message: string, details: string[] = []) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

export interface Session {
  user: User;
  accessToken: string;
}

export type SessionEvent = { type: 'session'; user: User } | { type: 'cleared' };
type SessionListener = (event: SessionEvent) => void;

type ChannelMessage =
  | { type: 'session'; user: User; accessToken: string; accessExpiresAt: number }
  | { type: 'cleared' };

let accessToken: string | null = null;
let accessExpiresAt = 0;
let sessionVersion = 0;
let lastUser: User | null = null;
let inflightRefresh: Promise<Session> | null = null;
const listeners = new Set<SessionListener>();

const channel: BroadcastChannel | null =
  typeof BroadcastChannel !== 'undefined' ? new BroadcastChannel(CHANNEL_NAME) : null;

function emit(event: SessionEvent) {
  listeners.forEach((listener) => listener(event));
}

export function subscribeSession(listener: SessionListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getStoredRefreshToken(): string | null {
  try {
    return localStorage.getItem(REFRESH_KEY);
  } catch {
    return null;
  }
}

function adoptAccess(user: User, token: string, expiresAt: number) {
  accessToken = token;
  accessExpiresAt = expiresAt;
  lastUser = user;
  sessionVersion += 1;
}

export function setSession(res: AuthResponse): Session {
  const expiresAt = Date.parse(res.tokens.access_expires_at) || 0;
  adoptAccess(res.user, res.tokens.access_token, expiresAt);
  localStorage.setItem(REFRESH_KEY, res.tokens.refresh_token);
  channel?.postMessage({
    type: 'session',
    user: res.user,
    accessToken: res.tokens.access_token,
    accessExpiresAt: expiresAt,
  } satisfies ChannelMessage);
  emit({ type: 'session', user: res.user });
  return { user: res.user, accessToken: res.tokens.access_token };
}

function dropLocalSession() {
  accessToken = null;
  accessExpiresAt = 0;
  lastUser = null;
  sessionVersion += 1;
}

export function clearSession() {
  dropLocalSession();
  try {
    localStorage.removeItem(REFRESH_KEY);
  } catch {
    // storage unavailable: nothing to clear
  }
  channel?.postMessage({ type: 'cleared' } satisfies ChannelMessage);
  emit({ type: 'cleared' });
}

channel?.addEventListener('message', (event: MessageEvent<ChannelMessage>) => {
  const msg = event.data;
  if (msg.type === 'session') {
    adoptAccess(msg.user, msg.accessToken, msg.accessExpiresAt);
    emit({ type: 'session', user: msg.user });
  } else if (msg.type === 'cleared') {
    dropLocalSession();
    emit({ type: 'cleared' });
  }
});

if (typeof window !== 'undefined') {
  window.addEventListener('storage', (event) => {
    if (event.key === REFRESH_KEY && event.newValue === null && accessToken !== null) {
      dropLocalSession();
      emit({ type: 'cleared' });
    }
  });
}

type Query = Record<string, string | number | undefined | null>;

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
  body?: unknown;
  query?: Query;
  auth?: boolean;
  signal?: AbortSignal;
}

function buildUrl(path: string, query?: Query): string {
  const params = new URLSearchParams();
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null && value !== '') params.set(key, String(value));
    }
  }
  const qs = params.toString();
  return `${API_BASE}${path}${qs ? `?${qs}` : ''}`;
}

function fallbackCode(status: number): string {
  if (status === 400) return 'invalid_input';
  if (status === 401) return 'unauthorized';
  if (status === 403) return 'forbidden';
  if (status === 404) return 'not_found';
  if (status === 409) return 'conflict';
  return 'internal_error';
}

function isErrorBody(data: unknown): data is ApiErrorBody {
  return (
    typeof data === 'object' &&
    data !== null &&
    'error' in data &&
    typeof (data as ApiErrorBody).error?.code === 'string'
  );
}

async function send<T>(path: string, opts: RequestOptions, token: string | null): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  const hasBody = opts.body !== undefined;
  if (hasBody) headers['Content-Type'] = 'application/json';
  if (token) headers.Authorization = `Bearer ${token}`;

  let res: Response;
  try {
    res = await fetch(buildUrl(path, opts.query), {
      method: opts.method ?? 'GET',
      headers,
      body: hasBody ? JSON.stringify(opts.body) : undefined,
      signal: opts.signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err;
    throw new ApiError(0, 'network', 'network request failed');
  }

  if (res.status === 204) return undefined as T;

  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }

  if (!res.ok) {
    const body = isErrorBody(data) ? data.error : undefined;
    throw new ApiError(
      res.status,
      body?.code ?? fallbackCode(res.status),
      body?.message ?? res.statusText,
      body?.details ?? [],
    );
  }
  return data as T;
}

function isTerminalRefreshError(err: unknown): boolean {
  return err instanceof ApiError && err.status >= 400 && err.status < 500;
}

async function refreshExclusive(): Promise<Session> {
  const seenToken = getStoredRefreshToken();
  const seenVersion = sessionVersion;

  const run = async (): Promise<Session> => {
    // Re-read after acquiring the lock: another tab may have rotated the token while we
    // waited. Presenting the old one would look like token reuse and revoke the family.
    const current = getStoredRefreshToken();
    if (!current) throw new ApiError(401, 'unauthorized', 'no refresh token');
    if (
      current !== seenToken &&
      sessionVersion !== seenVersion &&
      accessToken &&
      lastUser &&
      accessExpiresAt > Date.now() + 5_000
    ) {
      return { user: lastUser, accessToken };
    }
    const res = await send<AuthResponse>(
      '/auth/refresh',
      { method: 'POST', body: { refresh_token: current } },
      null,
    );
    return setSession(res);
  };

  try {
    if (typeof navigator !== 'undefined' && navigator.locks?.request) {
      return await navigator.locks.request(LOCK_NAME, run);
    }
    return await run();
  } catch (err) {
    if (isTerminalRefreshError(err)) clearSession();
    throw err;
  }
}

// Single-flight: parallel 401s in this tab must share one refresh, otherwise the second
// request would present an already-rotated refresh token and trip reuse detection.
export function refreshSession(): Promise<Session> {
  if (!inflightRefresh) {
    inflightRefresh = refreshExclusive().finally(() => {
      inflightRefresh = null;
    });
  }
  return inflightRefresh;
}

export async function apiRequest<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  if (opts.auth === false) return send<T>(path, opts, null);

  const token = accessToken ?? (await refreshSession()).accessToken;
  try {
    return await send<T>(path, opts, token);
  } catch (err) {
    if (!(err instanceof ApiError)) throw err;
    if (err.code === 'account_disabled') {
      clearSession();
      throw err;
    }
    if (err.status !== 401) throw err;

    const retryToken =
      accessToken && accessToken !== token ? accessToken : (await refreshSession()).accessToken;
    try {
      return await send<T>(path, opts, retryToken);
    } catch (retryErr) {
      if (retryErr instanceof ApiError && retryErr.status === 401) clearSession();
      throw retryErr;
    }
  }
}
