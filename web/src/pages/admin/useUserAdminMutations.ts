import { useMutation, useQueryClient } from '@tanstack/react-query';
import { createUser, setUserRole, setUserStatus } from '../../api/users';
import { queryKeys } from '../../api/queryKeys';
import type { CreateUserInput, Role, UserStatus } from '../../api/types';

function useInvalidateUsers() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.adminUsersAll });
    void queryClient.invalidateQueries({ queryKey: queryKeys.doctors });
  };
}

export function useSetUserRole() {
  const invalidate = useInvalidateUsers();
  return useMutation({
    mutationFn: ({ id, role }: { id: string; role: Role }) => setUserRole(id, role),
    onSettled: invalidate,
  });
}

export function useSetUserStatus() {
  const invalidate = useInvalidateUsers();
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: UserStatus }) => setUserStatus(id, status),
    onSettled: invalidate,
  });
}

export function useCreateUser() {
  const invalidate = useInvalidateUsers();
  return useMutation({
    mutationFn: (input: CreateUserInput) => createUser(input),
    onSuccess: invalidate,
  });
}
