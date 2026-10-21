import { useState, type FormEvent, type ReactNode } from 'react';
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Chip,
  Divider,
  Grid,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useMutation } from '@tanstack/react-query';
import { updateMe } from '../api/users';
import type { User } from '../api/types';
import { useAuth, useCurrentUser } from '../auth/authContext';
import { PageHeader } from '../components/PageHeader';
import { RoleChip } from '../components/RoleChip';
import { useNotify } from '../components/notifyContext';
import { errorMessage, mapFieldErrors } from '../lib/errors';
import { initials, userStatusLabels } from '../lib/labels';
import { formatLocalDate } from '../lib/time';
import { hasErrors, validateFullName, validatePhone, type FieldErrors } from '../lib/validation';

type Field = 'full_name' | 'phone';
const FIELDS = ['full_name', 'phone'] as const;

export function ProfilePage() {
  const user = useCurrentUser();
  return (
    <>
      <PageHeader title="Профиль" subtitle="Личные данные и учётная запись" />
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 5 }}>
          <AccountCard user={user} />
        </Grid>
        <Grid size={{ xs: 12, md: 7 }}>
          <ProfileForm key={user.updated_at} user={user} />
        </Grid>
      </Grid>
    </>
  );
}

function AccountCard({ user }: { user: User }) {
  const rows: [string, ReactNode][] = [
    ['Email', user.email],
    ['Роль', <RoleChip key="role" role={user.role} />],
    [
      'Статус',
      <Chip
        key="status"
        size="small"
        label={userStatusLabels[user.status]}
        color={user.status === 'active' ? 'success' : 'default'}
      />,
    ],
    ['Дата регистрации', formatLocalDate(user.created_at)],
  ];
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2} sx={{ alignItems: 'center', textAlign: 'center', py: 2 }}>
          <Avatar sx={{ width: 80, height: 80, fontSize: 30, bgcolor: 'primary.main' }}>
            {initials(user.full_name)}
          </Avatar>
          <Box>
            <Typography variant="h6">{user.full_name}</Typography>
            <Typography color="text.secondary">{user.phone || 'Телефон не указан'}</Typography>
          </Box>
        </Stack>
        <Divider sx={{ my: 2 }} />
        <Stack component="dl" spacing={1.5} sx={{ m: 0 }}>
          {rows.map(([label, value]) => (
            <Stack key={label} direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center', gap: 2 }}>
              <Typography component="dt" color="text.secondary" variant="body2">
                {label}
              </Typography>
              <Typography component="dd" variant="body2" sx={{ m: 0, textAlign: 'right', wordBreak: 'break-all' }}>
                {value}
              </Typography>
            </Stack>
          ))}
        </Stack>
      </CardContent>
    </Card>
  );
}

function ProfileForm({ user }: { user: User }) {
  const { setUser } = useAuth();
  const notify = useNotify();
  const [fullName, setFullName] = useState(user.full_name);
  const [phone, setPhone] = useState(user.phone);
  const [errors, setErrors] = useState<FieldErrors<Field>>({});
  const [formError, setFormError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: updateMe,
    onSuccess: (updated) => {
      setUser(updated);
      notify('Профиль обновлён');
    },
    onError: (err) => {
      const mapped = mapFieldErrors(err, FIELDS);
      setErrors(mapped.fields);
      setFormError(mapped.rest.length ? mapped.rest.join('; ') : Object.keys(mapped.fields).length ? null : errorMessage(err));
    },
  });

  const dirty = fullName.trim() !== user.full_name || phone.trim() !== user.phone;

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const next = { full_name: validateFullName(fullName), phone: validatePhone(phone) };
    setErrors(next);
    setFormError(null);
    if (hasErrors(next)) return;
    mutation.mutate({ full_name: fullName.trim(), phone: phone.trim() });
  };

  return (
    <Card>
      <CardHeader title="Личные данные" subheader="Эти данные видят врачи и администраторы клиники" />
      <CardContent component="form" onSubmit={handleSubmit} noValidate sx={{ pt: 0 }}>
        <Stack spacing={2}>
          {formError && <Alert severity="error">{formError}</Alert>}
          <TextField
            label="ФИО"
            autoComplete="name"
            value={fullName}
            onChange={(e) => setFullName(e.target.value)}
            error={!!errors.full_name}
            helperText={errors.full_name}
            required
          />
          <TextField
            label="Телефон"
            type="tel"
            autoComplete="tel"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            error={!!errors.phone}
            helperText={errors.phone ?? 'Необязательно'}
          />
          <Stack direction="row" spacing={1} sx={{ justifyContent: 'flex-end' }}>
            <Button
              disabled={!dirty || mutation.isPending}
              onClick={() => {
                setFullName(user.full_name);
                setPhone(user.phone);
                setErrors({});
                setFormError(null);
              }}
            >
              Сбросить
            </Button>
            <Button type="submit" variant="contained" disabled={!dirty} loading={mutation.isPending}>
              Сохранить
            </Button>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}
