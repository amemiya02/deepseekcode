import { render, screen, waitFor, within } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import CacheDoctorDashboard from './CacheDoctorDashboard.svelte'
import * as api from '../lib/api'

describe('CacheDoctorDashboard', () => {
  beforeEach(() => {
    vi.spyOn(api, 'fetchCacheReport').mockResolvedValue({
      total_usage_turns: 4,
      cache_hit_tokens: 38000,
      cache_miss_tokens: 14000,
      output_tokens: 900,
      cost_cny: 0.024,
      full_body_evictions: 2,
      max_miss_tokens: 16908,
      cache_hit_rate: 0.73,
    })
  })

  afterEach(() => vi.restoreAllMocks())

  it('renders hit rate and eviction count after load', async () => {
    render(CacheDoctorDashboard)
    // waitFor retries until the DOM settles after the async load() resolves
    await waitFor(() => {
      expect(screen.getByText('73%')).toBeTruthy()
    })
    // eviction count cell — query by its label row to be specific
    await waitFor(() => {
      const row = screen.getByText('Full-body evictions').closest('tr')!
      const cell = row.querySelector('td:last-child')!
      expect(cell.textContent).toBe('2')
    })
  })

  it('shows eviction warning banner when full_body_evictions > 0', async () => {
    render(CacheDoctorDashboard)
    // findByTestId retries until the element appears
    const banner = await screen.findByTestId('eviction-warning')
    expect(banner).toBeTruthy()
    // assert the banner content shows the eviction count and token spike
    expect(within(banner).getByText(/2 full-body cache eviction/)).toBeTruthy()
    expect(within(banner).getByText(/16,908/)).toBeTruthy()
  })

  it('renders error message without redundant Error: prefix when fetch fails', async () => {
    vi.spyOn(api, 'fetchCacheReport').mockRejectedValue(new Error('gateway error 500'))
    render(CacheDoctorDashboard)
    // The error element should show the message text only, not 'Error: gateway error 500'
    const errEl = await screen.findByText('gateway error 500')
    expect(errEl.className).toContain('error')
  })
})
