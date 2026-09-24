<script lang="ts">
  import type { FileChangeData } from '$lib/types';

  let { change, max = 0 }: { change: FileChangeData; max?: number } = $props();

  interface Line {
    kind: 'add' | 'del' | 'ctx' | 'hunk' | 'meta';
    text: string;
  }

  const lines = $derived.by<Line[]>(() => {
    const out: Line[] = [];
    for (const raw of (change.diff ?? '').split('\n')) {
      if (raw.startsWith('--- ') || raw.startsWith('+++ ')) continue;
      if (raw.startsWith('@@')) out.push({ kind: 'hunk', text: raw });
      else if (raw.startsWith('+')) out.push({ kind: 'add', text: raw });
      else if (raw.startsWith('-')) out.push({ kind: 'del', text: raw });
      else if (raw.startsWith('\\')) out.push({ kind: 'meta', text: raw });
      else out.push({ kind: 'ctx', text: raw });
    }
    while (out.length && out.at(-1)!.text === '') out.pop();
    return max > 0 ? out.slice(0, max) : out;
  });

  const hidden = $derived(
    max > 0 ? Math.max(0, (change.diff ?? '').split('\n').length - max - 2) : 0,
  );
</script>

{#if change.diff}
  <div class="rounded-md border border-line overflow-hidden bg-paper">
    {#each lines as l}
      <div
        class="diff-line {l.kind === 'add' ? 'diff-add' : l.kind === 'del' ? 'diff-del' : l.kind === 'hunk' ? 'diff-hunk' : 'text-ink-soft'}"
      >{l.text || ' '}</div>
    {/each}
    {#if hidden > 0}
      <div class="diff-line diff-hunk">… {hidden} more lines</div>
    {/if}
    {#if change.truncated}
      <div class="diff-line diff-hunk">… this change was too large to show in full</div>
    {/if}
  </div>
{:else}
  <p class="text-xs text-muted">
    {change.action === 'deleted' ? 'File deleted.' : `${change.additions} lines added, ${change.deletions} removed (no diff kept).`}
  </p>
{/if}
