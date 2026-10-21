import type { AppointmentStatus, Role, UserStatus } from '../api/types';

export const roleLabels: Record<Role, string> = {
  admin: 'Администратор',
  doctor: 'Врач',
  patient: 'Пациент',
};

export const userStatusLabels: Record<UserStatus, string> = {
  active: 'Активен',
  disabled: 'Отключён',
};

export const appointmentStatusLabels: Record<AppointmentStatus, string> = {
  scheduled: 'Запланирован',
  confirmed: 'Подтверждён',
  completed: 'Завершён',
  cancelled: 'Отменён',
  no_show: 'Неявка',
};

export const appointmentStatuses: AppointmentStatus[] = [
  'scheduled',
  'confirmed',
  'completed',
  'cancelled',
  'no_show',
];

export const roles: Role[] = ['patient', 'doctor', 'admin'];

export function initials(fullName: string): string {
  const parts = fullName.trim().split(/\s+/).filter(Boolean);
  return parts
    .slice(0, 2)
    .map((p) => p.charAt(0).toUpperCase())
    .join('');
}

export function firstName(fullName: string): string {
  const parts = fullName.trim().split(/\s+/);
  return parts.length >= 3 ? parts[1] : parts[0] ?? '';
}
