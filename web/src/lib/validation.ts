export const PASSWORD_MIN_CHARS = 12;
export const PASSWORD_MAX_BYTES = 72;
export const REASON_MAX_CHARS = 500;

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const NAME_RE = /^[\p{L}\s'’-]+$/u;
const PHONE_RE = /^\+?[0-9\s().-]+$/;

export function validateEmail(value: string): string | undefined {
  const v = value.trim();
  if (!v) return 'Введите email';
  if (v.length > 254) return 'Не более 254 символов';
  // eslint-disable-next-line no-control-regex
  if (!/^[\x00-\x7F]+$/.test(v)) return 'Email может содержать только латинские символы';
  if (!EMAIL_RE.test(v)) return 'Некорректный email';
  return undefined;
}

export function validateFullName(value: string): string | undefined {
  const v = value.trim();
  const len = [...v].length;
  if (len < 2) return 'Минимум 2 символа';
  if (len > 100) return 'Не более 100 символов';
  if (!NAME_RE.test(v)) return 'Только буквы, пробелы, дефисы и апострофы';
  return undefined;
}

export function validatePhone(value: string): string | undefined {
  const v = value.trim();
  if (!v) return undefined;
  if (v.length > 32) return 'Не более 32 символов';
  if (!PHONE_RE.test(v)) return 'Допустимы цифры, пробелы, «+» в начале, «-», «(», «)», «.»';
  const digits = v.replace(/\D/g, '').length;
  if (digits < 7) return 'Минимум 7 цифр';
  if (digits > 15) return 'Не более 15 цифр';
  return undefined;
}

export function validatePassword(value: string): string | undefined {
  if ([...value].length < PASSWORD_MIN_CHARS) return `Минимум ${PASSWORD_MIN_CHARS} символов`;
  if (new TextEncoder().encode(value).length > PASSWORD_MAX_BYTES) {
    return 'Пароль слишком длинный (не более 72 байт; кириллица занимает 2 байта на символ)';
  }
  return undefined;
}

export function validateReason(value: string): string | undefined {
  if ([...value].length > REASON_MAX_CHARS) return `Не более ${REASON_MAX_CHARS} символов`;
  return undefined;
}

export type FieldErrors<K extends string> = Partial<Record<K, string>>;

export function hasErrors<K extends string>(errors: FieldErrors<K>): boolean {
  return Object.values(errors).some(Boolean);
}
