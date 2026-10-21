import { IconButton, Tooltip } from '@mui/material';
import DarkModeRoundedIcon from '@mui/icons-material/DarkModeRounded';
import LightModeRoundedIcon from '@mui/icons-material/LightModeRounded';
import { useColorMode } from '../theme/colorMode';

export function ThemeToggleButton() {
  const { mode, toggle } = useColorMode();
  const label = mode === 'light' ? 'Включить тёмную тему' : 'Включить светлую тему';
  return (
    <Tooltip title={label}>
      <IconButton onClick={toggle} aria-label={label} color="inherit">
        {mode === 'light' ? <DarkModeRoundedIcon /> : <LightModeRoundedIcon />}
      </IconButton>
    </Tooltip>
  );
}
