import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  cancelAppointment,
  completeAppointment,
  confirmAppointment,
  createAppointment,
  markNoShow,
  rescheduleAppointment,
} from '../api/appointments';
import { queryKeys } from '../api/queryKeys';
import type { Appointment, CreateAppointmentInput, RescheduleInput } from '../api/types';

function useInvalidation() {
  const queryClient = useQueryClient();
  const refreshAll = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.appointmentsAll });
    void queryClient.invalidateQueries({ queryKey: queryKeys.slotsAll });
  };
  const onSuccess = (appt: Appointment) => {
    queryClient.setQueryData(queryKeys.appointment(appt.id), appt);
    refreshAll();
  };
  return { onSuccess, onError: refreshAll };
}

export function useCreateAppointment() {
  const handlers = useInvalidation();
  return useMutation({
    mutationFn: (input: CreateAppointmentInput) => createAppointment(input),
    ...handlers,
  });
}

export function useAppointmentActions() {
  const handlers = useInvalidation();
  const confirm = useMutation({ mutationFn: confirmAppointment, ...handlers });
  const complete = useMutation({ mutationFn: completeAppointment, ...handlers });
  const noShow = useMutation({ mutationFn: markNoShow, ...handlers });
  const cancel = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => cancelAppointment(id, reason),
    ...handlers,
  });
  const reschedule = useMutation({
    mutationFn: ({ id, input }: { id: string; input: RescheduleInput }) =>
      rescheduleAppointment(id, input),
    ...handlers,
  });
  return { confirm, complete, noShow, cancel, reschedule };
}
