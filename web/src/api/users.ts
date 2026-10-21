import { apiRequest } from './client';
import type {
  CreateUserInput,
  Role,
  UpdateProfileInput,
  User,
  UserList,
  UserListParams,
  UserStatus,
} from './types';

export function getMe(): Promise<User> {
  return apiRequest<User>('/users/me');
}

export function updateMe(input: UpdateProfileInput): Promise<User> {
  return apiRequest<User>('/users/me', { method: 'PATCH', body: input });
}

export function listUsers(params: UserListParams): Promise<UserList> {
  return apiRequest<UserList>('/admin/users', { query: { ...params } });
}

export function createUser(input: CreateUserInput): Promise<User> {
  return apiRequest<User>('/admin/users', { method: 'POST', body: input });
}

export function setUserRole(id: string, role: Role): Promise<User> {
  return apiRequest<User>(`/admin/users/${encodeURIComponent(id)}/role`, {
    method: 'PATCH',
    body: { role },
  });
}

export function setUserStatus(id: string, status: UserStatus): Promise<User> {
  return apiRequest<User>(`/admin/users/${encodeURIComponent(id)}/status`, {
    method: 'PATCH',
    body: { status },
  });
}
