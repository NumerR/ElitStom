import { Alert, Box, Button, Card, CardContent, IconButton, Skeleton, Stack, Tooltip, Typography } from '@mui/material';
import { DatePicker } from '@mui/x-date-pickers/DatePicker';
import ChevronLeftRoundedIcon from '@mui/icons-material/ChevronLeftRounded';
import ChevronRightRoundedIcon from '@mui/icons-material/ChevronRightRounded';
import EventAvailableRoundedIcon from '@mui/icons-material/EventAvailableRounded';
import { useQuery } from '@tanstack/react-query';
import { Link as RouterLink, useSearchParams } from 'react-router-dom';
import { listAppointments } from '../api/appointments';
import { queryKeys } from '../api/queryKeys';
import type { Appointment, AppointmentListParams } from '../api/types';
import { AppointmentActions } from '../components/appointments/AppointmentActions';
import { ClinicTimeNote } from '../components/ClinicTimeNote';
import { EmptyState } from '../components/EmptyState';
import { ErrorAlert } from '../components/ErrorAlert';
import { PageHeader } from '../components/PageHeader';
import { StatusChip } from '../components/StatusChip';
import { useClinicSchedule, useClinicTz } from '../hooks/useClinic';
import {
  DATE_KEY_FORMAT,
  dayBounds,
  dayjs,
  formatDateKeyLong,
  formatTime,
  isoWeekdayOfKey,
  shiftDateKey,
  todayKey,
} from '../lib/time';

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

export function SchedulePage() {
  const tz = useClinicTz();
  const schedule = useClinicSchedule();
  const [sp, setSp] = useSearchParams();
  const today = todayKey(tz);
  const raw = sp.get('date');
  const date = raw && DATE_RE.test(raw) ? raw : today;

  const setDate = (next: string) => {
    setSp(next === today ? {} : { date: next }, { replace: true });
  };

  const params: AppointmentListParams = { ...dayBounds(date, tz), order: 'asc', limit: 100 };
  const query = useQuery({
    queryKey: queryKeys.appointmentList(params),
    queryFn: () => listAppointments(params),
    refetchInterval: date === today ? 60_000 : false,
  });

  const dayOff = schedule.data ? !schedule.data.workdays.includes(isoWeekdayOfKey(date)) : false;

  return (
    <>
      <PageHeader title="Моё расписание" subtitle={<ClinicTimeNote />} />

      <Card sx={{ mb: 2 }}>
        <CardContent>
          <Stack
            direction={{ xs: 'column', sm: 'row' }}
            spacing={2}
            sx={{ alignItems: { xs: 'stretch', sm: 'center' }, justifyContent: 'space-between' }}
          >
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
              <Tooltip title="Предыдущий день">
                <IconButton aria-label="Предыдущий день" onClick={() => setDate(shiftDateKey(date, -1))}>
                  <ChevronLeftRoundedIcon />
                </IconButton>
              </Tooltip>
              <Button variant="tonal" onClick={() => setDate(today)} disabled={date === today}>
                Сегодня
              </Button>
              <Tooltip title="Следующий день">
                <IconButton aria-label="Следующий день" onClick={() => setDate(shiftDateKey(date, 1))}>
                  <ChevronRightRoundedIcon />
                </IconButton>
              </Tooltip>
              <Typography variant="h6" component="h2" sx={{ ml: 1, display: { xs: 'none', md: 'block' } }}>
                {formatDateKeyLong(date)}
              </Typography>
            </Stack>
            <DatePicker
              label="Дата"
              value={dayjs(date)}
              onChange={(v) => {
                if (v && v.isValid()) setDate(v.format(DATE_KEY_FORMAT));
              }}
              slotProps={{ textField: { size: 'small', sx: { width: { sm: 200 } } } }}
            />
          </Stack>
          <Typography variant="subtitle1" sx={{ mt: 2, display: { md: 'none' } }}>
            {formatDateKeyLong(date)}
          </Typography>
        </CardContent>
      </Card>

      {dayOff && (
        <Alert severity="info" sx={{ mb: 2 }}>
          Выходной день клиники.
        </Alert>
      )}

      {query.isPending && (
        <Stack spacing={1.5}>
          {Array.from({ length: 4 }, (_, i) => (
            <Skeleton key={i} variant="rounded" height={96} />
          ))}
        </Stack>
      )}
      {query.isError && <ErrorAlert error={query.error} onRetry={() => void query.refetch()} />}
      {query.isSuccess && query.data.appointments.length === 0 && (
        <Card>
          <EmptyState
            icon={<EventAvailableRoundedIcon />}
            title="На этот день записей нет"
            description="Свободный день — или выберите другую дату."
          />
        </Card>
      )}
      {query.isSuccess && query.data.appointments.length > 0 && (
        <Stack component="ol" spacing={1.5} sx={{ listStyle: 'none', p: 0, m: 0 }} aria-label="Приёмы за день">
          {query.data.appointments.map((a) => (
            <AgendaItem key={a.id} appointment={a} tz={tz} />
          ))}
        </Stack>
      )}
    </>
  );
}

function AgendaItem({ appointment: a, tz }: { appointment: Appointment; tz: string }) {
  const muted = a.status === 'cancelled' || a.status === 'no_show';
  return (
    <li>
      <Card sx={{ opacity: muted ? 0.7 : 1 }}>
        <CardContent sx={{ display: 'flex', gap: { xs: 2, sm: 3 }, alignItems: 'flex-start' }}>
          <Box
            sx={{
              width: { xs: 64, sm: 80 },
              flexShrink: 0,
              textAlign: 'center',
              py: 1,
              borderRadius: 3,
              bgcolor: (t) => t.palette.m3.primaryContainer,
              color: (t) => t.palette.m3.onPrimaryContainer,
              fontVariantNumeric: 'tabular-nums',
            }}
          >
            <Typography variant="h6" component="p" sx={{ lineHeight: 1.2 }}>
              {formatTime(a.starts_at, tz)}
            </Typography>
            <Typography variant="caption">до {formatTime(a.ends_at, tz)}</Typography>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 1 }}>
              <Typography
                component={RouterLink}
                to={`/appointments/${a.id}`}
                variant="subtitle1"
                sx={{ fontWeight: 600, color: 'inherit', textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}
              >
                {a.patient.full_name}
              </Typography>
              <StatusChip status={a.status} />
            </Stack>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 1.5, wordBreak: 'break-word' }}>
              {a.reason || 'Причина не указана'}
            </Typography>
            <AppointmentActions appointment={a} size="small" />
          </Box>
        </CardContent>
      </Card>
    </li>
  );
}
