import { Alert, AlertTitle, Button } from '@mui/material';
import { errorMessage } from '../lib/errors';

interface Props {
  error: unknown;
  title?: string;
  onRetry?: () => void;
}

export function ErrorAlert({ error, title = 'Не удалось загрузить данные', onRetry }: Props) {
  return (
    <Alert
      severity="error"
      action={
        onRetry ? (
          <Button color="inherit" size="small" onClick={onRetry}>
            Повторить
          </Button>
        ) : undefined
      }
    >
      <AlertTitle>{title}</AlertTitle>
      {errorMessage(error)}
    </Alert>
  );
}
