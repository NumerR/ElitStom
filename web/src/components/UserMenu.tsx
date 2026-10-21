import { useState } from 'react';
import {
  Avatar,
  Box,
  Divider,
  IconButton,
  ListItemIcon,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
} from '@mui/material';
import PersonRoundedIcon from '@mui/icons-material/PersonRounded';
import LogoutRoundedIcon from '@mui/icons-material/LogoutRounded';
import { useNavigate } from 'react-router-dom';
import { useAuth, useCurrentUser } from '../auth/authContext';
import { initials, roleLabels } from '../lib/labels';

export function UserMenu() {
  const user = useCurrentUser();
  const { logout } = useAuth();
  const navigate = useNavigate();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const open = Boolean(anchor);

  const handleLogout = async () => {
    setAnchor(null);
    await logout();
    navigate('/login', { replace: true });
  };

  return (
    <>
      <Tooltip title="Учётная запись">
        <IconButton
          onClick={(e) => setAnchor(e.currentTarget)}
          aria-label="Меню учётной записи"
          aria-controls={open ? 'user-menu' : undefined}
          aria-haspopup="true"
          aria-expanded={open ? 'true' : undefined}
          sx={{ p: 0.5 }}
        >
          <Avatar sx={{ width: 36, height: 36, bgcolor: 'primary.main', fontSize: 15 }}>
            {initials(user.full_name)}
          </Avatar>
        </IconButton>
      </Tooltip>
      <Menu
        id="user-menu"
        anchorEl={anchor}
        open={open}
        onClose={() => setAnchor(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        slotProps={{ paper: { sx: { minWidth: 240, mt: 1 } } }}
      >
        <Box sx={{ px: 2, py: 1.5 }}>
          <Typography variant="subtitle2" noWrap>
            {user.full_name}
          </Typography>
          <Typography variant="body2" color="text.secondary" noWrap>
            {user.email}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {roleLabels[user.role]}
          </Typography>
        </Box>
        <Divider />
        <MenuItem
          onClick={() => {
            setAnchor(null);
            navigate('/profile');
          }}
        >
          <ListItemIcon>
            <PersonRoundedIcon fontSize="small" />
          </ListItemIcon>
          Профиль
        </MenuItem>
        <MenuItem onClick={handleLogout}>
          <ListItemIcon>
            <LogoutRoundedIcon fontSize="small" />
          </ListItemIcon>
          Выйти
        </MenuItem>
      </Menu>
    </>
  );
}
