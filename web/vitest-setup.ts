import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// Auto-unmount components between tests so the jsdom document stays clean.
afterEach(() => cleanup())
