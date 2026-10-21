import { useState } from 'react';
import { AppBar, Box, Container, Drawer, IconButton, Toolbar } from '@mui/material';
import MenuRoundedIcon from '@mui/icons-material/MenuRounded';
import { Link as RouterLink, Outlet } from 'react-router-dom';
import { Logo } from './Logo';
import { NavDrawerContent } from './NavDrawerContent';
import { ThemeToggleButton } from './ThemeToggleButton';
import { UserMenu } from './UserMenu';

const DRAWER_WIDTH = 280;

export function AppShell() {
  const [mobileOpen, setMobileOpen] = useState(false);

  return (
    <Box sx={{ display: 'flex', minHeight: '100dvh' }}>
      <a href="#main-content" className="skip-link">
        Перейти к содержимому
      </a>
      <AppBar position="fixed" sx={{ zIndex: (t) => t.zIndex.drawer + 1 }}>
        <Toolbar sx={{ gap: 1 }}>
          <IconButton
            color="inherit"
            edge="start"
            aria-label="Открыть меню"
            onClick={() => setMobileOpen(true)}
            sx={{ display: { md: 'none' } }}
          >
            <MenuRoundedIcon />
          </IconButton>
          <Box
            component={RouterLink}
            to="/"
            aria-label="ElitStom — на главную"
            sx={{ color: 'inherit', textDecoration: 'none', borderRadius: 2 }}
          >
            <Logo />
          </Box>
          <Box sx={{ flexGrow: 1 }} />
          <ThemeToggleButton />
          <UserMenu />
        </Toolbar>
      </AppBar>

      <Drawer
        variant="temporary"
        open={mobileOpen}
        onClose={() => setMobileOpen(false)}
        ModalProps={{ keepMounted: true }}
        sx={{ display: { xs: 'block', md: 'none' }, '& .MuiDrawer-paper': { width: DRAWER_WIDTH } }}
      >
        <Toolbar>
          <Logo />
        </Toolbar>
        <NavDrawerContent onNavigate={() => setMobileOpen(false)} />
      </Drawer>
      <Drawer
        variant="permanent"
        open
        sx={{
          display: { xs: 'none', md: 'block' },
          width: DRAWER_WIDTH,
          flexShrink: 0,
          '& .MuiDrawer-paper': { width: DRAWER_WIDTH, boxSizing: 'border-box' },
        }}
      >
        <Toolbar />
        <NavDrawerContent />
      </Drawer>

      <Box
        component="main"
        id="main-content"
        tabIndex={-1}
        sx={{ flexGrow: 1, minWidth: 0, outline: 'none' }}
      >
        <Toolbar />
        <Container maxWidth="lg" sx={{ py: { xs: 3, md: 4 }, px: { xs: 2, sm: 3 } }}>
          <Outlet />
        </Container>
      </Box>
    </Box>
  );
}
