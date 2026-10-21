import { useState } from 'react';
import {
  Avatar,
  Card,
  CardActionArea,
  CardContent,
  Grid,
  Skeleton,
  TextField,
  Typography,
} from '@mui/material';
import CheckCircleRoundedIcon from '@mui/icons-material/CheckCircleRounded';
import PersonSearchRoundedIcon from '@mui/icons-material/PersonSearchRounded';
import type { PersonRef } from '../../api/types';
import { EmptyState } from '../../components/EmptyState';
import { ErrorAlert } from '../../components/ErrorAlert';
import { useDoctors } from '../../hooks/useClinic';
import { initials } from '../../lib/labels';

interface Props {
  value: PersonRef | null;
  onChange: (doctor: PersonRef) => void;
}

export function DoctorStep({ value, onChange }: Props) {
  const doctors = useDoctors();
  const [search, setSearch] = useState('');

  if (doctors.isError) {
    return <ErrorAlert error={doctors.error} onRetry={() => void doctors.refetch()} />;
  }
  if (doctors.isPending) {
    return (
      <Grid container spacing={2}>
        {Array.from({ length: 4 }, (_, i) => (
          <Grid key={i} size={{ xs: 12, sm: 6, md: 4 }}>
            <Skeleton variant="rounded" height={88} />
          </Grid>
        ))}
      </Grid>
    );
  }
  if (doctors.data.length === 0) {
    return (
      <EmptyState
        icon={<PersonSearchRoundedIcon />}
        title="Нет доступных врачей"
        description="Сейчас нет врачей, принимающих записи. Попробуйте позже."
      />
    );
  }

  const q = search.trim().toLowerCase();
  const filtered = q ? doctors.data.filter((d) => d.full_name.toLowerCase().includes(q)) : doctors.data;

  return (
    <>
      {doctors.data.length > 6 && (
        <TextField
          label="Поиск врача"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          sx={{ mb: 2, maxWidth: 400 }}
        />
      )}
      <Grid container spacing={2} role="radiogroup" aria-label="Врач">
        {filtered.map((d) => {
          const selected = value?.id === d.id;
          return (
            <Grid key={d.id} size={{ xs: 12, sm: 6, md: 4 }}>
              <Card
                sx={{
                  borderColor: selected ? 'primary.main' : undefined,
                  borderWidth: selected ? 2 : 1,
                  bgcolor: selected ? (t) => t.palette.m3.primaryContainer : undefined,
                }}
              >
                <CardActionArea role="radio" aria-checked={selected} onClick={() => onChange(d)}>
                  <CardContent sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                    <Avatar sx={{ bgcolor: selected ? 'primary.main' : 'secondary.main' }}>
                      {initials(d.full_name)}
                    </Avatar>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                        {d.full_name}
                      </Typography>
                      <Typography variant="body2" color="text.secondary">
                        Стоматолог
                      </Typography>
                    </div>
                    {selected && <CheckCircleRoundedIcon color="primary" aria-hidden />}
                  </CardContent>
                </CardActionArea>
              </Card>
            </Grid>
          );
        })}
        {filtered.length === 0 && (
          <Grid size={12}>
            <Typography color="text.secondary">Ничего не найдено</Typography>
          </Grid>
        )}
      </Grid>
    </>
  );
}
