<script lang="ts">
  import { onDestroy } from 'svelte'
  import ConversationView from './components/ConversationView.svelte'
  import CacheDoctorDashboard from './components/CacheDoctorDashboard.svelte'
  import SessionTimeline from './components/SessionTimeline.svelte'
  import { submitPrompt, GatewayClient } from './lib/api'
  import type { Message } from './components/ConversationView.svelte'

  type Tab = 'conversation' | 'cache' | 'timeline'
  let tab: Tab = $state('conversation')

  let messages: Message[] = $state([])
  let isStreaming = $state(false)
  let promptText = $state('')
  let sessionId = $state('')
  const client = new GatewayClient()

  // Close the EventSource when the component is destroyed (resource-leak fix).
  onDestroy(() => { client.close() })

  async function handleSubmit() {
    if (!promptText.trim()) return
    messages = [...messages, { role: 'user', text: promptText }]
    const draft = promptText
    promptText = ''
    isStreaming = true
    let assistantText = ''
    // Track the draft assistant bubble by its index so identity never depends on
    // text content (fixes the duplicate/drop bug when two messages share a prefix,
    // and the edge-case where text.length === 0 makes slice(0, 0) match nothing).
    let draftIndex = -1
    try {
      const reqId = await submitPrompt(draft, sessionId || undefined)
      sessionId = reqId
      client.openEventStream(reqId, {
        onDelta: (text) => {
          assistantText += text
          if (draftIndex === -1) {
            // First delta: append a new assistant bubble and remember its position.
            draftIndex = messages.length
            messages = [...messages, { role: 'assistant', text: assistantText }]
          } else {
            // Subsequent deltas: replace in-place by index.
            const updated = [...messages]
            updated[draftIndex] = { role: 'assistant', text: assistantText }
            messages = updated
          }
        },
        onTool: (name) => {
          messages = [...messages, { role: 'tool', text: name }]
        },
        onDone: () => { isStreaming = false; client.close() },
        onError: (_e: Event) => { isStreaming = false; client.close() },
      })
    } catch (err) {
      messages = [...messages, { role: 'assistant', text: `error: ${err}` }]
      isStreaming = false
    }
  }
</script>

<main>
  <nav>
    <button class:active={tab === 'conversation'} onclick={() => (tab = 'conversation')}>Conversation</button>
    <button class:active={tab === 'cache'} onclick={() => (tab = 'cache')}>Cache Doctor</button>
    <button class:active={tab === 'timeline'} onclick={() => (tab = 'timeline')}>Timeline</button>
  </nav>

  <div class="panel">
    {#if tab === 'conversation'}
      <ConversationView {messages} {isStreaming} />
      <form class="input-row" onsubmit={(e) => { e.preventDefault(); handleSubmit() }}>
        <input bind:value={promptText} placeholder="Type a prompt…" disabled={isStreaming} />
        <button type="submit" disabled={isStreaming}>Send</button>
      </form>
    {:else if tab === 'cache'}
      <CacheDoctorDashboard />
    {:else}
      <SessionTimeline {sessionId} />
    {/if}
  </div>
</main>

<style>
  main { display: flex; flex-direction: column; height: 100vh; background: #0f172a; color: #e2e8f0; }
  nav { display: flex; gap: 0; border-bottom: 1px solid #1e293b; }
  nav button { padding: 0.5rem 1.25rem; background: none; border: none; color: #94a3b8; cursor: pointer; font-family: monospace; }
  nav button.active { color: #38bdf8; border-bottom: 2px solid #38bdf8; }
  .panel { flex: 1; overflow: auto; display: flex; flex-direction: column; }
  .input-row { display: flex; gap: 0.5rem; padding: 0.75rem 1rem; border-top: 1px solid #1e293b; }
  .input-row input { flex: 1; background: #1e293b; border: 1px solid #334155; border-radius: 4px; padding: 0.5rem; color: #e2e8f0; font-family: monospace; }
  .input-row button { padding: 0.5rem 1rem; background: #0ea5e9; border: none; border-radius: 4px; color: white; cursor: pointer; font-family: monospace; }
  .input-row button:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
