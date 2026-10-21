import type { ReactNode } from 'react';
import { Box, Card, CardActionArea, CardContent, Grid, Typography } from '@mui/material';
import AddCircleRoundedIcon from '@mui/icons-material/AddCircleRounded';
import TodayRoundedIcon from '@mui/icons-material/TodayRounded';
import GroupRoundedIcon from '@mui/icons-material/GroupRounded';
import EventNoteRoundedIcon from '@mui/icons-material/EventNoteRounded';
import { Link as RouterLink } from 'react-router-dom';
import type { Role } from '../../api/types';

interface Action {
  to: string;
  title: string;
  description: string;
  icon: ReactNode;
}

function actionsFor(role: Role): Action[] {
  switch (role) {
    case 'patient':
      return [
        { to: '/book', title: 'Записаться на приём', description: 'Выберите врача и удобное время', icon: <AddCircleRoundedIcon /> },
        { to: '/appointments', title: 'Мои записи', description: 'Предстоящие и прошедшие приёмы', icon: <EventNoteRoundedIcon /> },
      ];
    case 'doctor':
      return [
        { to: '/schedule', title: 'Моё расписание', description: 'Приёмы на сегодня и другие дни', icon: <TodayRoundedIcon /> },
        { to: '/appointments', title: 'Мои записи', description: 'Все записи ко мне', icon: <EventNoteRoundedIcon /> },
      ];
    case 'admin':
      return [
        { to: '/book', title: 'Новая запись', description: 'Записать пациента к врачу', icon: <AddCircleRoundedIcon /> },
        { to: '/admin/users', title: 'Пользователи', description: 'Роли, доступ и новые учётные записи', icon: <GroupRoundedIcon /> },
      ];
  }
}

export function QuickActions({ role }: { role: Role }) {
  return (
    <Grid container spacing={2}>
      {actionsFor(role).map((a, i) => (
        <Grid key={a.to} size={{ xs: 12, sm: 6 }}>
          <Card
            sx={{
              height: '100%',
              ...(i === 0 && {
                bgcolor: (t) => t.palette.m3.secondaryContainer,
                color: (t) => t.palette.m3.onSecondaryContainer,
                borderColor: 'transparent',
              }),
            }}
          >
            <CardActionArea component={RouterLink} to={a.to} sx={{ height: '100%' }}>
              <CardContent sx={{ display: 'flex', gap: 2, alignItems: 'center' }}>
                <Box
                  aria-hidden
                  sx={{
                    width: 48,
                    height: 48,
                    borderRadius: 3,
                    display: 'grid',
                    placeItems: 'center',
                    flexShrink: 0,
                    bgcolor: i === 0 ? 'rgba(255,255,255,0.4)' : (t) => t.palette.m3.primaryContainer,
                    color: i === 0 ? 'inherit' : (t) => t.palette.m3.onPrimaryContainer,
                  }}
                >
                  {a.icon}
                </Box>
                <Box sx={{ minWidth: 0 }}>
                  <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                    {a.title}
                  </Typography>
                  <Typography variant="body2" sx={{ opacity: 0.8 }}>
                    {a.description}
                  </Typography>
                </Box>
              </CardContent>
            </CardActionArea>
          </Card>
        </Grid>
      ))}
    </Grid>
  );
}
