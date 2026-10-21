import { useQuery } from '@tanstack/react-query';
import { listUsers } from '../api/users';
import { queryKeys } from '../api/queryKeys';

const params = { role: 'patient', status: 'active', limit: 200 } as const;

export function usePatients(enabled = true) {
  return useQuery({
    queryKey: queryKeys.adminUsers(params),
    queryFn: () => listUsers(params),
    enabled,
    staleTime: 60 * 1000,
    select: (data) => data.users,
  });
}
