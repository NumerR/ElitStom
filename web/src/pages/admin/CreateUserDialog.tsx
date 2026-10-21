import { useState, type ChangeEvent, type FormEvent } from 'react';
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  Stack,
  TextField,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import type { Role } from '../../api/types';
import { PasswordField } from '../../components/PasswordField';
import { useNotify } from '../../components/notifyContext';
import { errorMessage, mapFieldErrors } from '../../lib/errors';
import { roleLabels, roles } from '../../lib/labels';
import {
  PASSWORD_MIN_CHARS,
  hasErrors,
  validateEmail,
  validateFullName,
  validatePassword,
  validatePhone,
  type FieldErrors,
} from '../../lib/validation';
import { useCreateUser } from './useUserAdminMutations';

type Field = 'email' | 'full_name' | 'phone' | 'role' | 'password';
const FIELDS = ['email', 'full_name', 'phone', 'role', 'password'] as const;

const empty = { email: '', full_name: '', phone: '', role: 'doctor' as Role, password: '' };

interface Props {
  open: boolean;
  onClose: () => void;
}

export function CreateUserDialog({ open, onClose }: Props) {
  const theme = useTheme();
  const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
  const notify = useNotify();
  const mutation = useCreateUser();
  const [values, setValues] = useState(empty);
  const [errors, setErrors] = useState<FieldErrors<Field>>({});
  const [formError, setFormError] = useState<string | null>(null);

  const set = (field: Field) => (e: ChangeEvent<HTMLInputElement>) =>
    setValues((v) => ({ ...v, [field]: e.target.value }));

  const close = () => {
    if (mutation.isPending) return;
    setValues(empty);
    setErrors({});
    setFormError(null);
    onClose();
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const next: FieldErrors<Field> = {
      email: validateEmail(values.email),
      full_name: validateFullName(values.full_name),
      phone: validatePhone(values.phone),
      password: validatePassword(values.password),
    };
    setErrors(next);
    setFormError(null);
    if (hasErrors(next)) return;

    mutation.mutate(
      {
        email: values.email.trim(),
        full_name: values.full_name.trim(),
        phone: values.phone.trim(),
        role: values.role,
        password: values.password,
      },
      {
        onSuccess: (user) => {
          notify(`Пользователь ${user.full_name} создан`);
          setValues(empty);
          onClose();
        },
        onError: (err) => {
          const mapped = mapFieldErrors(err, FIELDS);
          setErrors(mapped.fields);
          if (mapped.rest.length) setFormError(mapped.rest.join('; '));
          else if (!Object.keys(mapped.fields).length) {
            setFormError(errorMessage(err, { already_exists: 'Пользователь с таким email уже существует' }));
          }
        },
      },
    );
  };

  return (
    <Dialog open={open} onClose={close} fullScreen={fullScreen} maxWidth="sm" fullWidth aria-labelledby="create-user-title">
      <form onSubmit={handleSubmit} noValidate>
        <DialogTitle id="create-user-title">Новый пользователь</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ pt: 1 }}>
            {formError && <Alert severity="error">{formError}</Alert>}
            <TextField
              label="Email"
              type="email"
              autoComplete="off"
              value={values.email}
              onChange={set('email')}
              error={!!errors.email}
              helperText={errors.email}
              required
              autoFocus
            />
            <TextField
              label="ФИО"
              value={values.full_name}
              onChange={set('full_name')}
              error={!!errors.full_name}
              helperText={errors.full_name}
              required
            />
            <TextField
              label="Телефон"
              type="tel"
              value={values.phone}
              onChange={set('phone')}
              error={!!errors.phone}
              helperText={errors.phone ?? 'Необязательно'}
            />
            <TextField
              select
              label="Роль"
              value={values.role}
              onChange={set('role')}
              error={!!errors.role}
              helperText={errors.role}
              required
            >
              {roles.map((r) => (
                <MenuItem key={r} value={r}>
                  {roleLabels[r]}
                </MenuItem>
              ))}
            </TextField>
            <PasswordField
              label="Пароль"
              autoComplete="new-password"
              value={values.password}
              onChange={set('password')}
              error={!!errors.password}
              helperText={errors.password ?? `Не менее ${PASSWORD_MIN_CHARS} символов. Передайте пароль пользователю безопасным способом`}
              required
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={close} disabled={mutation.isPending}>
            Отмена
          </Button>
          <Button type="submit" variant="contained" loading={mutation.isPending}>
            Создать
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
