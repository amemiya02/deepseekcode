import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { AskCard } from './AskCard'
import type { AskAnswer, AskRequest } from '../lib/api'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

const single: AskRequest = {
  id: 'ask-1',
  questions: [
    {
      question: 'Use TypeScript?',
      header: 'lang',
      multiple: false,
      options: [{ label: 'Yes', description: '' }, { label: 'No', description: '' }],
    },
  ],
}
const multi: AskRequest = {
  id: 'ask-2',
  questions: [
    {
      question: 'Pick targets',
      header: 'targets',
      multiple: true,
      options: [{ label: 'web', description: '' }, { label: 'cli', description: '' }],
    },
  ],
}

describe('AskCard', () => {
  it('renders the question and its options', () => {
    wrap(<AskCard request={single} />)
    expect(screen.getByText('Use TypeScript?')).toBeInTheDocument()
    expect(screen.getByText('Yes')).toBeInTheDocument()
    expect(screen.getByText('No')).toBeInTheDocument()
  })

  it('single-select option click emits choices immediately', async () => {
    const answers: AskAnswer[] = []
    const user = userEvent.setup()
    wrap(<AskCard request={single} onAnswer={(a) => answers.push(a)} />)
    await user.click(screen.getByText('Yes'))
    expect(answers).toEqual([{ id: 'ask-1', questionIndex: 0, choices: ['Yes'] }])
  })

  it('multi-select accumulates then submits', async () => {
    const answers: AskAnswer[] = []
    const user = userEvent.setup()
    wrap(<AskCard request={multi} onAnswer={(a) => answers.push(a)} />)
    await user.click(screen.getByText('web'))
    await user.click(screen.getByText('cli'))
    await user.click(screen.getByTestId('ask-submit-0'))
    expect(answers).toEqual([{ id: 'ask-2', questionIndex: 0, choices: ['web', 'cli'] }])
  })

  it('free-text submit emits the typed text', async () => {
    const answers: AskAnswer[] = []
    const user = userEvent.setup()
    wrap(<AskCard request={single} onAnswer={(a) => answers.push(a)} />)
    await user.type(screen.getByTestId('ask-text-0'), 'maybe later')
    await user.click(screen.getByTestId('ask-text-submit-0'))
    expect(answers).toEqual([{ id: 'ask-1', questionIndex: 0, text: 'maybe later' }])
  })

  it('"Just chat" calls onDismiss', async () => {
    let dismissed = false
    const user = userEvent.setup()
    wrap(<AskCard request={single} onDismiss={() => { dismissed = true }} />)
    await user.click(screen.getByTestId('ask-chat'))
    expect(dismissed).toBe(true)
  })
})
