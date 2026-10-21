import {
  Avatar,
  Card,
  CardContent,
  Chip,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import type { User } from '../../api/types';
import { initials } from '../../lib/labels';
import { formatLocalDate } from '../../lib/time';
import { UserRoleControl, UserStatusControl } from './UserControls';

interface Props {
  users: User[];
  currentUserId: string;
}

function NameCell({ user, isSelf }: { user: User; isSelf: boolean }) {
  return (
    <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center', minWidth: 0 }}>
      <Avatar sx={{ width: 36, height: 36, fontSize: 14, bgcolor: user.status === 'active' ? 'primary.main' : 'action.disabled' }}>
        {initials(user.full_name)}
      </Avatar>
      <div style={{ minWidth: 0 }}>
        <Typography variant="body2" component="div" sx={{ fontWeight: 600 }} noWrap>
          {user.full_name} {isSelf && <Chip size="small" label="Вы" sx={{ ml: 0.5, height: 20 }} />}
        </Typography>
        <Typography variant="caption" color="text.secondary" noWrap component="div">
          {user.email}
        </Typography>
      </div>
    </Stack>
  );
}

export function UsersTable({ users, currentUserId }: Props) {
  return (
    <TableContainer component={Paper} variant="outlined" sx={{ overflowX: 'auto' }}>
      <Table aria-label="Пользователи">
        <TableHead>
          <TableRow>
            <TableCell>Пользователь</TableCell>
            <TableCell>Телефон</TableCell>
            <TableCell>Роль</TableCell>
            <TableCell>Доступ</TableCell>
            <TableCell>Создан</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {users.map((u) => {
            const isSelf = u.id === currentUserId;
            return (
              <TableRow key={u.id}>
                <TableCell sx={{ maxWidth: 320 }}>
                  <NameCell user={u} isSelf={isSelf} />
                </TableCell>
                <TableCell sx={{ whiteSpace: 'nowrap' }}>{u.phone || '—'}</TableCell>
                <TableCell>
                  <UserRoleControl user={u} isSelf={isSelf} />
                </TableCell>
                <TableCell>
                  <UserStatusControl user={u} isSelf={isSelf} />
                </TableCell>
                <TableCell sx={{ whiteSpace: 'nowrap' }}>{formatLocalDate(u.created_at)}</TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

export function UserCards({ users, currentUserId }: Props) {
  return (
    <Stack spacing={1.5} component="ul" sx={{ listStyle: 'none', p: 0, m: 0 }} aria-label="Пользователи">
      {users.map((u) => {
        const isSelf = u.id === currentUserId;
        return (
          <li key={u.id}>
            <Card>
              <CardContent>
                <NameCell user={u} isSelf={isSelf} />
                <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                  {u.phone || 'Телефон не указан'} · с {formatLocalDate(u.created_at)}
                </Typography>
                <Stack direction="row" useFlexGap sx={{ mt: 1.5, flexWrap: 'wrap', gap: 1.5, alignItems: 'center' }}>
                  <UserRoleControl user={u} isSelf={isSelf} />
                  <UserStatusControl user={u} isSelf={isSelf} />
                </Stack>
              </CardContent>
            </Card>
          </li>
        );
      })}
    </Stack>
  );
}
