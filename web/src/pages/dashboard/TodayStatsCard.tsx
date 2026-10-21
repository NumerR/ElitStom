import { Box, Card, CardContent, Chip, Skeleton, Stack, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { listAppointments } from '../../api/appointments';
import { queryKeys } from '../../api/queryKeys';
import type { AppointmentListParams, AppointmentStatus } from '../../api/types';
import { ErrorAlert } from '../../components/ErrorAlert';
import { useClinicTz } from '../../hooks/useClinic';
import { appointmentStatusLabels } from '../../lib/labels';
import { dayBounds, todayKey } from '../../lib/time';

const LIMIT = 200;

export function TodayStatsCard({ title }: { title: string }) {
  const tz = useClinicTz();
  const params: AppointmentListParams = { ...dayBounds(todayKey(tz), tz), order: 'asc', limit: LIMIT };
  const query = useQuery({
    queryKey: queryKeys.appointmentList(params),
    queryFn: () => listAppointments(params),
  });

  const all = query.data?.appointments ?? [];
  const active = all.filter((a) => a.status !== 'cancelled');
  const byStatus = all.reduce<Partial<Record<AppointmentStatus, number>>>((acc, a) => {
    acc[a.status] = (acc[a.status] ?? 0) + 1;
    return acc;
  }, {});
  const truncated = all.length === LIMIT;

  return (
    <Card
      sx={{
        height: '100%',
        bgcolor: (t) => t.palette.m3.primaryContainer,
        color: (t) => t.palette.m3.onPrimaryContainer,
        borderColor: 'transparent',
      }}
    >
      <CardContent>
        <Typography variant="subtitle2" sx={{ opacity: 0.85 }}>
          {title}
        </Typography>
        {query.isPending && <Skeleton variant="text" width={80} sx={{ fontSize: '3rem' }} />}
        {query.isError && (
          <Box sx={{ mt: 1 }}>
            <ErrorAlert error={query.error} onRetry={() => void query.refetch()} />
          </Box>
        )}
        {query.isSuccess && (
          <>
            <Typography variant="h2" component="p" sx={{ my: 1 }}>
              {active.length}
              {truncated ? '+' : ''}
            </Typography>
            <Stack direction="row" useFlexGap sx={{ flexWrap: 'wrap', gap: 0.75 }}>
              {(Object.keys(byStatus) as AppointmentStatus[]).map((s) => (
                <Chip
                  key={s}
                  size="small"
                  label={`${appointmentStatusLabels[s]}: ${byStatus[s]}`}
                  sx={{ bgcolor: 'rgba(255,255,255,0.35)', color: 'inherit' }}
                />
              ))}
              {all.length === 0 && <Typography variant="body2">Записей на сегодня нет</Typography>}
            </Stack>
          </>
        )}
      </CardContent>
    </Card>
  );
}
