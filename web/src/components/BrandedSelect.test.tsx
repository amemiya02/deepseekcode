import { render, screen, fireEvent } from '@testing-library/react'
import { BrandedSelect } from './BrandedSelect'

const options = [
  { value: 'a', label: 'Option A' },
  { value: 'b', label: 'Option B' },
  { value: 'c', label: 'Option C' },
]

it('renders trigger showing the current option label', () => {
  render(<BrandedSelect value="b" options={options} onChange={() => {}} testid="sel" />)
  expect(screen.getByTestId('sel')).toHaveTextContent('Option B')
})

it('clicking trigger opens a listbox', () => {
  render(<BrandedSelect value="a" options={options} onChange={() => {}} testid="sel" />)
  expect(screen.queryByRole('listbox')).toBeNull()
  fireEvent.click(screen.getByTestId('sel'))
  expect(screen.getByRole('listbox')).toBeInTheDocument()
})

it('clicking an option calls onChange and closes the listbox', () => {
  const onChange = vi.fn()
  render(<BrandedSelect value="a" options={options} onChange={onChange} testid="sel" />)
  fireEvent.click(screen.getByTestId('sel'))
  fireEvent.click(screen.getByRole('option', { name: 'Option C' }))
  expect(onChange).toHaveBeenCalledWith('c')
  expect(screen.queryByRole('listbox')).toBeNull()
})

it('the current option has aria-selected=true', () => {
  render(<BrandedSelect value="b" options={options} onChange={() => {}} testid="sel" />)
  fireEvent.click(screen.getByTestId('sel'))
  const selected = screen.getAllByRole('option').find((el) => el.getAttribute('aria-selected') === 'true')
  expect(selected).toHaveTextContent('Option B')
})

it('trigger has aria-haspopup="listbox" and aria-expanded reflects open state', () => {
  render(<BrandedSelect value="a" options={options} onChange={() => {}} testid="sel" />)
  const trigger = screen.getByTestId('sel')
  expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  fireEvent.click(trigger)
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
})
