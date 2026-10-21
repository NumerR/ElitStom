import { apiRequest } from './client';
import type { ClinicSchedule, DoctorList, SlotList } from './types';

export function getSchedule(): Promise<ClinicSchedule> {
  return apiRequest<ClinicSchedule>('/clinic/schedule');
}

export function listDoctors(): Promise<DoctorList> {
  return apiRequest<DoctorList>('/doctors');
}

export function listSlots(doctorId: string, date: string): Promise<SlotList> {
  return apiRequest<SlotList>(`/doctors/${encodeURIComponent(doctorId)}/slots`, { query: { date } });
}
