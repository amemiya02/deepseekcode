<script lang="ts">
  export interface Message {
    role: 'user' | 'assistant' | 'tool'
    text: string
  }

  let { messages = [], isStreaming = false }: { messages: Message[]; isStreaming?: boolean } = $props()
</script>

<div class="conversation">
  {#each messages as msg}
    <div class="msg msg-{msg.role}">
      {#if msg.role === 'user'}
        <span class="label">You</span>
      {:else if msg.role === 'assistant'}
        <span class="label">dsc</span>
      {:else}
        <span class="label label-tool">tool</span>
      {/if}
      <pre class="text">{msg.text}</pre>
    </div>
  {/each}
  {#if isStreaming}
    <div class="streaming-indicator" data-testid="streaming-indicator">
      <span class="dot"></span><span class="dot"></span><span class="dot"></span>
    </div>
  {/if}
</div>

<style>
  .conversation { display: flex; flex-direction: column; gap: 0.75rem; padding: 1rem; font-family: monospace; }
  .msg { display: flex; flex-direction: column; gap: 0.25rem; }
  .label { font-size: 0.75rem; font-weight: 600; opacity: 0.6; text-transform: uppercase; }
  .label-tool { color: #0ea5e9; }
  .text { margin: 0; white-space: pre-wrap; word-break: break-word; }
  .msg-user .text { color: #e2e8f0; }
  .msg-assistant .text { color: #94a3b8; }
  .msg-tool .text { color: #38bdf8; font-size: 0.85em; }
  .streaming-indicator { display: flex; gap: 4px; padding: 0.5rem 0; }
  .dot { width: 6px; height: 6px; border-radius: 50%; background: #64748b; animation: pulse 1.2s infinite; }
  .dot:nth-child(2) { animation-delay: 0.2s; }
  .dot:nth-child(3) { animation-delay: 0.4s; }
  @keyframes pulse { 0%,80%,100% { opacity: 0.3; } 40% { opacity: 1; } }
</style>
