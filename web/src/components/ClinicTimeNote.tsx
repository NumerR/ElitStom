import { Stack, Typography } from '@mui/material';
import ScheduleRoundedIcon from '@mui/icons-material/ScheduleRounded';
import { useClinicTz } from '../hooks/useClinic';
import { tzLabel } from '../lib/time';

export function ClinicTimeNote() {
  const tz = useClinicTz();
  return (
    <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', color: 'text.secondary' }}>
      <ScheduleRoundedIcon sx={{ fontSize: 16 }} aria-hidden />
      <Typography variant="caption">Указано {tzLabel(tz)}</Typography>
    </Stack>
  );
}
