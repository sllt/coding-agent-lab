import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    rolldownOptions: {
      output: {
        // Long-lived vendor chunks cache across app releases.
        codeSplitting: {
          groups: [
            { name: 'react', test: /node_modules[\\/](react|react-dom|react-router|react-router-dom|scheduler)[\\/]/, priority: 20 },
            { name: 'ui', test: /node_modules[\\/](@radix-ui|@tanstack|lucide-react|sonner|class-variance-authority|clsx|tailwind-merge)[\\/]/, priority: 10 },
          ],
        },
      },
    },
  },
  server: {
    host: '127.0.0.1',
    port: 43118,
    strictPort: true,
    cors: false,
    headers: {
      // Dev server only: React Fast Refresh injects an inline preamble and HMR
      // uses a websocket. The Go server's production CSP allows neither.
      'Content-Security-Policy': "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws://127.0.0.1:43118; frame-ancestors 'none'; base-uri 'self'; object-src 'none'",
      'X-Content-Type-Options': 'nosniff',
      'Referrer-Policy': 'no-referrer',
    },
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:43117',
        changeOrigin: false,
      },
    },
  },
})
