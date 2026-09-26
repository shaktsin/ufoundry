<script lang="ts">
  import { app } from '$lib/stores/app.svelte';
  import type { ComplexityDefaults, Complexity } from '$lib/types';

  let draft = $state<ComplexityDefaults | null>(null);
  $effect(() => {
    if (app.complexity && !draft) draft = JSON.parse(JSON.stringify(app.complexity));
  });

  const describe: Record<string, string> = {
    quick: 'Short answers and simple lookups. Least thinking.',
    standard: 'Everyday work.',
    deep: 'Research, coding and multi-step tasks. More thinking; may use agent teams.',
  };

  async function save() {
    if (!draft) return;
    const r = await app.try<ComplexityDefaults>('complexity/setDefaults', draft, 'Complexity settings saved.');
    if (r) {
      app.complexity = r;
      draft = JSON.parse(JSON.stringify(r));
    }
  }
</script>

{#if draft}
  <div class="card p-4 mb-4">
    <label class="label" for="cx-default">Default complexity for new chats</label>
    <select id="cx-default" class="input w-64" bind:value={draft.default}>
      {#each ['auto', 'quick', 'standard', 'deep'] as c}<option value={c as Complexity}>{c === 'auto' ? 'Auto (pick per message)' : c[0].toUpperCase() + c.slice(1)}</option>{/each}
    </select>
    <p class="text-xs text-muted mt-2">Auto looks at each message (length, code, words like “research” or “step by step”) and picks Quick, Standard or Deep.</p>
  </div>

  <div class="card p-4 mb-4">
    <h3 class="text-sm font-medium text-ink">Turn safety limits</h3>
    <p class="text-xs text-muted mt-1 mb-3">The agent keeps working until it finishes or reaches a real safety budget. These limits apply to every reasoning level.</p>
    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <label class="label">Time limit (minutes)<input class="input mt-1" type="number" min="1" max="480" step="5" bind:value={draft.limits.maxDurationMinutes} /></label>
      <label class="label">Token budget<input class="input mt-1" type="number" min="10000" max="10000000" step="10000" bind:value={draft.limits.maxTokens} /></label>
      <label class="label">Cost budget (USD)<input class="input mt-1" type="number" min="0.01" max="10000" step="1" bind:value={draft.limits.maxCostUsd} /></label>
      <label class="label">Tool-round safety ceiling<input class="input mt-1" type="number" min="1" max="1000" step="10" bind:value={draft.limits.maxToolRounds} /></label>
    </div>
  </div>

  <div class="card overflow-x-auto">
    <table class="data-table min-w-[680px]">
      <thead><tr><th>Level</th><th>Thinking</th><th>Max output tokens</th><th>Agent teams</th></tr></thead>
      <tbody>
        {#each draft.presets as p}
          <tr>
            <td><div class="text-ink capitalize">{p.level}</div><div class="text-[11px] text-muted max-w-56">{describe[p.level] ?? ''}</div></td>
            <td>
              <select class="select" bind:value={p.reasoning}>
                {#each ['off', 'low', 'medium', 'high'] as r}<option value={r}>{r}</option>{/each}
              </select>
            </td>
            <td><input class="input w-28 py-1" type="number" min="256" step="256" bind:value={p.maxOutputTokens} /></td>
            <td>
              <select class="select" bind:value={p.multiAgent}>
                <option value="never">never</option><option value="teamRouteOnly">matching routes only</option><option value="allowed">allowed</option>
              </select>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  <div class="flex justify-end gap-2 mt-3">
    <button class="btn-ghost" onclick={() => (draft = JSON.parse(JSON.stringify(app.complexity)))}>Reset</button>
    <button class="btn-primary" onclick={save}>Save</button>
  </div>
{/if}
