import { Autocomplete, Box, Skeleton, TextField, Typography } from '@mui/material';
import type { User } from '../../api/types';
import { ErrorAlert } from '../../components/ErrorAlert';
import { usePatients } from '../../hooks/usePatients';

interface Props {
  value: User | null;
  onChange: (patient: User | null) => void;
}

export function PatientStep({ value, onChange }: Props) {
  const patients = usePatients();

  if (patients.isError) {
    return <ErrorAlert error={patients.error} onRetry={() => void patients.refetch()} />;
  }
  if (patients.isPending) return <Skeleton variant="rounded" height={56} />;

  return (
    <Box sx={{ maxWidth: 560 }}>
      <Autocomplete
        options={patients.data}
        value={value}
        onChange={(_, v) => onChange(v)}
        getOptionLabel={(p) => `${p.full_name} — ${p.email}`}
        isOptionEqualToValue={(a, b) => a.id === b.id}
        noOptionsText="Пациенты не найдены"
        renderOption={({ key, ...props }, p) => (
          <Box component="li" key={key} {...props} sx={{ display: 'block !important' }}>
            <Typography variant="body1">{p.full_name}</Typography>
            <Typography variant="body2" color="text.secondary">
              {p.email}
              {p.phone ? ` · ${p.phone}` : ''}
            </Typography>
          </Box>
        )}
        renderInput={(params) => (
          <TextField {...params} label="Пациент" placeholder="Начните вводить имя или email" autoFocus />
        )}
      />
      {patients.data.length === 200 && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
          Показаны первые 200 активных пациентов.
        </Typography>
      )}
    </Box>
  );
}
