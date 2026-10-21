import { useState } from 'react';
import { Button, Stack } from '@mui/material';
import CheckCircleRoundedIcon from '@mui/icons-material/CheckCircleRounded';
import TaskAltRoundedIcon from '@mui/icons-material/TaskAltRounded';
import PersonOffRoundedIcon from '@mui/icons-material/PersonOffRounded';
import EventRepeatRoundedIcon from '@mui/icons-material/EventRepeatRounded';
import CancelRoundedIcon from '@mui/icons-material/CancelRounded';
import type { UseMutationResult } from '@tanstack/react-query';
import type { Appointment } from '../../api/types';
import { useCurrentUser } from '../../auth/authContext';
import { useAppointmentActions } from '../../hooks/useAppointmentMutations';
import { errorMessage } from '../../lib/errors';
import { appointmentPermissions, hasAnyPermission } from '../../lib/permissions';
import { ConfirmDialog } from '../ConfirmDialog';
import { useNotify } from '../notifyContext';
import { CancelDialog } from './CancelDialog';
import { RescheduleDialog } from './RescheduleDialog';

interface Props {
  appointment: Appointment;
  size?: 'small' | 'medium';
}

type DialogKind = 'cancel' | 'noShow' | 'reschedule' | null;

const transitionErrors = { conflict: 'Статус записи уже изменился — данные обновлены' };

export function AppointmentActions({ appointment, size = 'medium' }: Props) {
  const user = useCurrentUser();
  const notify = useNotify();
  const actions = useAppointmentActions();
  const [dialog, setDialog] = useState<DialogKind>(null);
  const perms = appointmentPermissions(appointment, user);

  if (!hasAnyPermission(perms)) return null;

  const busy =
    actions.confirm.isPending ||
    actions.complete.isPending ||
    actions.noShow.isPending ||
    actions.cancel.isPending;

  const run = <V,>(
    mutation: UseMutationResult<Appointment, Error, V>,
    variables: V,
    success: string,
  ) => {
    mutation.mutate(variables, {
      onSuccess: () => {
        notify(success);
        setDialog(null);
      },
      onError: (err) => {
        notify(errorMessage(err, transitionErrors), 'error');
        setDialog(null);
      },
    });
  };

  const id = appointment.id;

  return (
    <>
      <Stack direction="row" useFlexGap sx={{ flexWrap: 'wrap', gap: 1 }}>
        {perms.confirm && (
          <Button
            variant="tonal"
            color="primary"
            size={size}
            startIcon={<CheckCircleRoundedIcon />}
            loading={actions.confirm.isPending}
            disabled={busy}
            onClick={() => run(actions.confirm, id, 'Запись подтверждена')}
          >
            Подтвердить
          </Button>
        )}
        {perms.complete && (
          <Button
            variant="tonal"
            color="success"
            size={size}
            startIcon={<TaskAltRoundedIcon />}
            loading={actions.complete.isPending}
            disabled={busy}
            onClick={() => run(actions.complete, id, 'Приём завершён')}
          >
            Завершить
          </Button>
        )}
        {perms.noShow && (
          <Button
            variant="tonal"
            color="warning"
            size={size}
            startIcon={<PersonOffRoundedIcon />}
            disabled={busy}
            onClick={() => setDialog('noShow')}
          >
            Неявка
          </Button>
        )}
        {perms.reschedule && (
          <Button
            variant="outlined"
            size={size}
            startIcon={<EventRepeatRoundedIcon />}
            disabled={busy}
            onClick={() => setDialog('reschedule')}
          >
            Перенести
          </Button>
        )}
        {perms.cancel && (
          <Button
            variant="text"
            color="error"
            size={size}
            startIcon={<CancelRoundedIcon />}
            disabled={busy}
            onClick={() => setDialog('cancel')}
          >
            Отменить
          </Button>
        )}
      </Stack>

      {dialog === 'cancel' && (
        <CancelDialog
          open
          loading={actions.cancel.isPending}
          onClose={() => setDialog(null)}
          onConfirm={(reason) => run(actions.cancel, { id, reason }, 'Запись отменена')}
        />
      )}
      <ConfirmDialog
        open={dialog === 'noShow'}
        title="Отметить неявку?"
        description={`Пациент ${appointment.patient.full_name} не пришёл на приём.`}
        confirmLabel="Отметить неявку"
        destructive
        loading={actions.noShow.isPending}
        onClose={() => setDialog(null)}
        onConfirm={() => run(actions.noShow, id, 'Отмечена неявка')}
      />
      {dialog === 'reschedule' && (
        <RescheduleDialog appointment={appointment} open onClose={() => setDialog(null)} />
      )}
    </>
  );
}
