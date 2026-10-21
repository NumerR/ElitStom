import { alpha, createTheme, type Theme } from '@mui/material/styles';
import { ruRU } from '@mui/material/locale';
import { ruRU as pickersRuRU } from '@mui/x-date-pickers/locales';

export type ColorMode = 'light' | 'dark';

export interface M3Tokens {
  surface: string;
  surfaceContainerLowest: string;
  surfaceContainerLow: string;
  surfaceContainer: string;
  surfaceContainerHigh: string;
  surfaceContainerHighest: string;
  onSurface: string;
  onSurfaceVariant: string;
  outline: string;
  outlineVariant: string;
  primaryContainer: string;
  onPrimaryContainer: string;
  secondaryContainer: string;
  onSecondaryContainer: string;
  tertiaryContainer: string;
  onTertiaryContainer: string;
}

declare module '@mui/material/styles' {
  interface Palette {
    m3: M3Tokens;
  }
  interface PaletteOptions {
    m3?: M3Tokens;
  }
}

declare module '@mui/material/Button' {
  interface ButtonPropsVariantOverrides {
    tonal: true;
  }
}

const light = {
  primary: '#00658f',
  onPrimary: '#ffffff',
  secondary: '#8a5100',
  error: '#ba1a1a',
  success: '#2e6c35',
  warning: '#8c5000',
  info: '#2e5da8',
  background: '#f6fafe',
  m3: {
    surface: '#f6fafe',
    surfaceContainerLowest: '#ffffff',
    surfaceContainerLow: '#f0f4f9',
    surfaceContainer: '#eaeef3',
    surfaceContainerHigh: '#e4e9ed',
    surfaceContainerHighest: '#dee3e8',
    onSurface: '#171c1f',
    onSurfaceVariant: '#41484d',
    outline: '#71787e',
    outlineVariant: '#c1c7ce',
    primaryContainer: '#c7e7ff',
    onPrimaryContainer: '#001e2e',
    secondaryContainer: '#ffdcbe',
    onSecondaryContainer: '#2c1600',
    tertiaryContainer: '#d4e3ff',
    onTertiaryContainer: '#001c3a',
  } satisfies M3Tokens,
};

const dark = {
  primary: '#84cfff',
  onPrimary: '#00344c',
  secondary: '#ffb870',
  error: '#ffb4ab',
  success: '#94d895',
  warning: '#ffb870',
  info: '#a8c8ff',
  background: '#0f1417',
  m3: {
    surface: '#0f1417',
    surfaceContainerLowest: '#0a0f12',
    surfaceContainerLow: '#171c1f',
    surfaceContainer: '#1b2023',
    surfaceContainerHigh: '#262b2e',
    surfaceContainerHighest: '#313539',
    onSurface: '#dee3e8',
    onSurfaceVariant: '#c1c7ce',
    outline: '#8b9198',
    outlineVariant: '#41484d',
    primaryContainer: '#004c6d',
    onPrimaryContainer: '#c7e7ff',
    secondaryContainer: '#693c00',
    onSecondaryContainer: '#ffdcbe',
    tertiaryContainer: '#1f4785',
    onTertiaryContainer: '#d4e3ff',
  } satisfies M3Tokens,
};

const tonalColors = ['primary', 'secondary', 'error', 'success', 'warning', 'info'] as const;

