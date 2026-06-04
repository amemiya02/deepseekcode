import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
// Self-hosted fonts — ensures correct rendering offline (no Google Fonts CDN required).
// IBM Plex Sans SC is not published on fontsource; PingFang SC / system CJK glyphs
// cover the gap; the CDN <link> in index.html remains as a progressive enhancement.
import '@fontsource/ibm-plex-sans/400.css'
import '@fontsource/ibm-plex-sans/500.css'
import '@fontsource/ibm-plex-sans/600.css'
import '@fontsource/ibm-plex-sans/700.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import '@fontsource/jetbrains-mono/600.css'
import '@fontsource/jetbrains-mono/700.css'
import './app.css'
import { App } from './App'
import { installMockGateway } from './lib/mockGateway'

installMockGateway()

createRoot(document.getElementById('app')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
