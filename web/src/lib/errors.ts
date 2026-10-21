import { ApiError } from '../api/client';

const codeMessages: Record<string, string> = {
  invalid_input: 'Проверьте правильность заполнения полей',
  unauthorized: 'Требуется повторный вход в систему',
  account_disabled: 'Учётная запись отключена. Обратитесь в клинику',
  forbidden: 'Недостаточно прав для этого действия',
  not_found: 'Запись не найдена',
  already_exists: 'Такая запись уже существует',
  conflict: 'Действие невозможно в текущем состоянии',
  internal_error: 'Внутренняя ошибка сервера. Попробуйте позже',
  network: 'Нет связи с сервером. Проверьте подключение',
};

const detailTranslations: [RegExp, string | ((m: RegExpMatchArray) => string)][] = [
  [/^is required$/i, 'Обязательное поле'],
  [/^must be a valid address$/i, 'Некорректный email'],
  [/^must have a valid domain$/i, 'Некорректный домен email'],
  [/^must contain only ASCII characters$/i, 'Только латинские символы'],
  [/^must be at least (\d+) characters$/i, (m) => `Минимум ${m[1]} символов`],
  [/^must be at most (\d+) characters$/i, (m) => `Не более ${m[1]} символов`],
  [/^must not exceed 72 bytes$/i, 'Пароль слишком длинный (не более 72 байт)'],
  [/^must contain at least (\d+) digits$/i, (m) => `Минимум ${m[1]} цифр`],
  [/^must contain at most (\d+) digits$/i, (m) => `Не более ${m[1]} цифр`],
  [/^may only start with a plus sign$/i, '«+» допустим только в начале'],
  [/^must contain only digits/i, 'Допустимы только цифры, пробелы, дефисы, точки и скобки'],
  [/^must contain only letters/i, 'Только буквы, пробелы, дефисы и апострофы'],
  [/^must be in the future$/i, 'Время должно быть в будущем'],
  [/^must fall on a working day$/i, 'Клиника в этот день не работает'],
  [/^must be aligned to the slot grid$/i, 'Время должно совпадать с сеткой приёма'],
  [/^must fit within opening hours/i, 'Время вне часов работы клиники'],
  [/^must be within (\d+) days$/i, (m) => `Запись возможна не более чем на ${m[1]} дн. вперёд`],
];

function translateDetail(message: string): string {
  for (const [re, out] of detailTranslations) {
    const m = message.match(re);
    if (m) return typeof out === 'string' ? out : out(m);
  }
  return message;
}

export function errorMessage(err: unknown, overrides: Partial<Record<string, string>> = {}): string {
  if (err instanceof ApiError) {
    return overrides[err.code] ?? codeMessages[err.code] ?? err.message;
  }
  return 'Что-то пошло не так. Попробуйте ещё раз';
}

export interface MappedErrors<K extends string> {
  fields: Partial<Record<K, string>>;
  rest: string[];
}

export function mapFieldErrors<K extends string>(err: unknown, fields: readonly K[]): MappedErrors<K> {
  const result: MappedErrors<K> = { fields: {}, rest: [] };
  if (!(err instanceof ApiError) || err.code !== 'invalid_input') return result;
  for (const detail of err.details) {
    const idx = detail.indexOf(':');
    const field = idx > 0 ? detail.slice(0, idx).trim() : '';
    const message = translateDetail(idx > 0 ? detail.slice(idx + 1).trim() : detail);
    if ((fields as readonly string[]).includes(field)) {
      const key = field as K;
      if (!result.fields[key]) result.fields[key] = message;
    } else {
      result.rest.push(field ? `${field}: ${message}` : message);
    }
  }
  return result;
}

export function isApiError(err: unknown, code?: string): err is ApiError {
  return err instanceof ApiError && (code === undefined || err.code === code);
}
