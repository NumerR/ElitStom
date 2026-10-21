import { Grid, Stack, Typography } from '@mui/material';
import { useCurrentUser } from '../auth/authContext';
import { RoleChip } from '../components/RoleChip';
import { firstName } from '../lib/labels';
import { QuickActions } from './dashboard/QuickActions';
import { TodayStatsCard } from './dashboard/TodayStatsCard';
import { UpcomingCard } from './dashboard/UpcomingCard';

function greeting(hour: number): string {
  if (hour >= 5 && hour < 12) return 'Доброе утро';
  if (hour >= 12 && hour < 18) return 'Добрый день';
  if (hour >= 18 && hour < 23) return 'Добрый вечер';
  return 'Доброй ночи';
}

export function DashboardPage() {
  const user = useCurrentUser();
  const hello = greeting(new Date().getHours());

  return (
    <Stack spacing={3}>
      <div>
        <Typography variant="h4" component="h1">
          {hello}, {firstName(user.full_name)}!
        </Typography>
        <Stack direction="row" spacing={1} sx={{ mt: 1, alignItems: 'center' }}>
          <RoleChip role={user.role} />
          <Typography color="text.secondary" variant="body2">
            {user.full_name}
          </Typography>
        </Stack>
      </div>

      <QuickActions role={user.role} />

      <Grid container spacing={2}>
        {user.role !== 'patient' && (
          <Grid size={{ xs: 12, md: 4 }}>
            <TodayStatsCard title={user.role === 'admin' ? 'Записей в клинике сегодня' : 'Моих приёмов сегодня'} />
          </Grid>
        )}
        <Grid size={{ xs: 12, md: user.role === 'patient' ? 12 : 8 }}>
          <UpcomingCard />
        </Grid>
      </Grid>
    </Stack>
  );
}
