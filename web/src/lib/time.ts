import dayjs, { type Dayjs } from 'dayjs';
import utc from 'dayjs/plugin/utc';
import timezone from 'dayjs/plugin/timezone';
import isoWeek from 'dayjs/plugin/isoWeek';
import 'dayjs/locale/ru';
import type { ClinicSchedule } from '../api/types';

dayjs.extend(utc);
dayjs.extend(timezone);
dayjs.extend(isoWeek);
dayjs.locale('ru');

export const DEFAULT_CLINIC_TZ = 'Europe/Moscow';
export const DATE_KEY_FORMAT = 'YYYY-MM-DD';

export { dayjs };
export type { Dayjs };

export function toRFC3339(value: Dayjs): string {
  return value.utc().format('YYYY-MM-DDTHH:mm:ss[Z]');
}

export function nowRFC3339Minute(): string {
  return toRFC3339(dayjs().startOf('minute'));
}

export function inTz(iso: string, tz: string): Dayjs {
  return dayjs(iso).tz(tz);
}

export function formatTime(iso: string, tz: string): string {
  return inTz(iso, tz).format('HH:mm');
}

export function formatTimeRange(startIso: string, endIso: string, tz: string): string {
  return `${formatTime(startIso, tz)}–${formatTime(endIso, tz)}`;
}

export function formatDateLong(iso: string, tz: string): string {
  return capitalize(inTz(iso, tz).format('dddd, D MMMM YYYY'));
}

export function formatDateShort(iso: string, tz: string): string {
  return inTz(iso, tz).format('D MMM YYYY, dd');
}

export function formatDateTime(iso: string, tz: string): string {
  return inTz(iso, tz).format('D MMM YYYY, HH:mm');
}

export function formatLocalDate(iso: string): string {
  return dayjs(iso).format('D MMMM YYYY');
}

export function formatDateKeyLong(dateKey: string): string {
  return capitalize(dayjs(dateKey).format('dddd, D MMMM YYYY'));
}

export function durationMinutes(startIso: string, endIso: string): number {
  return dayjs(endIso).diff(dayjs(startIso), 'minute');
}

export function formatDuration(minutes: number): string {
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${m} мин`;
  return m === 0 ? `${h} ч` : `${h} ч ${m} мин`;
}

export function todayKey(tz: string): string {
  return dayjs().tz(tz).format(DATE_KEY_FORMAT);
}

export function shiftDateKey(dateKey: string, days: number): string {
  return dayjs(dateKey).add(days, 'day').format(DATE_KEY_FORMAT);
}

export function dayBounds(dateKey: string, tz: string): { from: string; to: string } {
  const start = dayjs.tz(dateKey, tz).startOf('day');
  return { from: toRFC3339(start), to: toRFC3339(start.add(1, 'day')) };
}

export function isoWeekdayOfKey(dateKey: string): number {
  return dayjs(dateKey).isoWeekday();
}

export function tzLabel(tz: string): string {
  return tz === 'Europe/Moscow' ? 'время клиники, МСК' : `время клиники, ${tz}`;
}

export interface BookingWindow {
  first: string;
  last: string;
}

export function bookingWindow(schedule: ClinicSchedule): BookingWindow {
  const first = todayKey(schedule.timezone);
  return { first, last: shiftDateKey(first, schedule.booking_horizon_days) };
}

export function isBookableDay(dateKey: string, schedule: ClinicSchedule): boolean {
  const { first, last } = bookingWindow(schedule);
  if (dateKey < first || dateKey > last) return false;
  return schedule.workdays.includes(isoWeekdayOfKey(dateKey));
}

export function firstBookableDay(schedule: ClinicSchedule): string | null {
  const { first } = bookingWindow(schedule);
  for (let i = 0; i <= schedule.booking_horizon_days; i += 1) {
    const key = shiftDateKey(first, i);
    if (isBookableDay(key, schedule)) return key;
  }
  return null;
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
