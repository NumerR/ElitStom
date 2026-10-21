import { Alert, Box, Chip, Grid, Skeleton, Stack, Typography } from '@mui/material';
import { DateCalendar } from '@mui/x-date-pickers/DateCalendar';
import EventBusyRoundedIcon from '@mui/icons-material/EventBusyRounded';
import type { ClinicSchedule, Slot } from '../api/types';
import { useClinicSchedule, useSlots } from '../hooks/useClinic';
import {
  DATE_KEY_FORMAT,
  bookingWindow,
  dayjs,
  firstBookableDay,
  formatDateKeyLong,
  formatTime,
  inTz,
  isBookableDay,
  tzLabel,
} from '../lib/time';
import { EmptyState } from './EmptyState';
import { ErrorAlert } from './ErrorAlert';

interface Props {
  doctorId: string;
  date: string | null;
  slot: Slot | null;
  onChange: (date: string, slot: Slot | null) => void;
}

export function SlotPicker(props: Props) {
  const schedule = useClinicSchedule();

  if (schedule.isPending) {
    return <Skeleton variant="rounded" height={340} />;
  }
  if (schedule.isError) {
    return <ErrorAlert error={schedule.error} onRetry={() => void schedule.refetch()} />;
  }
  return <SlotPickerInner {...props} schedule={schedule.data} />;
}

function SlotPickerInner({ doctorId, date, slot, onChange, schedule }: Props & { schedule: ClinicSchedule }) {
  const effectiveDate = date ?? firstBookableDay(schedule);
  const range = bookingWindow(schedule);

  if (!effectiveDate) {
    return <Alert severity="info">В ближайшие дни клиника не принимает записи.</Alert>;
  }

  return (
    <Grid container spacing={3}>
      <Grid size={{ xs: 12, md: 6 }}>
        <Box
          sx={{
            borderRadius: 4,
            bgcolor: (t) => t.palette.m3.surfaceContainer,
            display: 'flex',
            justifyContent: 'center',
          }}
        >
          <DateCalendar
            value={dayjs(effectiveDate)}
            onChange={(value) => {
              if (value) onChange(value.format(DATE_KEY_FORMAT), null);
            }}
            minDate={dayjs(range.first)}
            maxDate={dayjs(range.last)}
            shouldDisableDate={(day) => !isBookableDay(day.format(DATE_KEY_FORMAT), schedule)}
            views={['day']}
            aria-label="Выбор даты приёма"
          />
        </Box>
      </Grid>
      <Grid size={{ xs: 12, md: 6 }}>
        <Typography variant="subtitle1" component="h3">
          {formatDateKeyLong(effectiveDate)}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 2 }}>
          Свободное время, {tzLabel(schedule.timezone)}
        </Typography>
        <SlotGrid
          doctorId={doctorId}
          date={effectiveDate}
          selected={slot}
          tz={schedule.timezone}
          onSelect={(s) => onChange(effectiveDate, s)}
        />
      </Grid>
    </Grid>
  );
}

interface SlotGridProps {
  doctorId: string;
  date: string;
  selected: Slot | null;
  tz: string;
  onSelect: (slot: Slot) => void;
}

const periods = [
  { label: 'Утро', test: (h: number) => h < 12 },
  { label: 'День', test: (h: number) => h >= 12 && h < 17 },
  { label: 'Вечер', test: (h: number) => h >= 17 },
];

function SlotGrid({ doctorId, date, selected, tz, onSelect }: SlotGridProps) {
  const slots = useSlots(doctorId, date);

  if (slots.isPending) {
    return (
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }} aria-busy="true" aria-label="Загрузка времени">
        {Array.from({ length: 12 }, (_, i) => (
          <Skeleton key={i} variant="rounded" width={72} height={32} sx={{ borderRadius: 999 }} />
        ))}
      </Box>
    );
  }
  if (slots.isError) {
    return <ErrorAlert error={slots.error} onRetry={() => void slots.refetch()} />;
  }
  if (slots.data.length === 0) {
    return (
      <EmptyState
        dense
        icon={<EventBusyRoundedIcon />}
        title="Нет свободного времени"
        description="Выберите другую дату в календаре."
      />
    );
  }

  return (
    <Stack spacing={2} role="group" aria-label="Свободное время">
      {periods.map((period) => {
        const items = slots.data.filter((s) => period.test(inTz(s.starts_at, tz).hour()));
        if (items.length === 0) return null;
        return (
          <Box key={period.label}>
            <Typography variant="overline" color="text.secondary">
              {period.label}
            </Typography>
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, mt: 0.5 }}>
              {items.map((s) => {
                const isSelected = selected?.starts_at === s.starts_at;
                return (
                  <Chip
                    key={s.starts_at}
                    label={formatTime(s.starts_at, tz)}
                    clickable
                    color={isSelected ? 'primary' : 'default'}
                    variant={isSelected ? 'filled' : 'outlined'}
                    onClick={() => onSelect(s)}
                    aria-pressed={isSelected}
                    sx={{ minWidth: 72, fontVariantNumeric: 'tabular-nums' }}
                  />
                );
              })}
            </Box>
          </Box>
        );
      })}
    </Stack>
  );
}
