import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const apiTarget = process.env.GJ_CONSOLE_API || 'http://localhost:8080';

// The server refuses session requests from another origin. In dev the page is
// served by Vite, so the proxy presents the API's own origin instead.
function apiProxy() {
  return {
    target: apiTarget,
    changeOrigin: true,
    configure(proxy) {
      proxy.on('proxyReq', (req) => {
        if (req.getHeader('origin')) {
          req.setHeader('origin', apiTarget);
        }
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: '/',
  appType: 'spa',
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 3001,
    proxy: {
      '^/api/': apiProxy(),
    },
  },
  preview: {
    port: 4173,
    proxy: {
      '^/api/': apiProxy(),
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.js',
    css: true,
  },
  build: {
    outDir: 'build',
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) {
            return undefined;
          }
          if (id.includes('/graphiql') || id.includes('/@graphiql')) {
            return 'graphiql';
          }
          if (id.includes('/swagger-ui-react') || id.includes('/swagger-client') || id.includes('/swagger-ui')) {
            return 'swagger';
          }
          if (id.includes('/react') || id.includes('/react-dom') || id.includes('/react-router')) {
            return 'react-vendor';
          }
          return undefined;
        },
      },
    },
  },
});
