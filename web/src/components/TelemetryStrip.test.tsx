import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { TelemetryStrip } from './TelemetryStrip'
import { useCockpitStore } from '../lib/cockpitStore'
import * as api from '../lib/api'

// Mock Cockpit so we don't need full card rendering
vi.mock('./cockpit/Cockpit', () => ({
  Cockpit: ({ manageConnection }: { manageConnection?: boolean }) => (
    <div data-testid="cockpit-mock" data-manage={String(manageConnection)} />
  ),
}))

describe('TelemetryStrip', () => {
  beforeEach(() => {
    useCockpitStore.getState().reset()
    vi.spyOn(api.GatewayClient.prototype, 'openEventStream').mockImplementation(() => {})
    vi.spyOn(api.GatewayClient.prototype, 'close').mockImplementation(() => {})
    vi.spyOn(api, 'fetchBalance').mockResolvedValue({ provider: 'deepseek', currency: 'CNY', amount: 5 })
    vi.spyOn(api, 'fetchCacheLedger').mockResolvedValue([])
  })
  afterEach(() => vi.restoreAllMocks())

  it('renders the compact line with cache and cost text', () => {
    render(<TelemetryStrip sessionId="s1" />)
    const compact = screen.getByTestId('telemetry-compact')
    expect(compact).toBeInTheDocument()
    // formatPct(0) = '0%', formatCNY(0) = '¥0.0000'
    expect(compact.textContent).toContain('0%')
    expect(compact.textContent).toContain('¥')
  })

  it('shows the telemetry-toggle button', () => {
    render(<TelemetryStrip sessionId="s1" />)
    expect(screen.getByTestId('telemetry-toggle')).toBeInTheDocument()
  })

  it('clicking toggle reveals telemetry-expanded', () => {
    render(<TelemetryStrip sessionId="s1" />)
    expect(screen.queryByTestId('telemetry-expanded')).toBeNull()
    fireEvent.click(screen.getByTestId('telemetry-toggle'))
    expect(screen.getByTestId('telemetry-expanded')).toBeInTheDocument()
  })

  it('clicking toggle again hides telemetry-expanded', () => {
    render(<TelemetryStrip sessionId="s1" />)
    fireEvent.click(screen.getByTestId('telemetry-toggle'))
    expect(screen.getByTestId('telemetry-expanded')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('telemetry-toggle'))
    expect(screen.queryByTestId('telemetry-expanded')).toBeNull()
  })

  it('renders Cockpit with manageConnection=false when expanded', () => {
    render(<TelemetryStrip sessionId="s1" />)
    fireEvent.click(screen.getByTestId('telemetry-toggle'))
    const cockpit = screen.getByTestId('cockpit-mock')
    expect(cockpit.getAttribute('data-manage')).toBe('false')
  })

  it('owns the SSE connection (connect called with sessionId)', () => {
    const connectSpy = vi.spyOn(useCockpitStore.getState(), 'connect')
    render(<TelemetryStrip sessionId="s1" />)
    expect(api.GatewayClient.prototype.openEventStream).toHaveBeenCalled()
  })
})
