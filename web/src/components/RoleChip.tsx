import { Chip, type ChipProps } from '@mui/material';
import type { Role } from '../api/types';
import { roleLabels } from '../lib/labels';

const colors: Record<Role, ChipProps['color']> = {
  admin: 'secondary',
  doctor: 'primary',
  patient: 'default',
};

export function RoleChip({ role, size = 'small' }: { role: Role; size?: ChipProps['size'] }) {
  return <Chip label={roleLabels[role]} color={colors[role]} size={size} variant="outlined" />;
}
