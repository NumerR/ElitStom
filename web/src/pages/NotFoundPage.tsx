import { Button } from '@mui/material';
import SearchOffRoundedIcon from '@mui/icons-material/SearchOffRounded';
import { Link as RouterLink } from 'react-router-dom';
import { EmptyState } from '../components/EmptyState';

export function NotFoundPage() {
  return (
    <EmptyState
      icon={<SearchOffRoundedIcon />}
      title="Страница не найдена"
      description="Возможно, ссылка устарела или в адресе опечатка."
      action={
        <Button component={RouterLink} to="/" variant="contained">
          На главную
        </Button>
      }
    />
  );
}
