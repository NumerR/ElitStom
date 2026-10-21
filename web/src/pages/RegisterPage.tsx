import { useState, type ChangeEvent, type FormEvent } from 'react';
import { Alert, Button, Link, Stack, TextField, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { useAuth } from '../auth/authContext';
import { AuthLayout } from '../components/AuthLayout';
import { PasswordField } from '../components/PasswordField';
import { errorMessage, mapFieldErrors } from '../lib/errors';
import {
  PASSWORD_MIN_CHARS,
  hasErrors,
  validateEmail,
  validateFullName,
  validatePassword,
  validatePhone,
  type FieldErrors,
} from '../lib/validation';

type Field = 'email' | 'full_name' | 'phone' | 'password' | 'confirm';
const SERVER_FIELDS = ['email', 'full_name', 'phone', 'password'] as const;

export function RegisterPage() {
  const { register } = useAuth();
  const [values, setValues] = useState({ email: '', full_name: '', phone: '', password: '', confirm: '' });
  const [errors, setErrors] = useState<FieldErrors<Field>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const set = (field: Field) => (e: ChangeEvent<HTMLInputElement>) =>
    setValues((v) => ({ ...v, [field]: e.target.value }));

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const next: FieldErrors<Field> = {
      email: validateEmail(values.email),
      full_name: validateFullName(values.full_name),
      phone: validatePhone(values.phone),
      password: validatePassword(values.password),
      confirm: values.confirm === values.password ? undefined : 'Пароли не совпадают',
    };
    setErrors(next);
    setFormError(null);
    if (hasErrors(next)) return;

    setSubmitting(true);
    try {
      await register({
        email: values.email.trim(),
        full_name: values.full_name.trim(),
        phone: values.phone.trim(),
        password: values.password,
      });
    } catch (err) {
      const mapped = mapFieldErrors(err, SERVER_FIELDS);
      setErrors(mapped.fields);
      if (mapped.rest.length) setFormError(mapped.rest.join('; '));
      else if (!Object.keys(mapped.fields).length) {
        setFormError(
          errorMessage(err, { already_exists: 'Пользователь с таким email уже зарегистрирован' }),
        );
      }
      setSubmitting(false);
    }
  };

  return (
    <AuthLayout
      title="Регистрация"
      subtitle="Создайте учётную запись пациента, чтобы записываться на приём онлайн"
      footer={
        <Typography variant="body2" color="text.secondary">
          Уже есть учётная запись?{' '}
          <Link component={RouterLink} to="/login" sx={{ fontWeight: 600 }}>
            Войти
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
          value={values.email}
          onChange={set('email')}
          error={!!errors.email}
          helperText={errors.email}
          required
          autoFocus
        />
        <TextField
          label="ФИО"
          autoComplete="name"
          value={values.full_name}
          onChange={set('full_name')}
          error={!!errors.full_name}
          helperText={errors.full_name}
          required
        />
        <TextField
          label="Телефон"
          type="tel"
          autoComplete="tel"
          placeholder="+7 (999) 123-45-67"
          value={values.phone}
          onChange={set('phone')}
          error={!!errors.phone}
          helperText={errors.phone ?? 'Необязательно'}
        />
        <PasswordField
          label="Пароль"
          autoComplete="new-password"
          value={values.password}
          onChange={set('password')}
          error={!!errors.password}
          helperText={errors.password ?? `Не менее ${PASSWORD_MIN_CHARS} символов`}
          required
        />
        <PasswordField
          label="Повторите пароль"
          autoComplete="new-password"
          value={values.confirm}
          onChange={set('confirm')}
          error={!!errors.confirm}
          helperText={errors.confirm}
          required
        />
        <Button type="submit" variant="contained" size="large" loading={submitting} fullWidth>
          Зарегистрироваться
        </Button>
      </Stack>
    </AuthLayout>
  );
}
