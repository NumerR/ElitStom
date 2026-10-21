import { Chip, type ChipProps } from '@mui/material';
import type { AppointmentStatus } from '../api/types';
import { appointmentStatusLabels } from '../lib/labels';

const colors: Record<AppointmentStatus, ChipProps['color']> = {
  scheduled: 'info',
  confirmed: 'primary',
  completed: 'success',
  cancelled: 'default',
  no_show: 'warning',
};

export function StatusChip({ status, size = 'small' }: { status: AppointmentStatus; size?: ChipProps['size'] }) {
  return (
    <Chip
      label={appointmentStatusLabels[status]}
      color={colors[status]}
      size={size}
      variant={status === 'cancelled' ? 'outlined' : 'filled'}
    />
  );
}
