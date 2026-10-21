import type { ReactNode } from 'react';
import { Divider, Stack, Typography } from '@mui/material';
import type { PersonRef, Slot, User } from '../../api/types';
import { ClinicTimeNote } from '../../components/ClinicTimeNote';
import { durationMinutes, formatDateLong, formatDuration, formatTimeRange } from '../../lib/time';

interface Props {
  patient: User | null;
  doctor: PersonRef;
  slot: Slot;
  reason: string;
  tz: string;
}

export function ReviewStep({ patient, doctor, slot, reason, tz }: Props) {
  const rows: [string, ReactNode][] = [
    ...(patient ? ([['Пациент', `${patient.full_name} (${patient.email})`]] as [string, ReactNode][]) : []),
    ['Врач', doctor.full_name],
    ['Дата', formatDateLong(slot.starts_at, tz)],
    ['Время', formatTimeRange(slot.starts_at, slot.ends_at, tz)],
    ['Длительность', formatDuration(durationMinutes(slot.starts_at, slot.ends_at))],
    ['Причина обращения', reason.trim() || '—'],
  ];

  return (
    <Stack spacing={2} sx={{ maxWidth: 640 }}>
      <Stack component="dl" divider={<Divider flexItem />} spacing={1.5} sx={{ m: 0 }}>
        {rows.map(([label, value]) => (
          <Stack key={label} direction={{ xs: 'column', sm: 'row' }} spacing={{ xs: 0.25, sm: 2 }}>
            <Typography component="dt" color="text.secondary" sx={{ width: { sm: 200 }, flexShrink: 0 }}>
              {label}
            </Typography>
            <Typography component="dd" sx={{ m: 0, fontWeight: 500, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
              {value}
            </Typography>
          </Stack>
        ))}
      </Stack>
      <ClinicTimeNote />
    </Stack>
  );
}
