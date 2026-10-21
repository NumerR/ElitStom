import { useState, type FormEvent } from 'react';
import { Alert, Button, Link, Stack, TextField, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { useAuth } from '../auth/authContext';
import { AuthLayout } from '../components/AuthLayout';
import { PasswordField } from '../components/PasswordField';
import { errorMessage, mapFieldErrors } from '../lib/errors';
import { validateEmail, type FieldErrors } from '../lib/validation';

type Field = 'email' | 'password';
const FIELDS = ['email', 'password'] as const;

export function LoginPage() {
  const { login } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [errors, setErrors] = useState<FieldErrors<Field>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const next: FieldErrors<Field> = {
      email: validateEmail(email),
      password: password ? undefined : 'Введите пароль',
    };
    setErrors(next);
    setFormError(null);
    if (next.email || next.password) return;

    setSubmitting(true);
    try {
      await login({ email: email.trim(), password });
    } catch (err) {
      const mapped = mapFieldErrors(err, FIELDS);
      setErrors(mapped.fields);
      setFormError(
        mapped.rest.length
          ? mapped.rest.join('; ')
          : Object.keys(mapped.fields).length
            ? null
            : errorMessage(err, { unauthorized: 'Неверный email или пароль' }),
      );
      setSubmitting(false);
    }
  };

  return (
    <AuthLayout
      title="Вход"
      subtitle="Войдите, чтобы управлять записями на приём"
      footer={
        <Typography variant="body2" color="text.secondary">
          Нет учётной записи?{' '}
          <Link component={RouterLink} to="/register" sx={{ fontWeight: 600 }}>
            Зарегистрироваться
          </Link>
        </Typography>
      }
    >
      <Stack component="form" spacing={2} onSubmit={handleSubmit} noValidate>
        {formError && (
          <Alert severity="error" role="alert">
            {formError}
          </Alert>
        )}
        <TextField
          label="Email"
          type="email"
          autoComplete="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          error={!!errors.email}
          helperText={errors.email}
          required
          autoFocus
        />
        <PasswordField
          label="Пароль"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          error={!!errors.password}
          helperText={errors.password}
          required
        />
        <Button type="submit" variant="contained" size="large" loading={submitting} fullWidth>
          Войти
        </Button>
      </Stack>
    </AuthLayout>
  );
}
