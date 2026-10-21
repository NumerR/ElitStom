import type { AppointmentListParams, UserListParams } from './types';

export const queryKeys = {
  schedule: ['clinic', 'schedule'] as const,
  doctors: ['doctors'] as const,
  slotsAll: ['slots'] as const,
  slots: (doctorId: string, date: string) => ['slots', doctorId, date] as const,
  appointmentsAll: ['appointments'] as const,
  appointmentList: (params: AppointmentListParams) => ['appointments', 'list', params] as const,
  appointment: (id: string) => ['appointments', 'detail', id] as const,
  adminUsersAll: ['admin', 'users'] as const,
  adminUsers: (params: UserListParams) => ['admin', 'users', params] as const,
};
