import { apiRequest, clearSession, getStoredRefreshToken, refreshSession, setSession } from './client';
import type { Session } from './client';
import type { AuthResponse, LoginInput, RegisterInput } from './types';

export async function login(input: LoginInput): Promise<Session> {
  const res = await apiRequest<AuthResponse>('/auth/login', { method: 'POST', body: input, auth: false });
  return setSession(res);
}

export async function register(input: RegisterInput): Promise<Session> {
  const res = await apiRequest<AuthResponse>('/auth/register', {
    method: 'POST',
    body: input,
    auth: false,
  });
  return setSession(res);
}

export async function restoreSession(): Promise<Session | null> {
  if (!getStoredRefreshToken()) return null;
  return refreshSession();
}

export async function logout(): Promise<void> {
  const refreshToken = getStoredRefreshToken();
  try {
    if (refreshToken) {
      await apiRequest<void>('/auth/logout', {
        method: 'POST',
        body: { refresh_token: getStoredRefreshToken() ?? refreshToken },
      });
    }
  } catch {
    // best-effort: the local session is dropped regardless
  } finally {
    clearSession();
  }
}
