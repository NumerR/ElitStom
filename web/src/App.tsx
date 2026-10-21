import { useState } from 'react';
import { QueryClientProvider } from '@tanstack/react-query';
import { LocalizationProvider } from '@mui/x-date-pickers/LocalizationProvider';
import { AdapterDayjs } from '@mui/x-date-pickers/AdapterDayjs';
import { BrowserRouter } from 'react-router-dom';
import './lib/time';
import { AuthProvider } from './auth/AuthProvider';
import { NotifyProvider } from './components/NotifyProvider';
import { AppThemeProvider } from './theme/AppThemeProvider';
import { createQueryClient } from './queryClient';
import { AppRoutes } from './AppRoutes';

export function App() {
  const [queryClient] = useState(createQueryClient);

  return (
    <QueryClientProvider client={queryClient}>
      <AppThemeProvider>
        <LocalizationProvider dateAdapter={AdapterDayjs} adapterLocale="ru">
          <NotifyProvider>
            <BrowserRouter>
              <AuthProvider>
                <AppRoutes />
              </AuthProvider>
            </BrowserRouter>
          </NotifyProvider>
        </LocalizationProvider>
      </AppThemeProvider>
    </QueryClientProvider>
  );
}
