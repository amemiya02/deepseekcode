<script lang="ts">
  import type { Epoch } from '../lib/api'

  let {
    sessionId = '',
    epochs = [],
  }: { sessionId?: string; epochs?: Epoch[] } = $props()
</script>

<div class="timeline">
  <div class="header">
    <h2>Session Timeline</h2>
    {#if sessionId}
      <span class="session-id">{sessionId.slice(0, 8)}</span>
    {/if}
  </div>

  {#if epochs.length === 0}
    <p data-testid="timeline-empty" class="empty">No epochs yet. Start a conversation.</p>
  {:else}
    <ol class="epochs">
      {#each epochs as epoch, i}
        <li class="epoch" class:compacted={epoch.compacted}>
          <span class="num">{i + 1}</span>
          <span class="id">{epoch.id}</span>
          <span class="turns">{epoch.turns} turn{epoch.turns !== 1 ? 's' : ''}</span>
          {#if epoch.compacted}
            <span class="badge" data-testid="badge-compacted">compacted</span>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}
</div>

<style>
  .timeline { padding: 1rem; font-family: monospace; }
  .header { display: flex; align-items: center; gap: 1rem; margin-bottom: 1rem; }
  h2 { margin: 0; font-size: 1rem; color: #38bdf8; }
  .session-id { font-size: 0.75rem; color: #64748b; }
  .empty { color: #64748b; }
  .epochs { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.5rem; }
  .epoch { display: flex; align-items: center; gap: 0.75rem; padding: 0.4rem 0.5rem; border-left: 2px solid #334155; }
  .epoch.compacted { border-left-color: #f59e0b; }
  .num { color: #475569; width: 1.5rem; text-align: right; flex-shrink: 0; }
  .id { color: #94a3b8; flex: 1; }
  .turns { color: #64748b; font-size: 0.85em; }
  .badge { background: #78350f; color: #fcd34d; font-size: 0.75em; padding: 0.1rem 0.4rem; border-radius: 3px; }
</style>
