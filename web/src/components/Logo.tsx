import { Box, Typography } from '@mui/material';
import MedicalServicesRoundedIcon from '@mui/icons-material/MedicalServicesRounded';

export function Logo({ size = 'medium' }: { size?: 'medium' | 'large' }) {
  const box = size === 'large' ? 48 : 36;
  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.25 }}>
      <Box
        aria-hidden
        sx={{
          width: box,
          height: box,
          borderRadius: size === 'large' ? 3.5 : 2.5,
          display: 'grid',
          placeItems: 'center',
          bgcolor: 'primary.main',
          color: 'primary.contrastText',
        }}
      >
        <MedicalServicesRoundedIcon fontSize={size === 'large' ? 'medium' : 'small'} />
      </Box>
      <Typography
        component="span"
        variant={size === 'large' ? 'h5' : 'h6'}
        sx={{ fontWeight: 700, letterSpacing: '-0.01em' }}
      >
        ElitStom
      </Typography>
    </Box>
  );
}
