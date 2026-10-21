import { Card, CardActionArea, CardContent, Stack, Typography } from '@mui/material';
import { formatDateShort, formatTimeRange } from '../../lib/time';
import { StatusChip } from '../StatusChip';
import type { AppointmentListProps } from './AppointmentTable';

export function AppointmentCards({ appointments, tz, showPatient, showDoctor, onOpen }: AppointmentListProps) {
  return (
    <Stack spacing={1.5} component="ul" sx={{ listStyle: 'none', p: 0, m: 0 }} aria-label="Список записей">
      {appointments.map((a) => (
        <li key={a.id}>
          <Card>
            <CardActionArea onClick={() => onOpen(a)}>
              <CardContent>
                <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 1 }}>
                  <div>
                    <Typography variant="subtitle1" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                      {formatTimeRange(a.starts_at, a.ends_at, tz)}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      {formatDateShort(a.starts_at, tz)}
                    </Typography>
                  </div>
                  <StatusChip status={a.status} />
                </Stack>
                <Stack spacing={0.25} sx={{ mt: 1.5 }}>
                  {showDoctor && <Typography variant="body2">Врач: {a.doctor.full_name}</Typography>}
                  {showPatient && <Typography variant="body2">Пациент: {a.patient.full_name}</Typography>}
                  {a.reason && (
                    <Typography variant="body2" color="text.secondary" noWrap>
                      {a.reason}
                    </Typography>
                  )}
                </Stack>
              </CardContent>
            </CardActionArea>
          </Card>
        </li>
      ))}
    </Stack>
  );
}
