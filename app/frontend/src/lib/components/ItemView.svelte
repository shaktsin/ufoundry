<script lang="ts">
  import { Brain, AlertTriangle, Inbox, MessageSquarePlus, FilePlus2, FilePen, FileX2 } from '@lucide/svelte';
  import type { Item, FileChangeData } from '$lib/types';
  import { renderMarkdown } from '$lib/markdown';
  import { chat, type ThreadView } from '$lib/stores/chat.svelte';
  import { inspector } from '$lib/stores/inspector.svelte';
  import ToolCall from './ToolCall.svelte';
  import DiffView from './DiffView.svelte';

  let { item, highlight = false, view }: { item: Item; highlight?: boolean; view?: ThreadView } = $props();
  let showThinking = $state(false);
  let showDiff = $state(false);

  const html = $derived(item.kind === 'agentMessage' ? renderMarkdown(item.text || '') : '');
  const change = $derived(item.kind === 'fileChange' ? (item.data as FileChangeData) : null);
  const canAsk = $derived(!!item.text && (item.kind === 'agentMessage' || item.kind === 'userMessage'));

  function askAbout() {
    chat.startSide(item.text ?? '');
  }
</script>

<div
  id={`item-${item.id}`}
  class="group/item {highlight ? 'rounded-lg ring-1 ring-clay/60 ring-offset-4 ring-offset-paper' : ''}"
>
  {#if item.kind === 'userMessage'}
    <div class="flex justify-end gap-1 items-start">
      {#if canAsk}
        <button
          class="opacity-0 group-hover/item:opacity-100 transition-opacity btn-ghost btn-sm mt-1"
          title="Ask about this in a side chat"
          aria-label="Ask about this"
          onclick={askAbout}
        >
          <MessageSquarePlus class="w-3.5 h-3.5" />
        </button>
      {/if}
      <div class="max-w-[80%] bg-raised rounded-2xl rounded-br-md px-3.5 py-2 text-sm whitespace-pre-wrap selectable">{item.text}</div>
    </div>
  {:else if item.kind === 'agentMessage'}
    <div class="relative">
      <div class="prose-chat">
        {@html html}
        {#if item.status === 'inProgress'}<span class="inline-block w-1.5 h-4 bg-clay animate-pulse align-text-bottom"></span>{/if}
      </div>
      {#if canAsk && item.status !== 'inProgress'}
        <button
          class="opacity-0 group-hover/item:opacity-100 transition-opacity btn-ghost btn-sm mt-1"
          onclick={askAbout}
        >
          <MessageSquarePlus class="w-3.5 h-3.5" />Ask about this
        </button>
      {/if}
    </div>
  {:else if item.kind === 'reasoning'}
    <div class="text-xs">
      <button class="flex items-center gap-1.5 text-muted hover:text-ink" onclick={() => (showThinking = !showThinking)}>
        <Brain class="w-3.5 h-3.5 {item.status === 'inProgress' ? 'animate-pulse text-clay' : ''}" />
        {item.status === 'inProgress' ? 'Thinking…' : 'Thought'}
        <span class="text-faint">{showThinking ? 'hide' : 'show'}</span>
      </button>
      {#if showThinking && item.text}
        <div class="mt-1.5 pl-5 border-l border-line text-muted whitespace-pre-wrap selectable">{item.text}</div>
      {/if}
    </div>
  {:else if item.kind === 'toolCall' && item.tool}
    <ToolCall {item} />
  {:else if change}
    <div class="rounded-lg border border-line bg-surface/60 text-xs overflow-hidden">
      <div class="flex items-center gap-2 px-3 py-1.5">
        {#if change.action === 'created'}
          <FilePlus2 class="w-3.5 h-3.5 text-sage" />
        {:else if change.action === 'deleted'}
          <FileX2 class="w-3.5 h-3.5 text-rust" />
        {:else}
          <FilePen class="w-3.5 h-3.5 text-clay" />
        {/if}
        <button class="font-mono text-ink hover:underline truncate" onclick={() => inspector.openFile(change.path)}>
          {change.path}
        </button>
        <span class="text-sage">+{change.additions}</span>
        <span class="text-rust">−{change.deletions}</span>
        <div class="ml-auto flex items-center gap-1">
          <button class="text-faint hover:text-ink" onclick={() => (showDiff = !showDiff)}>
            {showDiff ? 'hide diff' : 'diff'}
          </button>
          <button class="text-faint hover:text-ink" onclick={() => inspector.openDiff(change)}>open</button>
        </div>
      </div>
      {#if showDiff}
        <div class="border-t border-line p-2"><DiffView {change} max={40} /></div>
      {/if}
    </div>
  {:else if item.kind === 'inboundEvent'}
    <div class="flex gap-2 text-xs text-muted border border-line rounded-lg px-3 py-2 selectable">
      <Inbox class="w-3.5 h-3.5 mt-0.5 shrink-0" /><span class="whitespace-pre-wrap">{item.text}</span>
    </div>
  {:else if item.kind === 'error'}
    <div class="flex gap-2 text-sm text-rust bg-rust-soft border border-rust/30 rounded-lg px-3 py-2 selectable">
      <AlertTriangle class="w-4 h-4 mt-0.5 shrink-0" /><span class="whitespace-pre-wrap">{item.text}</span>
    </div>
  {:else if item.text}
    <div class="text-xs text-muted whitespace-pre-wrap">{item.text}</div>
  {/if}
</div>
