import type { Appointment, User } from '../api/types';

export interface AppointmentPermissions {
  cancel: boolean;
  reschedule: boolean;
  confirm: boolean;
  complete: boolean;
  noShow: boolean;
}

export function appointmentPermissions(
  appt: Appointment,
  user: User,
  now: number = Date.now(),
): AppointmentPermissions {
  const start = Date.parse(appt.starts_at);
  const end = Date.parse(appt.ends_at);
  const active = appt.status === 'scheduled' || appt.status === 'confirmed';
  const beforeStart = now < start;
  const beforeEnd = now < end;
  const admin = user.role === 'admin';
  const ownPatient = user.role === 'patient' && appt.patient.id === user.id;
  const ownDoctor = user.role === 'doctor' && appt.doctor.id === user.id;
  const staff = admin || ownDoctor;

  return {
    cancel: active && beforeEnd && (staff || (ownPatient && beforeStart)),
    reschedule: active && (admin || (ownPatient && beforeStart)),
    confirm: appt.status === 'scheduled' && beforeEnd && staff,
    complete: active && !beforeStart && staff,
    noShow: active && !beforeStart && staff,
  };
}

export function hasAnyPermission(p: AppointmentPermissions): boolean {
  return p.cancel || p.reschedule || p.confirm || p.complete || p.noShow;
}
