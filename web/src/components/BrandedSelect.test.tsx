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
  // options are now div[role="option"] — getByRole('option') still works
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

it('listbox has aria-activedescendant pointing at the focused item', () => {
  render(<BrandedSelect value="a" options={options} onChange={() => {}} testid="sel" ariaLabel="Test select" />)
  fireEvent.click(screen.getByTestId('sel'))
  const listbox = screen.getByRole('listbox')
  // On open the current item (index 0 = "a") becomes active
  const activeId = listbox.getAttribute('aria-activedescendant')
  expect(activeId).toBeTruthy()
  const activeEl = document.getElementById(activeId!)
  expect(activeEl).toHaveTextContent('Option A')
})

it('listbox has an accessible label', () => {
  render(<BrandedSelect value="a" options={options} onChange={() => {}} testid="sel" ariaLabel="Pick a thing" />)
  fireEvent.click(screen.getByTestId('sel'))
  const listbox = screen.getByRole('listbox')
  expect(listbox).toHaveAttribute('aria-label', 'Pick a thing')
})

it('option items are div[role="option"], not buttons', () => {
  render(<BrandedSelect value="a" options={options} onChange={() => {}} testid="sel" />)
  fireEvent.click(screen.getByTestId('sel'))
  const opts = screen.getAllByRole('option')
  opts.forEach((el) => {
    expect(el.tagName.toLowerCase()).toBe('div')
  })
})

describe('keyboard navigation', () => {
  it('ArrowDown moves active descendant to next item', () => {
    render(<BrandedSelect value="a" options={options} onChange={() => {}} testid="sel" />)
    fireEvent.click(screen.getByTestId('sel'))
    const listbox = screen.getByRole('listbox')
    // starts at index 0 (Option A)
    fireEvent.keyDown(listbox, { key: 'ArrowDown' })
    const activeId = listbox.getAttribute('aria-activedescendant')!
    expect(document.getElementById(activeId)).toHaveTextContent('Option B')
  })

  it('ArrowUp moves active descendant to previous item', () => {
    render(<BrandedSelect value="c" options={options} onChange={() => {}} testid="sel" />)
    fireEvent.click(screen.getByTestId('sel'))
    const listbox = screen.getByRole('listbox')
    // starts at index 2 (Option C)
    fireEvent.keyDown(listbox, { key: 'ArrowUp' })
    const activeId = listbox.getAttribute('aria-activedescendant')!
    expect(document.getElementById(activeId)).toHaveTextContent('Option B')
  })

  it('Home moves to first item', () => {
    render(<BrandedSelect value="c" options={options} onChange={() => {}} testid="sel" />)
    fireEvent.click(screen.getByTestId('sel'))
    const listbox = screen.getByRole('listbox')
    fireEvent.keyDown(listbox, { key: 'Home' })
    const activeId = listbox.getAttribute('aria-activedescendant')!
    expect(document.getElementById(activeId)).toHaveTextContent('Option A')
  })

  it('End moves to last item', () => {
    render(<BrandedSelect value="a" options={options} onChange={() => {}} testid="sel" />)
    fireEvent.click(screen.getByTestId('sel'))
    const listbox = screen.getByRole('listbox')
    fireEvent.keyDown(listbox, { key: 'End' })
    const activeId = listbox.getAttribute('aria-activedescendant')!
    expect(document.getElementById(activeId)).toHaveTextContent('Option C')
  })

  it('Enter selects the active item and closes', () => {
    const onChange = vi.fn()
    render(<BrandedSelect value="a" options={options} onChange={onChange} testid="sel" />)
    fireEvent.click(screen.getByTestId('sel'))
    const listbox = screen.getByRole('listbox')
    // Move to Option B then Enter
    fireEvent.keyDown(listbox, { key: 'ArrowDown' })
    fireEvent.keyDown(listbox, { key: 'Enter' })
    expect(onChange).toHaveBeenCalledWith('b')
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('Escape closes the listbox without selecting', () => {
    const onChange = vi.fn()
    render(<BrandedSelect value="a" options={options} onChange={onChange} testid="sel" />)
    fireEvent.click(screen.getByTestId('sel'))
    fireEvent.keyDown(screen.getByRole('listbox'), { key: 'Escape' })
    expect(screen.queryByRole('listbox')).toBeNull()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('ArrowDown on trigger opens menu', () => {
    render(<BrandedSelect value="a" options={options} onChange={() => {}} testid="sel" />)
    const trigger = screen.getByTestId('sel')
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    expect(screen.getByRole('listbox')).toBeInTheDocument()
  })
})
