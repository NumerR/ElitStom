import { useCallback, useState, type ReactNode } from 'react';
import { Alert, Snackbar, type AlertColor } from '@mui/material';
import { NotifyContext } from './notifyContext';

interface Toast {
  key: number;
  message: string;
  severity: AlertColor;
}

export function NotifyProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<Toast | null>(null);
  const [open, setOpen] = useState(false);

  const notify = useCallback((message: string, severity: AlertColor = 'success') => {
    setToast({ key: Date.now(), message, severity });
    setOpen(true);
  }, []);

  return (
    <NotifyContext.Provider value={notify}>
      {children}
      <Snackbar
        key={toast?.key}
        open={open}
        autoHideDuration={toast?.severity === 'error' ? 7000 : 4000}
        onClose={(_, reason) => {
          if (reason !== 'clickaway') setOpen(false);
        }}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={() => setOpen(false)}
          severity={toast?.severity ?? 'success'}
          variant="filled"
          sx={{ width: '100%', alignItems: 'center' }}
        >
          {toast?.message}
        </Alert>
      </Snackbar>
    </NotifyContext.Provider>
  );
}
