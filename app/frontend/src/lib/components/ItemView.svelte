<script lang="ts">
  import { Brain, AlertTriangle, Inbox } from '@lucide/svelte';
  import type { Item } from '$lib/types';
  import { renderMarkdown } from '$lib/markdown';
  import ToolCall from './ToolCall.svelte';

  let { item, highlight = false }: { item: Item; highlight?: boolean } = $props();
  let showThinking = $state(false);
  const html = $derived(item.kind === 'agentMessage' ? renderMarkdown(item.text || '') : '');
</script>

<div id={`item-${item.id}`} class={highlight ? 'rounded-lg ring-1 ring-violet-500/60 ring-offset-4 ring-offset-zinc-950' : ''}>
  {#if item.kind === 'userMessage'}
    <div class="flex justify-end">
      <div class="max-w-[80%] bg-zinc-800 rounded-2xl rounded-br-md px-3.5 py-2 text-sm whitespace-pre-wrap selectable">{item.text}</div>
    </div>
  {:else if item.kind === 'agentMessage'}
    <div class="prose-chat">
      {@html html}
      {#if item.status === 'inProgress'}<span class="inline-block w-1.5 h-4 bg-violet-400 animate-pulse align-text-bottom"></span>{/if}
    </div>
  {:else if item.kind === 'reasoning'}
    <div class="text-xs">
      <button class="flex items-center gap-1.5 text-zinc-500 hover:text-zinc-300" onclick={() => (showThinking = !showThinking)}>
        <Brain class="w-3.5 h-3.5 {item.status === 'inProgress' ? 'animate-pulse text-violet-400' : ''}" />
        {item.status === 'inProgress' ? 'Thinking…' : 'Thought'}
        <span class="text-zinc-600">{showThinking ? 'hide' : 'show'}</span>
      </button>
      {#if showThinking && item.text}
        <div class="mt-1.5 pl-5 border-l border-zinc-800 text-zinc-500 whitespace-pre-wrap selectable">{item.text}</div>
      {/if}
    </div>
  {:else if item.kind === 'toolCall' && item.tool}
    <ToolCall {item} />
  {:else if item.kind === 'inboundEvent'}
    <div class="flex gap-2 text-xs text-zinc-400 border border-zinc-800 rounded-lg px-3 py-2 selectable">
      <Inbox class="w-3.5 h-3.5 mt-0.5 shrink-0" /><span class="whitespace-pre-wrap">{item.text}</span>
    </div>
  {:else if item.kind === 'error'}
    <div class="flex gap-2 text-sm text-red-300 bg-red-500/10 border border-red-500/20 rounded-lg px-3 py-2 selectable">
      <AlertTriangle class="w-4 h-4 mt-0.5 shrink-0" /><span class="whitespace-pre-wrap">{item.text}</span>
    </div>
  {:else if item.text}
    <div class="text-xs text-zinc-500 whitespace-pre-wrap">{item.text}</div>
  {/if}
</div>
