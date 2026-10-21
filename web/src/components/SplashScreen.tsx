import { Box, Button, CircularProgress, Stack, Typography } from '@mui/material';
import CloudOffRoundedIcon from '@mui/icons-material/CloudOffRounded';
import { Logo } from './Logo';

interface Props {
  error?: boolean;
  onRetry?: () => void;
}

export function SplashScreen({ error, onRetry }: Props) {
  return (
    <Box
      sx={{ minHeight: '100dvh', display: 'grid', placeItems: 'center', bgcolor: 'background.default', p: 3 }}
      role={error ? 'alert' : 'status'}
      aria-live="polite"
    >
      <Stack spacing={3} sx={{ alignItems: 'center', textAlign: 'center' }}>
        <Logo size="large" />
        {error ? (
          <>
            <CloudOffRoundedIcon color="disabled" sx={{ fontSize: 48 }} />
            <Typography color="text.secondary" sx={{ maxWidth: 360 }}>
              Не удалось связаться с сервером. Проверьте подключение и попробуйте ещё раз.
            </Typography>
            <Button variant="contained" onClick={onRetry}>
              Повторить
            </Button>
          </>
        ) : (
          <>
            <CircularProgress aria-label="Загрузка" />
            <Typography color="text.secondary">Восстанавливаем сессию…</Typography>
          </>
        )}
      </Stack>
    </Box>
  );
}
