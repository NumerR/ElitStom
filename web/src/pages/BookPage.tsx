import { useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Step,
  StepLabel,
  Stepper,
  TextField,
  Typography,
} from '@mui/material';
import ArrowBackRoundedIcon from '@mui/icons-material/ArrowBackRounded';
import ArrowForwardRoundedIcon from '@mui/icons-material/ArrowForwardRounded';
import { useNavigate } from 'react-router-dom';
import type { PersonRef, Slot, User } from '../api/types';
import { useCurrentUser } from '../auth/authContext';
import { PageHeader } from '../components/PageHeader';
import { SlotPicker } from '../components/SlotPicker';
import { useNotify } from '../components/notifyContext';
import { useClinicTz } from '../hooks/useClinic';
import { useCreateAppointment } from '../hooks/useAppointmentMutations';
import { errorMessage, isApiError, mapFieldErrors } from '../lib/errors';
import { REASON_MAX_CHARS, validateReason } from '../lib/validation';
import { DoctorStep } from './book/DoctorStep';
import { PatientStep } from './book/PatientStep';
import { ReviewStep } from './book/ReviewStep';

type StepId = 'patient' | 'doctor' | 'time' | 'reason' | 'review';

const stepLabels: Record<StepId, string> = {
  patient: 'Пациент',
  doctor: 'Врач',
  time: 'Дата и время',
  reason: 'Причина',
  review: 'Подтверждение',
};

const stepHints: Record<StepId, string> = {
  patient: 'Выберите пациента, которого нужно записать',
  doctor: 'Выберите врача',
  time: 'Выберите день и свободное время',
  reason: 'Кратко опишите причину обращения — это поможет врачу подготовиться',
  review: 'Проверьте данные записи',
};

export function BookPage() {
  const user = useCurrentUser();
  const isAdmin = user.role === 'admin';
  const steps: StepId[] = isAdmin
    ? ['patient', 'doctor', 'time', 'reason', 'review']
    : ['doctor', 'time', 'reason', 'review'];

  const navigate = useNavigate();
  const notify = useNotify();
  const tz = useClinicTz();
  const create = useCreateAppointment();

  const [active, setActive] = useState(0);
  const [patient, setPatient] = useState<User | null>(null);
  const [doctor, setDoctor] = useState<PersonRef | null>(null);
  const [date, setDate] = useState<string | null>(null);
  const [slot, setSlot] = useState<Slot | null>(null);
  const [reason, setReason] = useState('');
  const [submitError, setSubmitError] = useState<string | null>(null);

  const step = steps[active];
  const reasonError = validateReason(reason);

  const canProceed: Record<StepId, boolean> = {
    patient: !!patient,
    doctor: !!doctor,
    time: !!slot,
    reason: !reasonError,
    review: true,
  };

  const goTo = (id: StepId) => setActive(steps.indexOf(id));

  const submit = () => {
    if (!doctor || !slot) return;
    setSubmitError(null);
    create.mutate(
      {
        doctor_id: doctor.id,
        ...(isAdmin && patient ? { patient_id: patient.id } : {}),
        starts_at: slot.starts_at,
        reason: reason.trim(),
      },
      {
        onSuccess: (appt) => {
          notify(isAdmin ? 'Запись создана' : 'Вы записаны на приём');
          navigate(`/appointments/${appt.id}`, { replace: true });
        },
        onError: (err) => {
          if (isApiError(err, 'conflict')) {
            notify('Это время уже занято, выберите другое', 'error');
            setSlot(null);
            goTo('time');
            return;
          }
          const mapped = mapFieldErrors(err, [] as const);
          setSubmitError(
            mapped.rest.length ? `${errorMessage(err)}: ${mapped.rest.join('; ')}` : errorMessage(err),
          );
        },
      },
    );
  };

  return (
    <>
      <PageHeader
        title={isAdmin ? 'Новая запись' : 'Запись на приём'}
        subtitle={stepHints[step]}
      />
      <Stepper
        activeStep={active}
        alternativeLabel
        sx={{
          mb: 3,
          '& .MuiStepLabel-label': { display: { xs: 'none', sm: 'block' } },
        }}
      >
        {steps.map((id) => (
          <Step key={id}>
            <StepLabel>{stepLabels[id]}</StepLabel>
          </Step>
        ))}
      </Stepper>
      <Typography
        variant="overline"
        color="text.secondary"
        sx={{ display: { xs: 'block', sm: 'none' }, mb: 1 }}
      >
        Шаг {active + 1} из {steps.length}: {stepLabels[step]}
      </Typography>

      <Card>
        <CardContent sx={{ p: { xs: 2, sm: 3 } }}>
          {step === 'patient' && <PatientStep value={patient} onChange={setPatient} />}
          {step === 'doctor' && (
            <DoctorStep
              value={doctor}
              onChange={(d) => {
                if (d.id !== doctor?.id) {
                  setDate(null);
                  setSlot(null);
                }
                setDoctor(d);
              }}
            />
          )}
          {step === 'time' && doctor && (
            <SlotPicker
              doctorId={doctor.id}
              date={date}
              slot={slot}
              onChange={(d, s) => {
                setDate(d);
                setSlot(s);
              }}
            />
          )}
          {step === 'reason' && (
            <Box sx={{ maxWidth: 640 }}>
              <TextField
                label="Причина обращения (необязательно)"
                placeholder="Например: болит зуб, профилактический осмотр, чистка"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                multiline
                minRows={4}
                maxRows={10}
                error={!!reasonError}
                helperText={reasonError ?? `${[...reason].length}/${REASON_MAX_CHARS}`}
                autoFocus
              />
            </Box>
          )}
          {step === 'review' && doctor && slot && (
            <>
              {submitError && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  {submitError}
                </Alert>
              )}
              <ReviewStep patient={isAdmin ? patient : null} doctor={doctor} slot={slot} reason={reason} tz={tz} />
            </>
          )}

          <Box sx={{ display: 'flex', justifyContent: 'space-between', mt: 4, gap: 1 }}>
            <Button
              startIcon={<ArrowBackRoundedIcon />}
              onClick={() => setActive((a) => a - 1)}
              disabled={active === 0 || create.isPending}
            >
              Назад
            </Button>
            {step === 'review' ? (
              <Button variant="contained" size="large" onClick={submit} loading={create.isPending}>
                {isAdmin ? 'Создать запись' : 'Записаться'}
              </Button>
            ) : (
              <Button
                variant="contained"
                endIcon={<ArrowForwardRoundedIcon />}
                onClick={() => setActive((a) => a + 1)}
                disabled={!canProceed[step]}
              >
                Далее
              </Button>
            )}
          </Box>
        </CardContent>
      </Card>
    </>
  );
}
