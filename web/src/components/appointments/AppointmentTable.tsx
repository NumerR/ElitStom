import {
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import type { Appointment } from '../../api/types';
import { formatDateShort, formatTimeRange } from '../../lib/time';
import { StatusChip } from '../StatusChip';

export interface AppointmentListProps {
  appointments: Appointment[];
  tz: string;
  showPatient: boolean;
  showDoctor: boolean;
  onOpen: (appt: Appointment) => void;
}

export function AppointmentTable({ appointments, tz, showPatient, showDoctor, onOpen }: AppointmentListProps) {
  return (
    <TableContainer component={Paper} variant="outlined" sx={{ overflowX: 'auto' }}>
      <Table aria-label="Список записей">
        <TableHead>
          <TableRow>
            <TableCell>Дата</TableCell>
            <TableCell>Время</TableCell>
            {showDoctor && <TableCell>Врач</TableCell>}
            {showPatient && <TableCell>Пациент</TableCell>}
            <TableCell>Статус</TableCell>
            <TableCell>Причина</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {appointments.map((a) => (
            <TableRow
              key={a.id}
              hover
              tabIndex={0}
              onClick={() => onOpen(a)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  onOpen(a);
                }
              }}
              aria-label={`Открыть запись ${formatDateShort(a.starts_at, tz)}`}
              sx={{ cursor: 'pointer', '&:focus-visible': { outlineOffset: -2 } }}
            >
              <TableCell sx={{ whiteSpace: 'nowrap' }}>{formatDateShort(a.starts_at, tz)}</TableCell>
              <TableCell sx={{ whiteSpace: 'nowrap', fontVariantNumeric: 'tabular-nums' }}>
                {formatTimeRange(a.starts_at, a.ends_at, tz)}
              </TableCell>
              {showDoctor && <TableCell>{a.doctor.full_name}</TableCell>}
              {showPatient && <TableCell>{a.patient.full_name}</TableCell>}
              <TableCell>
                <StatusChip status={a.status} />
              </TableCell>
              <TableCell sx={{ maxWidth: 280 }}>
                <Typography variant="body2" color="text.secondary" noWrap>
                  {a.reason || '—'}
                </Typography>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
