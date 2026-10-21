import { Chip, Stack } from '@mui/material';
import type { AppointmentStatus } from '../../api/types';
import { appointmentStatusLabels, appointmentStatuses } from '../../lib/labels';

interface Props {
  value: AppointmentStatus | null;
  onChange: (status: AppointmentStatus | null) => void;
}

export function StatusFilter({ value, onChange }: Props) {
  const options: (AppointmentStatus | null)[] = [null, ...appointmentStatuses];
  return (
    <Stack
      direction="row"
      useFlexGap
      role="group"
      aria-label="Фильтр по статусу"
      sx={{ flexWrap: 'wrap', gap: 1 }}
    >
      {options.map((s) => {
        const selected = value === s;
        return (
          <Chip
            key={s ?? 'all'}
            label={s ? appointmentStatusLabels[s] : 'Все статусы'}
            clickable
            onClick={() => onChange(s)}
            color={selected ? 'primary' : 'default'}
            variant={selected ? 'filled' : 'outlined'}
            aria-pressed={selected}
          />
        );
      })}
    </Stack>
  );
}
