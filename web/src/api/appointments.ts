import { apiRequest } from './client';
import type {
  Appointment,
  AppointmentList,
  AppointmentListParams,
  CreateAppointmentInput,
  RescheduleInput,
} from './types';

const base = (id: string) => `/appointments/${encodeURIComponent(id)}`;

export function listAppointments(params: AppointmentListParams): Promise<AppointmentList> {
  return apiRequest<AppointmentList>('/appointments', { query: { ...params } });
}

export function getAppointment(id: string): Promise<Appointment> {
  return apiRequest<Appointment>(base(id));
}

export function createAppointment(input: CreateAppointmentInput): Promise<Appointment> {
  return apiRequest<Appointment>('/appointments', { method: 'POST', body: input });
}

export function cancelAppointment(id: string, reason: string): Promise<Appointment> {
  return apiRequest<Appointment>(`${base(id)}/cancel`, { method: 'POST', body: { reason } });
}

export function confirmAppointment(id: string): Promise<Appointment> {
  return apiRequest<Appointment>(`${base(id)}/confirm`, { method: 'POST' });
}

export function completeAppointment(id: string): Promise<Appointment> {
  return apiRequest<Appointment>(`${base(id)}/complete`, { method: 'POST' });
}

export function markNoShow(id: string): Promise<Appointment> {
  return apiRequest<Appointment>(`${base(id)}/no-show`, { method: 'POST' });
}

export function rescheduleAppointment(id: string, input: RescheduleInput): Promise<Appointment> {
  return apiRequest<Appointment>(`${base(id)}/reschedule`, { method: 'POST', body: input });
}
