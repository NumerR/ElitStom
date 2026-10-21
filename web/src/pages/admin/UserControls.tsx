import { useState } from 'react';
import { Chip, FormControlLabel, MenuItem, Select, Switch } from '@mui/material';
import type { Role, User } from '../../api/types';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { RoleChip } from '../../components/RoleChip';
import { useNotify } from '../../components/notifyContext';
import { errorMessage } from '../../lib/errors';
import { roleLabels, roles, userStatusLabels } from '../../lib/labels';
import { useSetUserRole, useSetUserStatus } from './useUserAdminMutations';

interface Props {
  user: User;
  isSelf: boolean;
}

export function UserRoleControl({ user, isSelf }: Props) {
  const notify = useNotify();
  const mutation = useSetUserRole();

  if (isSelf) return <RoleChip role={user.role} />;

  return (
    <Select
      size="small"
      value={user.role}
      disabled={mutation.isPending}
      onChange={(e) => {
        const role = e.target.value as Role;
        mutation.mutate(
          { id: user.id, role },
          {
            onSuccess: (u) => notify(`${u.full_name}: роль изменена на «${roleLabels[u.role]}»`),
            onError: (err) => notify(errorMessage(err), 'error'),
          },
        );
      }}
      inputProps={{ 'aria-label': `Роль пользователя ${user.full_name}` }}
      sx={{ minWidth: 170, borderRadius: 999 }}
    >
      {roles.map((r) => (
        <MenuItem key={r} value={r}>
          {roleLabels[r]}
        </MenuItem>
      ))}
    </Select>
  );
}

export function UserStatusControl({ user, isSelf }: Props) {
  const notify = useNotify();
  const mutation = useSetUserStatus();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const active = user.status === 'active';

  if (isSelf) {
    return <Chip size="small" color="success" label={userStatusLabels[user.status]} />;
  }

  const apply = (status: User['status']) =>
    mutation.mutate(
      { id: user.id, status },
      {
        onSuccess: (u) =>
          notify(u.status === 'active' ? `${u.full_name}: доступ восстановлен` : `${u.full_name}: доступ отключён`),
        onError: (err) => notify(errorMessage(err), 'error'),
        onSettled: () => setConfirmOpen(false),
      },
    );

  return (
    <>
      <FormControlLabel
        control={
          <Switch
            checked={active}
            disabled={mutation.isPending}
            onChange={(_, checked) => (checked ? apply('active') : setConfirmOpen(true))}
            slotProps={{ input: { 'aria-label': `Доступ пользователя ${user.full_name}` } }}
          />
        }
        label={userStatusLabels[user.status]}
        sx={{ m: 0, gap: 0.5 }}
      />
      <ConfirmDialog
        open={confirmOpen}
        title="Отключить пользователя?"
        description={`${user.full_name} (${user.email}) не сможет войти в систему, активные сессии будут недействительны. Доступ можно восстановить позже.`}
        confirmLabel="Отключить"
        destructive
        loading={mutation.isPending}
        onClose={() => setConfirmOpen(false)}
        onConfirm={() => apply('disabled')}
      />
    </>
  );
}
