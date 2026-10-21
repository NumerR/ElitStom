import { Route, Routes } from 'react-router-dom';
import { useAuth } from './auth/authContext';
import { PublicOnly, RequireAuth, RequireRole } from './auth/guards';
import { AppShell } from './components/AppShell';
import { SplashScreen } from './components/SplashScreen';
import { AdminUsersPage } from './pages/AdminUsersPage';
import { AppointmentDetailPage } from './pages/AppointmentDetailPage';
import { AppointmentsPage } from './pages/AppointmentsPage';
import { BookPage } from './pages/BookPage';
import { DashboardPage } from './pages/DashboardPage';
import { LoginPage } from './pages/LoginPage';
import { NotFoundPage } from './pages/NotFoundPage';
import { ProfilePage } from './pages/ProfilePage';
import { RegisterPage } from './pages/RegisterPage';
import { SchedulePage } from './pages/SchedulePage';

export function AppRoutes() {
  const { status, retry } = useAuth();

  if (status === 'loading') return <SplashScreen />;
  if (status === 'error') return <SplashScreen error onRetry={retry} />;

  return (
    <Routes>
      <Route element={<PublicOnly />}>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
      </Route>
      <Route element={<RequireAuth />}>
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="profile" element={<ProfilePage />} />
          <Route path="appointments" element={<AppointmentsPage />} />
          <Route path="appointments/:id" element={<AppointmentDetailPage />} />
          <Route element={<RequireRole roles={['patient', 'admin']} />}>
            <Route path="book" element={<BookPage />} />
          </Route>
          <Route element={<RequireRole roles={['doctor']} />}>
            <Route path="schedule" element={<SchedulePage />} />
          </Route>
          <Route element={<RequireRole roles={['admin']} />}>
            <Route path="admin/users" element={<AdminUsersPage />} />
          </Route>
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Route>
    </Routes>
  );
}
