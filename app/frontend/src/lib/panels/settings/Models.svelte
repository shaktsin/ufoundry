<script lang="ts">
  import { RefreshCw, Eye, EyeOff } from '@lucide/svelte';
  import { app } from '$lib/stores/app.svelte';
  import { errMsg } from '$lib/format';
  import type { Model } from '$lib/types';

  let all = $state<Model[]>([]);
  let editing = $state<string | null>(null);
  let price = $state({ in: '', cached: '', out: '' });

  async function load() {
    const r = await app.try<{ models: Model[] }>('model/list', { includeHidden: true });
    if (r) all = r.models;
  }

  $effect(() => {
    if (app.conn === 'open') void load();
  });

  const byProvider = $derived.by(() => {
    const g: Record<string, Model[]> = {};
    for (const m of all) (g[m.provider] ??= []).push(m);
    return Object.entries(g);
  });

  async function setHidden(m: Model, hidden: boolean) {
    await app.try('model/setHidden', { provider: m.provider, model: m.id, hidden });
    await load();
    await app.refreshCatalog();
  }

  async function refreshFrom(provider: string) {
    const c = app.credentials.find((x) => x.provider === provider && x.enabled && x.isDefault) ?? app.credentials.find((x) => x.provider === provider && x.enabled);
    if (!c) {
      app.toast('error', 'Add a key for this provider first.');
      return;
    }
    try {
      await app.call('model/refresh', { credentialId: c.id }, 60_000);
      app.toast('info', 'Model list updated from the provider.');
    } catch (e) {
      app.toast('error', errMsg(e));
    }
    await load();
    await app.refreshCatalog();
  }

  function edit(m: Model) {
    editing = `${m.provider}/${m.id}`;
    price = { in: String(m.inputPerMTok || ''), cached: String(m.cachedInputPerMTok || ''), out: String(m.outputPerMTok || '') };
  }

  async function savePrice(m: Model) {
    const n = (s: string) => parseFloat(s || '0') || 0;
    await app.try('model/setPrice', { provider: m.provider, model: m.id, inputPerMTok: n(price.in), cachedInputPerMTok: n(price.cached), outputPerMTok: n(price.out) }, 'Price saved.');
    editing = null;
    await load();
    await app.refreshCatalog();
  }

  const names: Record<string, string> = { claude: 'Anthropic Claude', openai: 'OpenAI', gemini: 'Google Gemini', openai_compatible: 'OpenAI-compatible' };
</script>

<p class="text-sm text-muted mb-3">Hidden models don't appear in the model picker. Prices are USD per million tokens and drive the cost numbers in Usage.</p>

{#each byProvider as [prov, list]}
  <div class="flex items-center mt-5 mb-2">
    <h2 class="text-xs font-semibold uppercase tracking-wider text-muted">{names[prov] ?? prov}</h2>
    <button class="btn-ghost btn-sm ml-auto" onclick={() => refreshFrom(prov)}><RefreshCw class="w-3.5 h-3.5" />Fetch from provider</button>
  </div>
  <div class="card overflow-hidden">
    <table class="data-table">
      <thead><tr><th>Model</th><th>Capabilities</th><th class="text-right">In</th><th class="text-right">Cached</th><th class="text-right">Out</th><th></th></tr></thead>
      <tbody>
        {#each list as m (m.id)}
          {@const key = `${m.provider}/${m.id}`}
          <tr class={m.hidden ? 'opacity-50' : ''}>
            <td><div class="text-ink">{m.displayName || m.id}</div><div class="text-[11px] text-muted font-mono selectable">{m.id}</div></td>
            <td class="text-[11px] text-muted">{[m.supportsTools && 'tools', m.supportsImages && 'images', m.supportsReasoning && 'thinking'].filter(Boolean).join(' · ')}</td>
            {#if editing === key}
              <td class="text-right"><input class="input w-20 text-right py-1" bind:value={price.in} /></td>
              <td class="text-right"><input class="input w-20 text-right py-1" bind:value={price.cached} /></td>
              <td class="text-right"><input class="input w-20 text-right py-1" bind:value={price.out} /></td>
              <td class="text-right whitespace-nowrap"><button class="btn-ghost btn-sm" onclick={() => (editing = null)}>Cancel</button><button class="btn-primary btn-sm" onclick={() => savePrice(m)}>Save</button></td>
            {:else}
              <td class="text-right text-muted">{m.inputPerMTok ? `$${m.inputPerMTok}` : '—'}</td>
              <td class="text-right text-muted">{m.cachedInputPerMTok ? `$${m.cachedInputPerMTok}` : '—'}</td>
              <td class="text-right text-muted">{m.outputPerMTok ? `$${m.outputPerMTok}` : '—'}</td>
              <td class="text-right whitespace-nowrap">
                <button class="btn-ghost btn-sm" onclick={() => edit(m)}>Edit price</button>
                <button class="btn-ghost btn-sm" title={m.hidden ? 'Show in picker' : 'Hide from picker'} aria-label={m.hidden ? 'Show' : 'Hide'} onclick={() => setHidden(m, !m.hidden)}>
                  {#if m.hidden}<EyeOff class="w-3.5 h-3.5" />{:else}<Eye class="w-3.5 h-3.5" />{/if}
                </button>
              </td>
            {/if}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/each}
