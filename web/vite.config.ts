import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const vendorGroups: [string, RegExp][] = [
  ['react', /node_modules[\\/](react|react-dom|react-router|react-router-dom|scheduler)[\\/]/],
  ['mui-pickers', /node_modules[\\/](@mui[\\/]x-date-pickers|@mui[\\/]x-internals|dayjs)[\\/]/],
  ['mui', /node_modules[\\/](@mui|@emotion|@popperjs|react-transition-group)[\\/]/],
];

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: vendorGroups.map(([name, test]) => ({ name, test })),
        },
      },
    },
  },
});
