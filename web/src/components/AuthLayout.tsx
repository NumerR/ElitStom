import type { ReactNode } from 'react';
import { Box, Card, CardContent, Stack, Typography } from '@mui/material';
import { Logo } from './Logo';
import { ThemeToggleButton } from './ThemeToggleButton';

interface Props {
  title: string;
  subtitle: string;
  children: ReactNode;
  footer: ReactNode;
}

export function AuthLayout({ title, subtitle, children, footer }: Props) {
  return (
    <Box
      component="main"
      sx={{
        minHeight: '100dvh',
        display: 'grid',
        placeItems: 'center',
        p: { xs: 2, sm: 3 },
        background: (t) =>
          `radial-gradient(1200px 600px at 10% -10%, ${t.palette.m3.primaryContainer} 0%, transparent 60%),
           radial-gradient(900px 500px at 110% 110%, ${t.palette.m3.secondaryContainer} 0%, transparent 55%),
           ${t.palette.background.default}`,
      }}
    >
      <Box sx={{ position: 'fixed', top: 12, right: 12 }}>
        <ThemeToggleButton />
      </Box>
      <Card sx={{ width: '100%', maxWidth: 440, borderRadius: 7 }}>
        <CardContent sx={{ p: { xs: 3, sm: 5 } }}>
          <Stack spacing={3}>
            <Logo size="large" />
            <Box>
              <Typography variant="h5" component="h1">
                {title}
              </Typography>
              <Typography color="text.secondary" sx={{ mt: 0.5 }}>
                {subtitle}
              </Typography>
            </Box>
            {children}
            <Box sx={{ textAlign: 'center' }}>{footer}</Box>
          </Stack>
        </CardContent>
      </Card>
    </Box>
  );
}
