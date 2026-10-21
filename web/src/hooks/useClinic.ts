import { useQuery } from '@tanstack/react-query';
import { getSchedule, listDoctors, listSlots } from '../api/clinic';
import { queryKeys } from '../api/queryKeys';
import { DEFAULT_CLINIC_TZ } from '../lib/time';

export function useClinicSchedule() {
  return useQuery({
    queryKey: queryKeys.schedule,
    queryFn: getSchedule,
    staleTime: 60 * 60 * 1000,
  });
}

export function useClinicTz(): string {
  const { data } = useClinicSchedule();
  return data?.timezone ?? DEFAULT_CLINIC_TZ;
}

export function useDoctors() {
  return useQuery({
    queryKey: queryKeys.doctors,
    queryFn: listDoctors,
    staleTime: 5 * 60 * 1000,
    select: (data) => data.doctors,
  });
}

export function useSlots(doctorId: string | null, date: string | null) {
  return useQuery({
    queryKey: queryKeys.slots(doctorId ?? '', date ?? ''),
    queryFn: () => listSlots(doctorId as string, date as string),
    enabled: !!doctorId && !!date,
    staleTime: 15 * 1000,
    select: (data) => data.slots,
  });
}
