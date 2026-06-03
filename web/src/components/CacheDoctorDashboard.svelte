<script lang="ts">
  import { fetchCacheReport } from '../lib/api'
  import type { CacheReport } from '../lib/api'

  let report: CacheReport | null = $state(null)
  let loading = $state(true)
  let error = $state('')

  async function load() {
    loading = true
    error = ''
    try {
      report = await fetchCacheReport()
    } catch (e) {
      error = (e instanceof Error ? e.message : String(e))
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })
</script>

<div class="cache-doctor">
  <div class="header">
    <h2>Cache Doctor</h2>
    <button onclick={load}>Refresh</button>
  </div>

  {#if loading}
    <p class="status">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if report}
    {#if report.full_body_evictions > 0}
      <div class="banner" data-testid="eviction-warning">
        ⚠ {report.full_body_evictions} full-body cache eviction{report.full_body_evictions > 1 ? 's' : ''} detected.
        Max miss spike: {report.max_miss_tokens.toLocaleString()} tokens.
      </div>
    {/if}

    <table class="summary">
      <tbody>
        <tr><td>Turns sampled</td><td>{report.total_usage_turns}</td></tr>
        <tr><td>Cache hit rate</td><td>{Math.round(report.cache_hit_rate * 100)}%</td></tr>
        <tr><td>Hit tokens</td><td>{report.cache_hit_tokens.toLocaleString()}</td></tr>
        <tr><td>Miss tokens</td><td>{report.cache_miss_tokens.toLocaleString()}</td></tr>
        <tr><td>Output tokens</td><td>{report.output_tokens.toLocaleString()}</td></tr>
        <tr><td>Cost (CNY)</td><td>¥{report.cost_cny.toFixed(4)}</td></tr>
        <tr class:danger={report.full_body_evictions > 0}>
          <td>Full-body evictions</td><td>{report.full_body_evictions}</td>
        </tr>
        <tr><td>Max miss spike</td><td>{report.max_miss_tokens.toLocaleString()} tok</td></tr>
      </tbody>
    </table>
  {/if}
</div>

<style>
  .cache-doctor { padding: 1rem; font-family: monospace; }
  .header { display: flex; align-items: center; gap: 1rem; margin-bottom: 1rem; }
  h2 { margin: 0; font-size: 1rem; color: #38bdf8; }
  button { background: #1e293b; border: 1px solid #334155; color: #94a3b8; padding: 0.25rem 0.75rem; cursor: pointer; border-radius: 4px; }
  .status { color: #64748b; }
  .error { color: #f87171; }
  .banner { background: #7c2d12; border: 1px solid #dc2626; border-radius: 4px; padding: 0.5rem 0.75rem; margin-bottom: 1rem; color: #fca5a5; font-size: 0.9em; }
  table.summary { border-collapse: collapse; width: 100%; max-width: 480px; }
  td { padding: 0.3rem 0.5rem; border-bottom: 1px solid #1e293b; color: #94a3b8; }
  td:last-child { text-align: right; color: #e2e8f0; }
  tr.danger td { color: #f87171; }
</style>
