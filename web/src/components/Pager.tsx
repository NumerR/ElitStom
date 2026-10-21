import { IconButton, Stack, Typography } from '@mui/material';
import ChevronLeftRoundedIcon from '@mui/icons-material/ChevronLeftRounded';
import ChevronRightRoundedIcon from '@mui/icons-material/ChevronRightRounded';

interface Props {
  page: number;
  hasNext: boolean;
  onChange: (page: number) => void;
  disabled?: boolean;
}

export function Pager({ page, hasNext, onChange, disabled }: Props) {
  if (page === 0 && !hasNext) return null;
  return (
    <Stack
      direction="row"
      spacing={1}
      sx={{ alignItems: 'center', justifyContent: 'flex-end', mt: 2 }}
      component="nav"
      aria-label="Постраничная навигация"
    >
      <IconButton
        aria-label="Предыдущая страница"
        onClick={() => onChange(page - 1)}
        disabled={disabled || page === 0}
      >
        <ChevronLeftRoundedIcon />
      </IconButton>
      <Typography variant="body2" color="text.secondary" aria-live="polite">
        Страница {page + 1}
      </Typography>
      <IconButton
        aria-label="Следующая страница"
        onClick={() => onChange(page + 1)}
        disabled={disabled || !hasNext}
      >
        <ChevronRightRoundedIcon />
      </IconButton>
    </Stack>
  );
}
