import { useState } from 'react';
import { Button, Card, CardContent, MenuItem, Skeleton, Stack, TextField } from '@mui/material';
import PersonAddRoundedIcon from '@mui/icons-material/PersonAddRounded';
import GroupOffRoundedIcon from '@mui/icons-material/GroupOffRounded';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { listUsers } from '../api/users';
import { queryKeys } from '../api/queryKeys';
import type { Role, UserListParams, UserStatus } from '../api/types';
import { useCurrentUser } from '../auth/authContext';
import { EmptyState } from '../components/EmptyState';
import { ErrorAlert } from '../components/ErrorAlert';
import { PageHeader } from '../components/PageHeader';
import { Pager } from '../components/Pager';
import { useIsMobile } from '../hooks/useIsMobile';
import { roleLabels, roles, userStatusLabels } from '../lib/labels';
import { CreateUserDialog } from './admin/CreateUserDialog';
import { UserCards, UsersTable } from './admin/UsersTable';

const PAGE_SIZE = 25;
const statuses: UserStatus[] = ['active', 'disabled'];

export function AdminUsersPage() {
  const me = useCurrentUser();
  const isMobile = useIsMobile();
  const [createOpen, setCreateOpen] = useState(false);
  const [sp, setSp] = useSearchParams();

  const rawRole = sp.get('role');
  const role = roles.includes(rawRole as Role) ? (rawRole as Role) : undefined;
  const rawStatus = sp.get('status');
  const status = statuses.includes(rawStatus as UserStatus) ? (rawStatus as UserStatus) : undefined;
  const page = Math.max(0, Number(sp.get('page')) || 0);

  const update = (patch: Record<string, string | null>) => {
    const next = new URLSearchParams(sp);
    for (const [key, value] of Object.entries(patch)) {
      if (!value) next.delete(key);
      else next.set(key, value);
    }
    if (!('page' in patch)) next.delete('page');
    setSp(next, { replace: true });
  };

  const params: UserListParams = { role, status, limit: PAGE_SIZE, offset: page * PAGE_SIZE };
  const query = useQuery({
    queryKey: queryKeys.adminUsers(params),
    queryFn: () => listUsers(params),
    placeholderData: keepPreviousData,
  });
  const users = query.data?.users ?? [];

  return (
    <>
      <PageHeader
        title="Пользователи"
        subtitle="Управление ролями и доступом"
        actions={
          <Button variant="contained" startIcon={<PersonAddRoundedIcon />} onClick={() => setCreateOpen(true)}>
            Создать пользователя
          </Button>
        }
      />

      <Card sx={{ mb: 2 }}>
        <CardContent>
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
            <TextField
              select
              size="small"
              label="Роль"
              value={role ?? ''}
              onChange={(e) => update({ role: e.target.value || null })}
              sx={{ maxWidth: { sm: 220 } }}
            >
              <MenuItem value="">Все роли</MenuItem>
              {roles.map((r) => (
                <MenuItem key={r} value={r}>
                  {roleLabels[r]}
                </MenuItem>
              ))}
            </TextField>
            <TextField
              select
              size="small"
              label="Статус"
              value={status ?? ''}
              onChange={(e) => update({ status: e.target.value || null })}
              sx={{ maxWidth: { sm: 220 } }}
            >
              <MenuItem value="">Все статусы</MenuItem>
              {statuses.map((s) => (
                <MenuItem key={s} value={s}>
                  {userStatusLabels[s]}
                </MenuItem>
              ))}
            </TextField>
          </Stack>
        </CardContent>
      </Card>

      {query.isPending && (
        <Stack spacing={1}>
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} variant="rounded" height={60} />
          ))}
        </Stack>
      )}
      {query.isError && <ErrorAlert error={query.error} onRetry={() => void query.refetch()} />}
      {query.isSuccess && users.length === 0 && (
        <Card>
          <EmptyState
            icon={<GroupOffRoundedIcon />}
            title="Пользователи не найдены"
            description={role || status ? 'Попробуйте изменить фильтры.' : undefined}
            action={
              <Button variant="tonal" onClick={() => setCreateOpen(true)}>
                Создать пользователя
              </Button>
            }
          />
        </Card>
      )}
      {query.isSuccess && users.length > 0 && (
        <div style={{ opacity: query.isPlaceholderData ? 0.6 : 1, transition: 'opacity 150ms' }}>
          {isMobile ? (
            <UserCards users={users} currentUserId={me.id} />
          ) : (
            <UsersTable users={users} currentUserId={me.id} />
          )}
        </div>
      )}
      <Pager
        page={page}
        hasNext={users.length === PAGE_SIZE}
        disabled={query.isFetching}
        onChange={(p) => update({ page: p === 0 ? null : String(p) })}
      />

      <CreateUserDialog open={createOpen} onClose={() => setCreateOpen(false)} />
    </>
  );
}
