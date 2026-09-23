import { app } from './app.svelte';
import { projects } from './projects.svelte';
import type { FileChangeData, ProjectFileContent, RevertResult } from '$lib/types';

export type TabKind = 'file' | 'diff' | 'output';

export interface InspectorTab {
  id: string;
  kind: TabKind;
  title: string;
  path?: string;
  /** file */
  content?: string;
  truncated?: boolean;
  binary?: boolean;
  /** diff */
  change?: FileChangeData;
  /** tool output */
  text?: string;
  loading?: boolean;
  error?: string;
}

/** The third column: files, diffs and long tool output, read-only. */
class Inspector {
  tabs = $state<InspectorTab[]>([]);
  activeId = $state<string | null>(null);

  get open(): boolean {
    return this.tabs.length > 0;
  }

  get active(): InspectorTab | undefined {
    return this.tabs.find((t) => t.id === this.activeId);
  }

  private show(tab: InspectorTab) {
    const i = this.tabs.findIndex((t) => t.id === tab.id);
    if (i >= 0) this.tabs[i] = tab;
    else this.tabs = [...this.tabs, tab];
    this.activeId = tab.id;
  }

  close(id: string) {
    const i = this.tabs.findIndex((t) => t.id === id);
    this.tabs = this.tabs.filter((t) => t.id !== id);
    if (this.activeId === id) this.activeId = this.tabs[Math.max(0, i - 1)]?.id ?? null;
  }

  closeAll() {
    this.tabs = [];
    this.activeId = null;
  }

  /** Open a file from the project, at whatever it says on disk now. */
  async openFile(path: string) {
    const id = 'file:' + path;
    this.show({ id, kind: 'file', title: path.split('/').pop() || path, path, loading: true });
    const projectId = projects.activeId;
    if (!projectId) return;
    const r = await app.try<ProjectFileContent>('project/readFile', { projectId, path });
    const tab: InspectorTab = { id, kind: 'file', title: path.split('/').pop() || path, path };
    if (!r) tab.error = 'This file could not be read.';
    else Object.assign(tab, { content: r.content, truncated: r.truncated, binary: r.binary });
    this.show(tab);
  }

  /** Open one file change (a unified diff) from a turn. */
  openDiff(change: FileChangeData) {
    const id = `diff:${change.turnId ?? ''}:${change.path}`;
    this.show({ id, kind: 'diff', title: change.path.split('/').pop() || change.path, path: change.path, change });
  }

  /** Open the full output of a tool call. */
  openOutput(title: string, text: string) {
    this.show({ id: 'out:' + title + ':' + text.length, kind: 'output', title, text });
  }

  /** Undo the file changes of a turn, then refresh anything open. */
  async revertTurn(turnId: string, paths?: string[]) {
    const r = await app.try<RevertResult>('project/revertTurn', { turnId, paths });
    if (!r) return;
    if (r.reverted.length === 0) {
      app.toast('warn', r.reason || 'Nothing could be reverted.');
    } else {
      app.toast('info', `Put back ${r.reverted.join(', ')}.`);
    }
    if (r.reason && r.skipped?.length) app.toast('warn', r.reason);
    for (const tab of this.tabs) {
      if (tab.kind === 'file' && tab.path && r.reverted.includes(tab.path)) await this.openFile(tab.path);
    }
    await projects.loadDir('');
  }
}

export const inspector = new Inspector();
