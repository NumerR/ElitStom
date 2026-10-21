import { Autocomplete, Grid, TextField } from '@mui/material';
import { useDoctors } from '../../hooks/useClinic';
import { usePatients } from '../../hooks/usePatients';

interface Props {
  doctorId: string | null;
  patientId: string | null;
  onDoctorChange: (id: string | null) => void;
  onPatientChange: (id: string | null) => void;
}

export function AdminFilters({ doctorId, patientId, onDoctorChange, onPatientChange }: Props) {
  const doctors = useDoctors();
  const patients = usePatients();
  const doctor = doctors.data?.find((d) => d.id === doctorId) ?? null;
  const patient = patients.data?.find((p) => p.id === patientId) ?? null;

  return (
    <Grid container spacing={2}>
      <Grid size={{ xs: 12, sm: 6 }}>
        <Autocomplete
          options={doctors.data ?? []}
          loading={doctors.isPending}
          value={doctor}
          onChange={(_, v) => onDoctorChange(v?.id ?? null)}
          getOptionLabel={(d) => d.full_name}
          isOptionEqualToValue={(a, b) => a.id === b.id}
          noOptionsText="Врачи не найдены"
          loadingText="Загрузка…"
          renderInput={(params) => <TextField {...params} label="Врач" size="small" />}
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <Autocomplete
          options={patients.data ?? []}
          loading={patients.isPending}
          value={patient}
          onChange={(_, v) => onPatientChange(v?.id ?? null)}
          getOptionLabel={(p) => `${p.full_name} — ${p.email}`}
          isOptionEqualToValue={(a, b) => a.id === b.id}
          noOptionsText="Пациенты не найдены"
          loadingText="Загрузка…"
          renderInput={(params) => <TextField {...params} label="Пациент" size="small" />}
        />
      </Grid>
    </Grid>
  );
}
