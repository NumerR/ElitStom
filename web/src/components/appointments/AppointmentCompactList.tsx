import { Box, List, ListItemButton, ListItemText, Stack, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import type { Appointment, Role } from '../../api/types';
import { formatTime, inTz } from '../../lib/time';
import { StatusChip } from '../StatusChip';

interface Props {
  appointments: Appointment[];
  tz: string;
  viewerRole: Role;
}

export function AppointmentCompactList({ appointments, tz, viewerRole }: Props) {
  return (
    <List disablePadding sx={{ display: 'grid', gap: 1 }}>
      {appointments.map((a) => {
        const start = inTz(a.starts_at, tz);
        const counterpart =
          viewerRole === 'patient'
            ? a.doctor.full_name
            : viewerRole === 'doctor'
              ? a.patient.full_name
              : `${a.patient.full_name} → ${a.doctor.full_name}`;
        return (
          <ListItemButton
            key={a.id}
            component={RouterLink}
            to={`/appointments/${a.id}`}
            sx={{ borderRadius: 3, bgcolor: (t) => t.palette.m3.surfaceContainer, gap: 2, py: 1.25 }}
          >
            <Box
              sx={{
                width: 56,
                flexShrink: 0,
                textAlign: 'center',
                borderRadius: 2.5,
                py: 0.75,
                bgcolor: (t) => t.palette.m3.primaryContainer,
                color: (t) => t.palette.m3.onPrimaryContainer,
              }}
            >
              <Typography variant="h6" component="span" sx={{ display: 'block', lineHeight: 1.1 }}>
                {start.format('D')}
              </Typography>
              <Typography variant="caption" component="span">
                {start.format('MMM')}
              </Typography>
            </Box>
            <ListItemText
              primary={
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                  <span style={{ fontVariantNumeric: 'tabular-nums' }}>
                    {formatTime(a.starts_at, tz)} · {start.format('dd')}
                  </span>
                </Stack>
              }
              secondary={counterpart}
              slotProps={{ primary: { sx: { fontWeight: 600 } }, secondary: { noWrap: true } }}
              sx={{ minWidth: 0 }}
            />
            <StatusChip status={a.status} />
          </ListItemButton>
        );
      })}
    </List>
  );
}
