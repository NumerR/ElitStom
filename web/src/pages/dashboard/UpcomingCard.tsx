import { Button, Card, CardContent, CardHeader, Skeleton, Stack } from '@mui/material';
import EventAvailableRoundedIcon from '@mui/icons-material/EventAvailableRounded';
import { useQuery } from '@tanstack/react-query';
import { Link as RouterLink } from 'react-router-dom';
import { listAppointments } from '../../api/appointments';
import { queryKeys } from '../../api/queryKeys';
import type { AppointmentListParams } from '../../api/types';
import { useCurrentUser } from '../../auth/authContext';
import { AppointmentCompactList } from '../../components/appointments/AppointmentCompactList';
import { ClinicTimeNote } from '../../components/ClinicTimeNote';
import { EmptyState } from '../../components/EmptyState';
import { ErrorAlert } from '../../components/ErrorAlert';
import { useClinicTz } from '../../hooks/useClinic';
import { nowRFC3339Minute } from '../../lib/time';

const VISIBLE = 5;

export function UpcomingCard() {
  const user = useCurrentUser();
  const tz = useClinicTz();
  const params: AppointmentListParams = { from: nowRFC3339Minute(), order: 'asc', limit: 20 };
  const query = useQuery({
    queryKey: queryKeys.appointmentList(params),
    queryFn: () => listAppointments(params),
    select: (data) =>
      data.appointments
        .filter((a) => a.status === 'scheduled' || a.status === 'confirmed')
        .slice(0, VISIBLE),
  });

  const title = user.role === 'admin' ? 'Ближайшие записи клиники' : 'Ближайшие приёмы';

  return (
    <Card sx={{ height: '100%' }}>
      <CardHeader
        title={title}
        subheader={<ClinicTimeNote />}
        action={
          <Button component={RouterLink} to="/appointments" size="small">
            Все записи
          </Button>
        }
      />
      <CardContent sx={{ pt: 0 }}>
        {query.isPending && (
          <Stack spacing={1}>
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} variant="rounded" height={64} />
            ))}
          </Stack>
        )}
        {query.isError && <ErrorAlert error={query.error} onRetry={() => void query.refetch()} />}
        {query.isSuccess && query.data.length === 0 && (
          <EmptyState
            dense
            icon={<EventAvailableRoundedIcon />}
            title="Предстоящих приёмов нет"
            description={
              user.role === 'patient' ? 'Выберите удобное время у нужного врача.' : undefined
            }
            action={
              user.role === 'patient' || user.role === 'admin' ? (
                <Button component={RouterLink} to="/book" variant="tonal">
                  {user.role === 'patient' ? 'Записаться на приём' : 'Новая запись'}
                </Button>
              ) : undefined
            }
          />
        )}
        {query.isSuccess && query.data.length > 0 && (
          <AppointmentCompactList appointments={query.data} tz={tz} viewerRole={user.role} />
        )}
      </CardContent>
    </Card>
  );
}