export function createAppTheme(mode: ColorMode): Theme {
  const t = mode === 'light' ? light : dark;

  const base = createTheme(
    {
      palette: {
        mode,
        primary: { main: t.primary, contrastText: t.onPrimary },
        secondary: { main: t.secondary },
        error: { main: t.error },
        success: { main: t.success },
        warning: { main: t.warning },
        info: { main: t.info },
        background: {
          default: t.background,
          paper: mode === 'light' ? t.m3.surfaceContainerLowest : t.m3.surfaceContainerLow,
        },
        text: { primary: t.m3.onSurface, secondary: t.m3.onSurfaceVariant },
        divider: t.m3.outlineVariant,
        m3: t.m3,
      },
      shape: { borderRadius: 12 },
      typography: {
        fontFamily: '"Inter", "Roboto", "Helvetica", "Arial", sans-serif',
        h1: { fontSize: '2.75rem', fontWeight: 600, letterSpacing: '-0.02em' },
        h2: { fontSize: '2.25rem', fontWeight: 600, letterSpacing: '-0.015em' },
        h3: { fontSize: '1.875rem', fontWeight: 600, letterSpacing: '-0.01em' },
        h4: { fontSize: '1.625rem', fontWeight: 600, letterSpacing: '-0.01em' },
        h5: { fontSize: '1.375rem', fontWeight: 600 },
        h6: { fontSize: '1.125rem', fontWeight: 600 },
        subtitle1: { fontWeight: 500 },
        subtitle2: { fontWeight: 600 },
        button: { textTransform: 'none', fontWeight: 600, letterSpacing: '0.01em' },
      },
    },
    ruRU,
    pickersRuRU,
  );

  const focusRing = {
    outline: `2px solid ${base.palette.primary.main}`,
    outlineOffset: 2,
  };

  return createTheme(base, {
    components: {
      MuiCssBaseline: {
        styleOverrides: {
          body: { backgroundColor: t.background },
          '*:focus-visible': { outline: `2px solid ${base.palette.primary.main}`, outlineOffset: 2 },
          '.skip-link': {
            position: 'fixed',
            top: 8,
            left: 8,
            zIndex: 2000,
            padding: '8px 16px',
            borderRadius: 999,
            background: base.palette.primary.main,
            color: base.palette.primary.contrastText,
            fontWeight: 600,
            textDecoration: 'none',
            transform: 'translateY(-200%)',
            '&:focus': { transform: 'none' },
          },
        },
      },
      MuiButtonBase: {
        styleOverrides: { root: { '&.Mui-focusVisible': focusRing } },
      },
      MuiButton: {
        defaultProps: { disableElevation: true },
        styleOverrides: {
          root: {
            borderRadius: 999,
            paddingInline: 20,
            minHeight: 40,
            variants: tonalColors.map((color) => ({
              props: { variant: 'tonal', color },
              style: {
                backgroundColor:
                  color === 'primary'
                    ? t.m3.primaryContainer
                    : alpha(base.palette[color].main, mode === 'light' ? 0.14 : 0.22),
                color:
                  color === 'primary'
                    ? t.m3.onPrimaryContainer
                    : mode === 'light'
                      ? base.palette[color].dark
                      : base.palette[color].light,
                '&:hover': {
                  backgroundColor: alpha(base.palette[color].main, mode === 'light' ? 0.22 : 0.3),
                },
                '&.Mui-disabled': {
                  backgroundColor: alpha(t.m3.onSurface, 0.12),
                  color: alpha(t.m3.onSurface, 0.38),
                },
              },
            })),
          },
          sizeSmall: { paddingInline: 14, minHeight: 32 },
          sizeLarge: { paddingInline: 24, minHeight: 48 },
        },
      },
      MuiIconButton: {
        styleOverrides: { root: { borderRadius: 999 } },
      },
      MuiFab: {
        styleOverrides: { root: { borderRadius: 16, boxShadow: base.shadows[2] } },
      },
      MuiPaper: {
        defaultProps: { elevation: 0 },
        styleOverrides: {
          root: { backgroundImage: 'none' },
          rounded: { borderRadius: 16 },
          outlined: { borderColor: t.m3.outlineVariant },
        },
      },
      MuiCard: {
        defaultProps: { elevation: 0 },
        styleOverrides: {
          root: {
            borderRadius: 16,
            backgroundColor: mode === 'light' ? t.m3.surfaceContainerLowest : t.m3.surfaceContainerLow,
            border: `1px solid ${alpha(t.m3.outlineVariant, mode === 'light' ? 0.6 : 0.5)}`,
            boxShadow:
              mode === 'light'
                ? '0 1px 2px rgba(0, 32, 48, 0.06), 0 1px 3px 1px rgba(0, 32, 48, 0.04)'
                : 'none',
          },
        },
      },
      MuiCardActionArea: {
        styleOverrides: { root: { borderRadius: 16 } },
      },
      MuiCardHeader: {
        styleOverrides: { title: { fontWeight: 600, fontSize: '1.05rem' } },
      },
      MuiChip: {
        styleOverrides: {
          root: { borderRadius: 999, fontWeight: 500 },
        },
      },
      MuiDialog: {
        styleOverrides: {
          paper: { borderRadius: 28, backgroundColor: t.m3.surfaceContainerHigh },
        },
      },
      MuiDialogTitle: {
        styleOverrides: { root: { fontSize: '1.375rem', fontWeight: 600, paddingTop: 24 } },
      },
      MuiDialogActions: {
        styleOverrides: { root: { padding: '8px 24px 20px', gap: 8 } },
      },
      MuiMenu: {
        styleOverrides: {
          paper: {
            borderRadius: 12,
            backgroundColor: t.m3.surfaceContainer,
            boxShadow: base.shadows[3],
          },
        },
      },
      MuiOutlinedInput: {
        styleOverrides: { root: { borderRadius: 12 } },
      },
      MuiTextField: {
        defaultProps: { variant: 'outlined', fullWidth: true },
      },
      MuiAppBar: {
        defaultProps: { elevation: 0, color: 'inherit' },
        styleOverrides: {
          root: {
            backgroundColor: t.m3.surface,
            color: t.m3.onSurface,
            borderBottom: `1px solid ${alpha(t.m3.outlineVariant, 0.6)}`,
          },
        },
      },
      MuiDrawer: {
        styleOverrides: {
          paper: { backgroundColor: t.m3.surfaceContainerLow, borderRight: 'none' },
        },
      },
      MuiListItemButton: {
        styleOverrides: {
          root: {
            borderRadius: 999,
            '&.Mui-selected': {
              backgroundColor: t.m3.secondaryContainer,
              color: t.m3.onSecondaryContainer,
              '& .MuiListItemIcon-root': { color: t.m3.onSecondaryContainer },
              '&:hover': { backgroundColor: alpha(t.secondary, 0.3) },
            },
          },
        },
      },
      MuiTableCell: {
        styleOverrides: {
          root: { borderBottomColor: t.m3.outlineVariant },
          head: { fontWeight: 600, color: t.m3.onSurfaceVariant, whiteSpace: 'nowrap' },
        },
      },
      MuiTableRow: {
        styleOverrides: {
          root: { '&:last-child td': { borderBottom: 0 } },
        },
      },
      MuiAlert: {
        styleOverrides: { root: { borderRadius: 12 } },
      },
      MuiTab: {
        styleOverrides: { root: { textTransform: 'none', fontWeight: 600, minHeight: 48 } },
      },
      MuiStepIcon: {
        styleOverrides: { root: { '&.Mui-completed': { color: t.primary } } },
      },
      MuiSkeleton: {
        styleOverrides: { rounded: { borderRadius: 12 } },
      },
      MuiTooltip: {
        styleOverrides: { tooltip: { borderRadius: 8, fontSize: '0.8125rem' } },
      },
    },
  });
}
