<script lang="ts">
  import { app } from '$lib/stores/app.svelte';
  import type { ComplexityDefaults, Complexity } from '$lib/types';

  let draft = $state<ComplexityDefaults | null>(null);
  $effect(() => {
    if (app.complexity && !draft) draft = JSON.parse(JSON.stringify(app.complexity));
  });

  const describe: Record<string, string> = {
    quick: 'Short answers and simple lookups. Least thinking, fewest tool steps.',
    standard: 'Everyday work.',
    deep: 'Research, coding and multi-step tasks. More thinking and tool steps; may use agent teams.',
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

  <div class="card overflow-hidden">
    <table class="data-table">
      <thead><tr><th>Level</th><th>Thinking</th><th>Max tool steps</th><th>Max output tokens</th><th>Agent teams</th></tr></thead>
      <tbody>
        {#each draft.presets as p}
          <tr>
            <td><div class="text-ink capitalize">{p.level}</div><div class="text-[11px] text-muted max-w-56">{describe[p.level] ?? ''}</div></td>
            <td>
              <select class="select" bind:value={p.reasoning}>
                {#each ['off', 'low', 'medium', 'high'] as r}<option value={r}>{r}</option>{/each}
              </select>
            </td>
            <td><input class="input w-24 py-1" type="number" min="1" max="100" bind:value={p.maxToolSteps} /></td>
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
