import { Navigate, Outlet, useLocation, type Location } from 'react-router-dom';
import type { Role } from '../api/types';
import { useAuth } from './authContext';
import { ForbiddenPage } from '../pages/ForbiddenPage';

interface FromState {
  from?: Location;
}

export function RequireAuth() {
  const { status } = useAuth();
  const location = useLocation();
  if (status !== 'authenticated') {
    return <Navigate to="/login" replace state={{ from: location } satisfies FromState} />;
  }
  return <Outlet />;
}

export function RequireRole({ roles }: { roles: Role[] }) {
  const { user } = useAuth();
  if (!user || !roles.includes(user.role)) return <ForbiddenPage />;
  return <Outlet />;
}

export function PublicOnly() {
  const { status } = useAuth();
  const location = useLocation();
  if (status === 'authenticated') {
    const from = (location.state as FromState | null)?.from;
    const target = from && from.pathname !== '/login' ? `${from.pathname}${from.search}` : '/';
    return <Navigate to={target} replace />;
  }
  return <Outlet />;
}
