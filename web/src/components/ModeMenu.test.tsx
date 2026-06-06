import { render, screen, fireEvent } from '@testing-library/react'
import { LocaleProvider } from '../lib/i18n'
import { ModeMenu } from './ModeMenu'

const wrap = (ui: React.ReactNode) => render(<LocaleProvider>{ui}</LocaleProvider>)

it('shows the current mode on the trigger', () => {
  wrap(<ModeMenu mode="plan" onChange={() => {}} />)
  expect(screen.getByTestId('mode-trigger')).toHaveTextContent('Plan')
})

it('opens and selects a mode', () => {
  const onChange = vi.fn()
  wrap(<ModeMenu mode="ask" onChange={onChange} />)
  fireEvent.click(screen.getByTestId('mode-trigger'))
  fireEvent.click(screen.getByTestId('mode-option-yolo'))
  expect(onChange).toHaveBeenCalledWith('yolo')
})
