import type { ReactNode } from 'react';
import { Box, Stack, Typography } from '@mui/material';

interface Props {
  icon: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  dense?: boolean;
}

export function EmptyState({ icon, title, description, action, dense }: Props) {
  return (
    <Stack
      spacing={1.5}
      sx={{ alignItems: 'center', textAlign: 'center', py: dense ? 3 : 6, px: 2 }}
    >
      <Box
        aria-hidden
        sx={{
          width: dense ? 56 : 72,
          height: dense ? 56 : 72,
          borderRadius: '50%',
          display: 'grid',
          placeItems: 'center',
          bgcolor: (t) => t.palette.m3.primaryContainer,
          color: (t) => t.palette.m3.onPrimaryContainer,
          '& svg': { fontSize: dense ? 28 : 36 },
        }}
      >
        {icon}
      </Box>
      <Typography variant={dense ? 'subtitle1' : 'h6'}>{title}</Typography>
      {description && (
        <Typography color="text.secondary" sx={{ maxWidth: 420 }}>
          {description}
        </Typography>
      )}
      {action && <Box sx={{ pt: 1 }}>{action}</Box>}
    </Stack>
  );
}
