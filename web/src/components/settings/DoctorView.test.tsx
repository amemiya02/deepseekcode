import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { DoctorView } from './DoctorView'
import * as system from '../../lib/system'

afterEach(() => vi.restoreAllMocks())

describe('DoctorView', () => {
  it('lists each check with a pass/fail indicator', async () => {
    vi.spyOn(system, 'fetchDoctor').mockResolvedValue({
      allOk: false,
      checks: [
        { name: 'api key', ok: true, detail: 'present' },
        { name: 'base url', ok: false, detail: 'empty' },
      ],
    })
    render(<DoctorView />)
    await waitFor(() => {
      expect(screen.getByText('api key')).toBeInTheDocument()
      expect(screen.getByText('base url')).toBeInTheDocument()
    })
    const failRow = screen.getByText('base url').closest('[data-ok]')!
    expect(failRow.getAttribute('data-ok')).toBe('false')
  })

  it('re-runs checks on the Run again button', async () => {
    const spy = vi.spyOn(system, 'fetchDoctor').mockResolvedValue({ allOk: true, checks: [{ name: 'x', ok: true, detail: 'ok' }] })
    render(<DoctorView />)
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1))
    await userEvent.click(screen.getByRole('button', { name: /run|again|refresh/i }))
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2))
  })
})
