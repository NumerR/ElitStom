import { useState } from 'react';
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import { useQueryClient } from '@tanstack/react-query';
import type { Appointment, Slot } from '../../api/types';
import { queryKeys } from '../../api/queryKeys';
import { useClinicSchedule } from '../../hooks/useClinic';
import { useAppointmentActions } from '../../hooks/useAppointmentMutations';
import { errorMessage, isApiError } from '../../lib/errors';
import { durationMinutes, formatDateLong, formatTimeRange, DEFAULT_CLINIC_TZ } from '../../lib/time';
import { SlotPicker } from '../SlotPicker';
import { useNotify } from '../notifyContext';

interface Props {
  appointment: Appointment;
  open: boolean;
  onClose: () => void;
}

export function RescheduleDialog({ appointment, open, onClose }: Props) {
  const theme = useTheme();
  const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
  const notify = useNotify();
  const queryClient = useQueryClient();
  const schedule = useClinicSchedule();
  const { reschedule } = useAppointmentActions();
  const [date, setDate] = useState<string | null>(null);
  const [slot, setSlot] = useState<Slot | null>(null);
  const tz = schedule.data?.timezone ?? DEFAULT_CLINIC_TZ;

  const submit = () => {
    if (!slot) return;
    const duration = durationMinutes(appointment.starts_at, appointment.ends_at);
    const keepDuration = schedule.data && duration !== schedule.data.slot_minutes;
    reschedule.mutate(
      {
        id: appointment.id,
        input: { starts_at: slot.starts_at, ...(keepDuration ? { duration_minutes: duration } : {}) },
      },
      {
        onSuccess: () => {
          notify('Запись перенесена');
          onClose();
        },
        onError: (err) => {
          if (isApiError(err, 'conflict')) {
            notify('Это время уже занято, выберите другое', 'error');
            setSlot(null);
            void queryClient.invalidateQueries({ queryKey: queryKeys.slotsAll });
          } else {
            notify(errorMessage(err), 'error');
          }
        },
      },
    );
  };

  return (
    <Dialog
      open={open}
      onClose={reschedule.isPending ? undefined : onClose}
      fullScreen={fullScreen}
      maxWidth="md"
      fullWidth
      aria-labelledby="reschedule-title"
    >
      <DialogTitle id="reschedule-title">Перенос записи</DialogTitle>
      <DialogContent>
        <Alert severity="info" sx={{ mb: 3 }}>
          Сейчас: {formatDateLong(appointment.starts_at, tz)},{' '}
          {formatTimeRange(appointment.starts_at, appointment.ends_at, tz)} · {appointment.doctor.full_name}
        </Alert>
        <SlotPicker
          doctorId={appointment.doctor.id}
          date={date}
          slot={slot}
          onChange={(d, s) => {
            setDate(d);
            setSlot(s);
          }}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={reschedule.isPending}>
          Отмена
        </Button>
        <Button variant="contained" onClick={submit} disabled={!slot} loading={reschedule.isPending}>
          Перенести
        </Button>
      </DialogActions>
    </Dialog>
  );
}
