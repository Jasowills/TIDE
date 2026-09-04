import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // Object form with explicit ws:true — the live event stream (/v1/stream)
      // upgrades through this proxy; the string shorthand leaves websocket
      // support to defaults, which silently drops the stream.
      '/v1': { target: 'http://localhost:8080', changeOrigin: true, ws: true },
    },
  },
});
