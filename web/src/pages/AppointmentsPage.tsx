import { Box, Button, Card, Skeleton, Stack, Tab, Tabs } from '@mui/material';
import EventNoteRoundedIcon from '@mui/icons-material/EventNoteRounded';
import AddRoundedIcon from '@mui/icons-material/AddRounded';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { Link as RouterLink, useNavigate, useSearchParams } from 'react-router-dom';
import { listAppointments } from '../api/appointments';
import { queryKeys } from '../api/queryKeys';
import type { AppointmentListParams, AppointmentStatus } from '../api/types';
import { useCurrentUser } from '../auth/authContext';
import { AppointmentListView } from '../components/appointments/AppointmentListView';
import { ClinicTimeNote } from '../components/ClinicTimeNote';
import { EmptyState } from '../components/EmptyState';
import { ErrorAlert } from '../components/ErrorAlert';
import { PageHeader } from '../components/PageHeader';
import { Pager } from '../components/Pager';
import { useClinicTz } from '../hooks/useClinic';
import { appointmentStatuses } from '../lib/labels';
import { nowRFC3339Minute } from '../lib/time';
import { AdminFilters } from './appointments/AdminFilters';
import { StatusFilter } from './appointments/StatusFilter';

const PAGE_SIZE = 20;

type TabId = 'upcoming' | 'past';

export function AppointmentsPage() {
  const user = useCurrentUser();
  const isAdmin = user.role === 'admin';
  const tz = useClinicTz();
  const navigate = useNavigate();
  const [sp, setSp] = useSearchParams();

  const tab: TabId = sp.get('tab') === 'past' ? 'past' : 'upcoming';
  const rawStatus = sp.get('status');
  const status = appointmentStatuses.includes(rawStatus as AppointmentStatus)
    ? (rawStatus as AppointmentStatus)
    : null;
  const doctorId = isAdmin ? sp.get('doctor') : null;
  const patientId = isAdmin ? sp.get('patient') : null;
  const page = Math.max(0, Number(sp.get('page')) || 0);

  const update = (patch: Record<string, string | null>) => {
    const next = new URLSearchParams(sp);
    for (const [key, value] of Object.entries(patch)) {
      if (value === null) next.delete(key);
      else next.set(key, value);
    }
    if (!('page' in patch)) next.delete('page');
    setSp(next, { replace: true });
  };

  const now = nowRFC3339Minute();
  const params: AppointmentListParams = {
    status: status ?? undefined,
    doctor_id: doctorId ?? undefined,
    patient_id: patientId ?? undefined,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
    ...(tab === 'upcoming' ? { from: now, order: 'asc' } : { to: now, order: 'desc' }),
  };

  const query = useQuery({
    queryKey: queryKeys.appointmentList(params),
    queryFn: () => listAppointments(params),
    placeholderData: keepPreviousData,
  });

  const items = query.data?.appointments ?? [];
  const canBook = user.role === 'patient' || isAdmin;

  return (
    <>
      <PageHeader
        title={isAdmin ? 'Все записи' : 'Мои записи'}
        subtitle={<ClinicTimeNote />}
        actions={
          canBook ? (
            <Button component={RouterLink} to="/book" variant="contained" startIcon={<AddRoundedIcon />}>
              {isAdmin ? 'Новая запись' : 'Записаться'}
            </Button>
          ) : undefined
        }
      />

      <Card sx={{ mb: 2 }}>
        <Tabs
          value={tab}
          onChange={(_, v: TabId) => update({ tab: v === 'upcoming' ? null : v })}
          aria-label="Период"
          sx={{ px: 2, borderBottom: 1, borderColor: 'divider' }}
        >
          <Tab value="upcoming" label="Предстоящие" />
          <Tab value="past" label="Прошедшие" />
        </Tabs>
        <Stack spacing={2} sx={{ p: 2 }}>
          <StatusFilter value={status} onChange={(s) => update({ status: s })} />
          {isAdmin && (
            <AdminFilters
              doctorId={doctorId}
              patientId={patientId}
              onDoctorChange={(id) => update({ doctor: id })}
              onPatientChange={(id) => update({ patient: id })}
            />
          )}
        </Stack>
      </Card>

      {query.isPending && (
        <Stack spacing={1}>
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} variant="rounded" height={56} />
          ))}
        </Stack>
      )}
      {query.isError && <ErrorAlert error={query.error} onRetry={() => void query.refetch()} />}
      {query.isSuccess && items.length === 0 && (
        <Card>
          <EmptyState
            icon={<EventNoteRoundedIcon />}
            title={tab === 'upcoming' ? 'Предстоящих записей нет' : 'Прошедших записей нет'}
            description={
              status || doctorId || patientId ? 'Попробуйте изменить фильтры.' : undefined
            }
            action={
              canBook && tab === 'upcoming' ? (
                <Button component={RouterLink} to="/book" variant="tonal">
                  {isAdmin ? 'Создать запись' : 'Записаться на приём'}
                </Button>
              ) : undefined
            }
          />
        </Card>
      )}
      {query.isSuccess && items.length > 0 && (
        <Box sx={{ opacity: query.isPlaceholderData ? 0.6 : 1, transition: 'opacity 150ms' }}>
          <AppointmentListView
            appointments={items}
            tz={tz}
            showDoctor={user.role !== 'doctor'}
            showPatient={user.role !== 'patient'}
            onOpen={(a) => navigate(`/appointments/${a.id}`)}
          />
        </Box>
      )}
      <Pager
        page={page}
        hasNext={items.length === PAGE_SIZE}
        disabled={query.isFetching}
        onChange={(p) => update({ page: p === 0 ? null : String(p) })}
      />
    </>
  );
}
