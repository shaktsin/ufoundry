<script lang="ts">
  import { app } from '$lib/stores/app.svelte';
  import { projects } from '$lib/stores/projects.svelte';
  import type { Approval } from '$lib/types';

  let { approval, compact = false }: { approval: Approval; compact?: boolean } = $props();
  let remember = $state(false);

  const canRemember = $derived(!!projects.active);
</script>

<div class="px-3 py-2 {compact ? 'border-t border-clay/30' : ''} bg-clay-soft/20 flex flex-wrap items-center gap-2">
  <span class="text-amber-warm flex-1 min-w-40 selectable">
    {approval.actionSummary || `Run ${approval.tool}`}{approval.reason ? ` — ${approval.reason}` : ''}
  </span>
  {#if canRemember}
    <label class="flex items-center gap-1.5 text-[11px] text-muted" title="Answer the same way next time in this project">
      <input type="checkbox" bind:checked={remember} />
      always in {projects.active?.name}
    </label>
  {/if}
  <button class="btn-outline btn-sm" onclick={() => app.respondApproval(approval.id, false, remember)}>Deny</button>
  <button class="btn-primary btn-sm" onclick={() => app.respondApproval(approval.id, true, remember)}>Approve</button>
</div>
