import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError, getStoredRefreshToken, subscribeSession } from '../api/client';
import * as authApi from '../api/auth';
import type { LoginInput, RegisterInput, User } from '../api/types';
import { AuthContext, type AuthContextValue, type AuthStatus } from './authContext';

interface State {
  status: AuthStatus;
  user: User | null;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [state, setState] = useState<State>(() => ({
    status: getStoredRefreshToken() ? 'loading' : 'anonymous',
    user: null,
  }));

  useEffect(
    () =>
      subscribeSession((event) => {
        if (event.type === 'session') {
          setState({ status: 'authenticated', user: event.user });
        } else {
          setState({ status: 'anonymous', user: null });
          queryClient.clear();
        }
      }),
    [queryClient],
  );

  const restore = useCallback(() => {
    authApi
      .restoreSession()
      .then((session) => {
        if (!session) setState({ status: 'anonymous', user: null });
      })
      .catch((err: unknown) => {
        const terminal = err instanceof ApiError && err.status >= 400 && err.status < 500;
        setState({ status: terminal ? 'anonymous' : 'error', user: null });
      });
  }, []);

  useEffect(() => {
    if (getStoredRefreshToken()) restore();
  }, [restore]);

  const login = useCallback(async (input: LoginInput) => {
    await authApi.login(input);
  }, []);

  const register = useCallback(async (input: RegisterInput) => {
    await authApi.register(input);
  }, []);

  const logout = useCallback(async () => {
    await authApi.logout();
  }, []);

  const setUser = useCallback((user: User) => {
    setState((prev) => ({ ...prev, user }));
  }, []);

  const retry = useCallback(() => {
    setState({ status: 'loading', user: null });
    restore();
  }, [restore]);

  const value = useMemo<AuthContextValue>(
    () => ({ ...state, login, register, logout, setUser, retry }),
    [state, login, register, logout, setUser, retry],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
