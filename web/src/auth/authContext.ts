import { createContext, useContext } from 'react';
import type { LoginInput, RegisterInput, Role, User } from '../api/types';

export type AuthStatus = 'loading' | 'authenticated' | 'anonymous' | 'error';

export interface AuthContextValue {
  status: AuthStatus;
  user: User | null;
  login: (input: LoginInput) => Promise<void>;
  register: (input: RegisterInput) => Promise<void>;
  logout: () => Promise<void>;
  setUser: (user: User) => void;
  retry: () => void;
}

export const AuthContext = createContext<AuthContextValue | null>(null);

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

export function useCurrentUser(): User {
  const { user } = useAuth();
  if (!user) throw new Error('useCurrentUser requires an authenticated session');
  return user;
}

export function useHasRole(...roles: Role[]): boolean {
  const { user } = useAuth();
  return !!user && roles.includes(user.role);
}
