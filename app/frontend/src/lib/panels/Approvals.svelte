<script lang="ts">
  import { ShieldCheck } from '@lucide/svelte';
  import { app } from '$lib/stores/app.svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { prettyJSON, relTime } from '$lib/format';
</script>

<div class="flex items-center mb-4">
  <h1 class="page-title">Approvals</h1>
  <span class="ml-3 text-xs text-zinc-500">Risky actions wait here until you decide. Telegram and other admin clients see the same requests; the first answer wins.</span>
</div>

{#if app.approvals.length === 0}
  <div class="card p-10 text-center">
    <ShieldCheck class="w-8 h-8 mx-auto text-emerald-400 mb-2" />
    <p class="text-sm text-zinc-400">Nothing waiting for approval.</p>
  </div>
{/if}

<div class="space-y-3">
  {#each app.approvals as a (a.id)}
    <div class="card p-4">
      <div class="flex items-start gap-3">
        <span class="risk-{a.risk} mt-0.5">{a.risk}</span>
        <div class="flex-1 min-w-0">
          <div class="text-sm text-zinc-100 selectable">{a.actionSummary || a.tool}</div>
          <div class="text-xs text-zinc-500 mt-0.5">
            <code class="font-mono">{a.tool}</code>{a.reason ? ` · ${a.reason}` : ''} · {relTime(a.createdAt)}
          </div>
          <details class="mt-2">
            <summary class="text-xs text-zinc-500 cursor-default">Arguments</summary>
            <pre class="mt-1 font-mono text-[11px] text-zinc-400 bg-zinc-950 rounded p-2 whitespace-pre-wrap break-all max-h-60 overflow-auto">{prettyJSON(a.args)}</pre>
          </details>
        </div>
        <div class="flex gap-2 shrink-0">
          <button class="btn-ghost btn-sm" onclick={() => chat.open(a.threadId)}>Open chat</button>
          <button class="btn-outline btn-sm" onclick={() => app.respondApproval(a.id, false)}>Deny</button>
          <button class="btn-primary btn-sm" onclick={() => app.respondApproval(a.id, true)}>Approve</button>
        </div>
      </div>
    </div>
  {/each}
</div>
