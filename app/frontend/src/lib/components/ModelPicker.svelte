<script lang="ts">
  import { app } from '$lib/stores/app.svelte';
  import type { Complexity, ModelSelection } from '$lib/types';

  let { value, onchange, compact = false }: { value: ModelSelection; onchange: (v: ModelSelection) => void; compact?: boolean } = $props();

  const levels: { id: Complexity; label: string; hint: string }[] = [
    { id: 'auto', label: 'Auto', hint: 'Pick per message' },
    { id: 'quick', label: 'Quick', hint: 'Fast, little or no thinking' },
    { id: 'standard', label: 'Standard', hint: 'Balanced' },
    { id: 'deep', label: 'Deep', hint: 'More thinking and tool steps' },
  ];

  const modelKey = $derived(value.provider && value.model ? `${value.provider}/${value.model}` : value.provider ? `${value.provider}/` : '');
  const keys = $derived(value.provider ? app.credentialsFor(value.provider) : []);
  const defaultComplexity = $derived(app.complexity?.default || 'auto');

  function setModel(k: string) {
    if (!k) {
      onchange({ ...value, provider: undefined, model: undefined, credentialId: undefined });
      return;
    }
    const i = k.indexOf('/');
    const provider = k.slice(0, i);
    const model = k.slice(i + 1) || undefined;
    const credentialId = provider === value.provider ? value.credentialId : undefined;
    onchange({ ...value, provider, model, credentialId });
  }

  function setComplexity(c: Complexity) {
    onchange({ ...value, complexity: c === value.complexity ? undefined : c });
  }

  function setKey(id: string) {
    onchange({ ...value, credentialId: id || undefined });
  }
</script>

<div class="flex items-center gap-1.5 flex-wrap">
  <select class="select max-w-56" title="Model" value={modelKey} onchange={(e) => setModel(e.currentTarget.value)}>
    <option value="">Default model</option>
    {#each app.usableProviders as p}
      <optgroup label={p.displayName}>
        <option value={`${p.id}/`}>{p.displayName} default{p.defaultModel ? ` (${p.defaultModel})` : ''}</option>
        {#each app.modelsFor(p.id) as m}
          <option value={`${p.id}/${m.id}`}>{m.displayName || m.id}</option>
        {/each}
      </optgroup>
    {/each}
    {#if value.provider && !app.usableProviders.some((p) => p.id === value.provider)}
      <option value={modelKey}>{value.provider}/{value.model ?? 'default'} (no key)</option>
    {/if}
  </select>

  <div class="inline-flex rounded-md border border-line-strong overflow-hidden" role="group" aria-label="Complexity">
    {#each levels as l}
      {@const on = (value.complexity || '') === l.id}
      {@const inherited = !value.complexity && defaultComplexity === l.id}
      <button
        class="px-2 py-1 text-xs transition-colors {on ? 'bg-clay text-white' : inherited ? 'bg-raised text-ink' : 'text-muted hover:bg-raised'}"
        title={`${l.hint}${inherited ? ' (default)' : ''}`}
        aria-pressed={on}
        onclick={() => setComplexity(l.id)}
      >
        {compact ? l.label[0] : l.label}
      </button>
    {/each}
  </div>

  {#if keys.length > 1}
    <select class="select" title="API key" value={value.credentialId ?? ''} onchange={(e) => setKey(e.currentTarget.value)}>
      <option value="">Default key</option>
      {#each keys as k}
        <option value={k.id}>{k.label} ····{k.last4}</option>
      {/each}
    </select>
  {/if}
</div>
