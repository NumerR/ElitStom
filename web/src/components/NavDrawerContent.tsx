import { Box, List, ListItemButton, ListItemIcon, ListItemText, Typography } from '@mui/material';
import { Link as RouterLink, matchPath, useLocation } from 'react-router-dom';
import { useCurrentUser } from '../auth/authContext';
import { roleLabels } from '../lib/labels';
import { navItemsFor } from './navItems';

export function NavDrawerContent({ onNavigate }: { onNavigate?: () => void }) {
  const user = useCurrentUser();
  const { pathname } = useLocation();
  const items = navItemsFor(user.role);

  return (
    <Box component="nav" aria-label="Основная навигация" sx={{ px: 1.5, py: 2 }}>
      <Typography
        variant="overline"
        color="text.secondary"
        sx={{ px: 2, display: 'block', lineHeight: 2.5 }}
      >
        {roleLabels[user.role]}
      </Typography>
      <List disablePadding sx={{ display: 'grid', gap: 0.5 }}>
        {items.map((item) => {
          const selected =
            item.to === '/' ? pathname === '/' : !!matchPath({ path: item.to, end: false }, pathname);
          return (
            <ListItemButton
              key={item.to}
              component={RouterLink}
              to={item.to}
              selected={selected}
              aria-current={selected ? 'page' : undefined}
              onClick={onNavigate}
              sx={{ minHeight: 52, px: 2 }}
            >
              <ListItemIcon sx={{ minWidth: 40 }}>{item.icon}</ListItemIcon>
              <ListItemText
                primary={item.label}
                slotProps={{ primary: { sx: { fontWeight: selected ? 600 : 500 } } }}
              />
            </ListItemButton>
          );
        })}
      </List>
    </Box>
  );
}
