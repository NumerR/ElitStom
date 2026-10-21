import type { ReactNode } from 'react';
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  Divider,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from '@mui/material';
import ArrowBackRoundedIcon from '@mui/icons-material/ArrowBackRounded';
import EventBusyRoundedIcon from '@mui/icons-material/EventBusyRounded';
import { useQuery } from '@tanstack/react-query';
import { Link as RouterLink, useParams } from 'react-router-dom';
import { getAppointment } from '../api/appointments';
import { queryKeys } from '../api/queryKeys';
import type { Appointment } from '../api/types';
import { AppointmentActions } from '../components/appointments/AppointmentActions';
import { ClinicTimeNote } from '../components/ClinicTimeNote';
import { EmptyState } from '../components/EmptyState';
import { ErrorAlert } from '../components/ErrorAlert';
import { StatusChip } from '../components/StatusChip';
import { useClinicTz } from '../hooks/useClinic';
import { isApiError } from '../lib/errors';
import { initials } from '../lib/labels';
import {
  durationMinutes,
  formatDateLong,
  formatDateTime,
  formatDuration,
  formatTimeRange,
} from '../lib/time';

export function AppointmentDetailPage() {
  const { id = '' } = useParams();
  const query = useQuery({
    queryKey: queryKeys.appointment(id),
    queryFn: () => getAppointment(id),
    retry: (count, err) => !isApiError(err, 'not_found') && !isApiError(err, 'forbidden') && count < 2,
  });

  return (
    <>
      <Button component={RouterLink} to="/appointments" startIcon={<ArrowBackRoundedIcon />} sx={{ mb: 2, ml: -1 }}>
        К списку записей
      </Button>
      {query.isPending && <Skeleton variant="rounded" height={360} />}
      {query.isError &&
        (isApiError(query.error, 'not_found') || isApiError(query.error, 'forbidden') ? (
          <Card>
            <EmptyState
              icon={<EventBusyRoundedIcon />}
              title="Запись не найдена"
              description="Возможно, она была удалена или у вас нет к ней доступа."
              action={
                <Button component={RouterLink} to="/appointments" variant="tonal">
                  Все записи
                </Button>
              }
            />
          </Card>
        ) : (
          <ErrorAlert error={query.error} onRetry={() => void query.refetch()} />
        ))}
      {query.isSuccess && <AppointmentDetail appointment={query.data} />}
    </>
  );
}

function PersonRow({ label, name }: { label: string; name: string }) {
  return (
    <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center' }}>
      <Avatar sx={{ width: 40, height: 40, bgcolor: 'secondary.main', fontSize: 15 }}>{initials(name)}</Avatar>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="caption" color="text.secondary">
          {label}
        </Typography>
        <Typography sx={{ fontWeight: 500 }}>{name}</Typography>
      </Box>
    </Stack>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Box>
      <Typography variant="caption" color="text.secondary" component="dt">
        {label}
      </Typography>
      <Typography component="dd" sx={{ m: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
        {children}
      </Typography>
    </Box>
  );
}

function AppointmentDetail({ appointment: a }: { appointment: Appointment }) {
  const tz = useClinicTz();
  const minutes = durationMinutes(a.starts_at, a.ends_at);

  return (
    <Card>
      <Box
        sx={{
          p: { xs: 2.5, sm: 4 },
          bgcolor: (t) => t.palette.m3.primaryContainer,
          color: (t) => t.palette.m3.onPrimaryContainer,
        }}
      >
        <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 2 }}>
          <Box>
            <Typography variant="overline" sx={{ opacity: 0.8 }}>
              Приём
            </Typography>
            <Typography variant="h4" component="h1" sx={{ fontVariantNumeric: 'tabular-nums' }}>
              {formatTimeRange(a.starts_at, a.ends_at, tz)}
            </Typography>
            <Typography variant="subtitle1">{formatDateLong(a.starts_at, tz)}</Typography>
          </Box>
          <StatusChip status={a.status} size="medium" />
        </Stack>
      </Box>
      <CardContent sx={{ p: { xs: 2.5, sm: 4 } }}>
        <Stack spacing={3}>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, sm: 6 }}>
              <PersonRow label="Врач" name={a.doctor.full_name} />
            </Grid>
            <Grid size={{ xs: 12, sm: 6 }}>
              <PersonRow label="Пациент" name={a.patient.full_name} />
            </Grid>
          </Grid>
          <Divider />
          <Grid container spacing={2} component="dl" sx={{ m: 0 }}>
            <Grid size={{ xs: 12, sm: 6 }}>
              <Field label="Длительность">{formatDuration(minutes)}</Field>
            </Grid>
            <Grid size={{ xs: 12, sm: 6 }}>
              <Field label="Создана">{formatDateTime(a.created_at, tz)}</Field>
            </Grid>
            <Grid size={12}>
              <Field label="Причина обращения">{a.reason || '—'}</Field>
            </Grid>
          </Grid>
          {a.status === 'cancelled' && (
            <Alert severity="warning" variant="outlined">
              Запись отменена{a.cancel_reason ? `: ${a.cancel_reason}` : '.'}
            </Alert>
          )}
          <ClinicTimeNote />
          <AppointmentActions appointment={a} />
        </Stack>
      </CardContent>
    </Card>
  );
}
