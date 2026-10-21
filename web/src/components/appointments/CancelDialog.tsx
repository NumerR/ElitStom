import { useState } from 'react';
import { TextField } from '@mui/material';
import { ConfirmDialog } from '../ConfirmDialog';
import { REASON_MAX_CHARS, validateReason } from '../../lib/validation';

interface Props {
  open: boolean;
  loading: boolean;
  onClose: () => void;
  onConfirm: (reason: string) => void;
}

export function CancelDialog({ open, loading, onClose, onConfirm }: Props) {
  const [reason, setReason] = useState('');
  const error = validateReason(reason);

  return (
    <ConfirmDialog
      open={open}
      title="Отменить запись?"
      description="Время освободится для других пациентов. Это действие нельзя отменить."
      confirmLabel="Отменить запись"
      cancelLabel="Не отменять"
      destructive
      loading={loading}
      onClose={onClose}
      onConfirm={() => {
        if (!error) onConfirm(reason.trim());
      }}
    >
      <TextField
        label="Причина отмены (необязательно)"
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        multiline
        minRows={2}
        maxRows={6}
        error={!!error}
        helperText={error ?? `${[...reason].length}/${REASON_MAX_CHARS}`}
        autoFocus
      />
    </ConfirmDialog>
  );
}
