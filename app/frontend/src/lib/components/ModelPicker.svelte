<script lang="ts">
  import { app } from '$lib/stores/app.svelte';
  import type { Complexity, ConfiguredModel, ModelSelection } from '$lib/types';

  let { value, onchange, compact = false }: { value: ModelSelection; onchange: (v: ModelSelection) => void; compact?: boolean } = $props();

  const levels: { id: Complexity; label: string; hint: string }[] = [
    { id: 'auto', label: 'Auto', hint: 'Pick per message' },
    { id: 'quick', label: 'Quick', hint: 'Fast, little or no thinking' },
    { id: 'standard', label: 'Standard', hint: 'Balanced' },
    { id: 'deep', label: 'Deep', hint: 'More thinking and tool steps' },
  ];

  const modelKey = $derived(value.provider && value.model ? `${value.provider}/${value.model}` : value.provider ? `${value.provider}/` : '');
  const defaultComplexity = $derived(app.complexity?.default || 'auto');

  const providerNames: Record<string, string> = {
    claude: 'Claude', openai: 'OpenAI', gemini: 'Gemini', openai_compatible: 'OpenAI-compatible',
  };
  const pools = $derived(app.routing?.pools ?? []);
  const configured = $derived(app.routing?.models ?? []);
  const configuredByProvider = $derived.by(() => {
    const groups: Record<string, ConfiguredModel[]> = {};
    for (const model of configured) {
      if (model.enabled) (groups[model.provider] ??= []).push(model);
    }
    return Object.entries(groups);
  });

  function setModel(k: string) {
    if (!k) {
      onchange({ ...value, provider: undefined, model: undefined, credentialId: undefined });
      return;
    }
    const i = k.indexOf('/');
    const provider = k.slice(0, i);
    const model = k.slice(i + 1) || undefined;
    onchange({ ...value, provider, model, credentialId: undefined });
  }

  function setComplexity(c: Complexity) {
    onchange({ ...value, complexity: c === value.complexity ? undefined : c });
  }

</script>

<div class="flex items-center gap-1.5 flex-wrap">
  <select class="select max-w-56" title="Model" value={modelKey} onchange={(e) => setModel(e.currentTarget.value)}>
    <option value="">Default model</option>
    {#if pools.some((p) => p.enabled)}
      <optgroup label="Model pools">
        {#each pools.filter((p) => p.enabled) as pool}
          <option value={`pool/${pool.id}`}>{pool.name}</option>
        {/each}
      </optgroup>
    {/if}
    {#each configuredByProvider as [provider, models]}
      <optgroup label={providerNames[provider] ?? provider}>
        {#each models as m}
          <option value={`${m.provider}/${m.model}`}>{m.name || m.model}</option>
        {/each}
      </optgroup>
    {/each}
    {#if value.provider && !configured.some((m) => `${m.provider}/${m.model}` === modelKey) && !pools.some((p) => `pool/${p.id}` === modelKey)}
      <option value={modelKey}>{value.provider}/{value.model ?? 'default'} (not configured)</option>
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
</div>
