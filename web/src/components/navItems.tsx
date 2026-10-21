import type { ReactNode } from 'react';
import DashboardRoundedIcon from '@mui/icons-material/DashboardRounded';
import EventNoteRoundedIcon from '@mui/icons-material/EventNoteRounded';
import AddCircleRoundedIcon from '@mui/icons-material/AddCircleRounded';
import TodayRoundedIcon from '@mui/icons-material/TodayRounded';
import GroupRoundedIcon from '@mui/icons-material/GroupRounded';
import PersonRoundedIcon from '@mui/icons-material/PersonRounded';
import type { Role } from '../api/types';

export interface NavItem {
  to: string;
  label: string;
  icon: ReactNode;
}

export function navItemsFor(role: Role): NavItem[] {
  const items: NavItem[] = [{ to: '/', label: 'Главная', icon: <DashboardRoundedIcon /> }];
  if (role === 'patient') {
    items.push({ to: '/book', label: 'Записаться на приём', icon: <AddCircleRoundedIcon /> });
  }
  if (role === 'admin') {
    items.push({ to: '/book', label: 'Новая запись', icon: <AddCircleRoundedIcon /> });
  }
  if (role === 'doctor') {
    items.push({ to: '/schedule', label: 'Моё расписание', icon: <TodayRoundedIcon /> });
  }
  items.push({
    to: '/appointments',
    label: role === 'admin' ? 'Все записи' : 'Мои записи',
    icon: <EventNoteRoundedIcon />,
  });
  if (role === 'admin') {
    items.push({ to: '/admin/users', label: 'Пользователи', icon: <GroupRoundedIcon /> });
  }
  items.push({ to: '/profile', label: 'Профиль', icon: <PersonRoundedIcon /> });
  return items;
}
