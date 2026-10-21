import type { ReactNode } from 'react';
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
} from '@mui/material';

interface Props {
  open: boolean;
  title: string;
  description?: ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  destructive?: boolean;
  loading?: boolean;
  onConfirm: () => void;
  onClose: () => void;
  children?: ReactNode;
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  cancelLabel = 'Отмена',
  destructive,
  loading,
  onConfirm,
  onClose,
  children,
}: Props) {
  return (
    <Dialog
      open={open}
      onClose={loading ? undefined : onClose}
      aria-labelledby="confirm-dialog-title"
      fullWidth
      maxWidth="xs"
    >
      <DialogTitle id="confirm-dialog-title">{title}</DialogTitle>
      <DialogContent>
        {description && <DialogContentText sx={{ mb: children ? 2 : 0 }}>{description}</DialogContentText>}
        {children}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={loading}>
          {cancelLabel}
        </Button>
        <Button
          variant="contained"
          color={destructive ? 'error' : 'primary'}
          onClick={onConfirm}
          loading={loading}
        >
          {confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
