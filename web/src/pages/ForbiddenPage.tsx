import { Button } from '@mui/material';
import LockRoundedIcon from '@mui/icons-material/LockRounded';
import { Link as RouterLink } from 'react-router-dom';
import { EmptyState } from '../components/EmptyState';

export function ForbiddenPage() {
  return (
    <EmptyState
      icon={<LockRoundedIcon />}
      title="Нет доступа"
      description="У вашей роли нет прав на просмотр этой страницы."
      action={
        <Button component={RouterLink} to="/" variant="contained">
          На главную
        </Button>
      }
    />
  );
}
