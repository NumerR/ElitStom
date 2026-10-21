export type Role = 'admin' | 'doctor' | 'patient';
export type UserStatus = 'active' | 'disabled';

export interface User {
  id: string;
  email: string;
  full_name: string;
  phone: string;
  role: Role;
  status: UserStatus;
  created_at: string;
  updated_at: string;
}

export interface Tokens {
  access_token: string;
  token_type: 'Bearer';
  access_expires_at: string;
  refresh_token: string;
  refresh_expires_at: string;
}

export interface AuthResponse {
  user: User;
  tokens: Tokens;
}

export interface RegisterInput {
  email: string;
  full_name: string;
  phone: string;
  password: string;
}

export interface LoginInput {
  email: string;
  password: string;
}

export interface UpdateProfileInput {
  full_name: string;
  phone: string;
}

export interface CreateUserInput {
  email: string;
  full_name: string;
  phone: string;
  role: Role;
  password: string;
}

export interface UserListParams {
  role?: Role;
  status?: UserStatus;
  limit?: number;
  offset?: number;
}

export interface UserList {
  users: User[];
  limit: number;
  offset: number;
}

export interface ClinicSchedule {
  timezone: string;
  opens_at: string;
  closes_at: string;
  workdays: number[];
  slot_minutes: number;
  booking_horizon_days: number;
}

export interface PersonRef {
  id: string;
  full_name: string;
}

export interface DoctorList {
  doctors: PersonRef[];
}

export interface Slot {
  starts_at: string;
  ends_at: string;
}

export interface SlotList {
  date: string;
  slots: Slot[];
}

export type AppointmentStatus = 'scheduled' | 'confirmed' | 'completed' | 'cancelled' | 'no_show';

export interface Appointment {
  id: string;
  patient: PersonRef;
  doctor: PersonRef;
  starts_at: string;
  ends_at: string;
  status: AppointmentStatus;
  reason: string;
  cancel_reason: string;
  created_at: string;
  updated_at: string;
}

export interface CreateAppointmentInput {
  doctor_id: string;
  patient_id?: string;
  starts_at: string;
  duration_minutes?: number;
  reason: string;
}

export interface RescheduleInput {
  starts_at: string;
  duration_minutes?: number;
}

export interface AppointmentListParams {
  status?: AppointmentStatus;
  doctor_id?: string;
  patient_id?: string;
  from?: string;
  to?: string;
  order?: 'asc' | 'desc';
  limit?: number;
  offset?: number;
}

export interface AppointmentList {
  appointments: Appointment[];
  limit: number;
  offset: number;
}

export interface ApiErrorBody {
  error: {
    code: string;
    message: string;
    details?: string[];
  };
}
