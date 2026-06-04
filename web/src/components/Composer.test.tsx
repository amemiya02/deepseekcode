import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { Composer, type ComposerPayload } from './Composer'
import * as workspace from '../lib/workspace'
import { LocaleProvider } from '../lib/i18n'

const commands = [{ name: 'review', description: 'Review the diff', kind: 'builtin' as const }]

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('Composer', () => {
  it('typing "/" opens the SlashMenu', async () => {
    wrap(<Composer streaming={false} mode="ask" commands={commands} onSend={() => {}} onCancel={() => {}} onModeChange={() => {}} />)
    await userEvent.type(screen.getByTestId('composer-input'), '/rev')
    expect(await screen.findByText('/review')).toBeInTheDocument()
  })

  it('typing "@" opens the FileMenu from fetchFiles', async () => {
    vi.spyOn(workspace, 'fetchFiles').mockResolvedValue({ entries: [{ name: 'main.go', path: 'main.go', is_dir: false }] })
    wrap(<Composer streaming={false} mode="ask" commands={commands} onSend={() => {}} onCancel={() => {}} onModeChange={() => {}} />)
    await userEvent.type(screen.getByTestId('composer-input'), 'see @ma')
    expect(await screen.findByText('main.go')).toBeInTheDocument()
  })

  it('Enter sends the payload and clears the input', async () => {
    const sent: ComposerPayload[] = []
    wrap(<Composer streaming={false} mode="ask" commands={commands} onSend={(p) => sent.push(p)} onCancel={() => {}} onModeChange={() => {}} />)
    const input = screen.getByTestId('composer-input')
    await userEvent.type(input, 'fix the bug{Enter}')
    expect(sent).toHaveLength(1)
    expect(sent[0].text).toBe('fix the bug')
    expect((input as HTMLTextAreaElement).value).toBe('')
  })

  it('streaming: the send control becomes Stop and fires onCancel', async () => {
    let cancelled = false
    wrap(<Composer streaming mode="ask" commands={commands} onSend={() => {}} onCancel={() => { cancelled = true }} onModeChange={() => {}} />)
    await userEvent.click(screen.getByTestId('send-stop'))
    expect(cancelled).toBe(true)
  })

  it('Shift+Tab cycles the autonomy mode', async () => {
    const modes: string[] = []
    wrap(<Composer streaming={false} mode="ask" commands={commands} onSend={() => {}} onCancel={() => {}} onModeChange={(m) => modes.push(m)} />)
    const input = screen.getByTestId('composer-input')
    input.focus()
    await userEvent.keyboard('{Shift>}{Tab}{/Shift}')
    expect(modes).toEqual(['auto-edit'])
  })
})
