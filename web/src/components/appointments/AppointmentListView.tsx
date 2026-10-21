import { useIsMobile } from '../../hooks/useIsMobile';
import { AppointmentCards } from './AppointmentCards';
import { AppointmentTable, type AppointmentListProps } from './AppointmentTable';

export function AppointmentListView(props: AppointmentListProps) {
  const isMobile = useIsMobile();
  return isMobile ? <AppointmentCards {...props} /> : <AppointmentTable {...props} />;
}
