import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Toaster } from 'sonner'
import App from './App.tsx'
import { TooltipProvider } from './components/ui/index.ts'
import './index.css'
import { queryClient } from './lib/queries.ts'
import { applyTheme, resolvedDark, storedTheme } from './lib/theme.ts'

// Apply the theme before the first paint. This runs from the bundled module,
// so the strict CSP (no inline scripts) still holds.
applyTheme(storedTheme())

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <App />
        <Toaster position="bottom-right" richColors closeButton theme={resolvedDark(storedTheme()) ? 'dark' : 'light'} />
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
)
