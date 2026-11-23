import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development (npm run dev) /api is proxied to the Go API on port 3000.
// In Docker, Nginx does the same proxying (see nginx.conf).
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://localhost:3000',
    },
  },
})
