import { app } from './app.svelte';
import { errMsg } from '$lib/format';
import type { Attachment, Item, ModelSelection, SearchHit, Thread, Turn } from '$lib/types';

function sortThreads(list: Thread[]): Thread[] {
  return [...list].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    return b.updatedAt.localeCompare(a.updatedAt);
  });
}

class ChatState {
  threads = $state<Thread[]>([]);
  showArchived = $state(false);
  query = $state('');
  hits = $state<SearchHit[]>([]);
  activeId = $state<string | null>(null);
  turns = $state<Turn[]>([]);
  items = $state<Item[]>([]);
  loading = $state(false);
  sending = $state(false);
  /** Selection for the next message (thread settings for an existing thread). */
  selection = $state<ModelSelection>({});
  highlightItem = $state<string | null>(null);

  constructor() {
    const r = app.rpc;
    r.on('thread/updated', (p: { thread: Thread }) => this.upsertThread(p.thread));
    r.on('turn/started', (p: { turn: Turn }) => this.upsertTurn(p.turn));
    r.on('turn/completed', (p: { turn: Turn }) => {
      this.upsertTurn(p.turn);
      if (p.turn.threadId === this.activeId) this.sending = false;
    });
    r.on('item/started', (p: { item: Item }) => this.upsertItem(p.item));
    r.on('item/completed', (p: { item: Item }) => this.upsertItem(p.item));
    r.on('item/delta', (p: { threadId: string; itemId: string; text?: string; output?: string }) => {
      if (p.threadId !== this.activeId) return;
      const it = this.items.find((i) => i.id === p.itemId);
      if (!it) return;
      if (p.text) it.text = (it.text || '') + p.text;
      if (p.output && it.tool) it.tool.output = (it.tool.output || '') + p.output;
    });
    app.onConnected(() => this.reload());
  }

  get active(): Thread | undefined {
    return this.threads.find((t) => t.id === this.activeId);
  }

  get running(): Turn | undefined {
    return this.turns.find((t) => t.status === 'running');
  }

  async reload() {
    await this.loadThreads();
    if (this.activeId) await this.open(this.activeId, false);
  }

  async loadThreads() {
    try {
      const r = await app.call<{ threads: Thread[] }>('thread/list', { archived: this.showArchived, limit: 200 });
      this.threads = sortThreads(r.threads ?? []);
    } catch (e) {
      app.toast('error', errMsg(e));
    }
  }

  private upsertThread(t: Thread) {
    const matches = t.archived === this.showArchived;
    const rest = this.threads.filter((x) => x.id !== t.id);
    this.threads = sortThreads(matches ? [...rest, t] : rest);
  }

  private upsertTurn(t: Turn) {
    if (t.threadId !== this.activeId) return;
    const i = this.turns.findIndex((x) => x.id === t.id);
    if (i >= 0) this.turns[i] = t;
    else this.turns = [...this.turns, t];
  }

  private upsertItem(it: Item) {
    if (it.threadId !== this.activeId) return;
    const i = this.items.findIndex((x) => x.id === it.id);
    if (i >= 0) {
      // Keep streamed text if the completed item arrives without it.
      const prev = this.items[i];
      if (!it.text && prev.text) it.text = prev.text;
      this.items[i] = it;
    } else {
      this.items = [...this.items, it].sort((a, b) => a.seq - b.seq);
    }
  }

  async open(id: string, resetScroll = true) {
    this.activeId = id;
    this.loading = resetScroll;
    app.view = 'chat';
    try {
      const r = await app.call<{ thread: Thread; turns: Turn[]; items: Item[] }>('thread/read', { threadId: id });
      if (this.activeId !== id) return;
      this.turns = r.turns ?? [];
      this.items = (r.items ?? []).sort((a, b) => a.seq - b.seq);
      this.selection = { ...(r.thread.settings ?? {}) };
      this.sending = this.turns.some((t) => t.status === 'running');
      this.upsertThread(r.thread);
    } catch (e) {
      app.toast('error', errMsg(e));
    } finally {
      this.loading = false;
    }
  }

  newChat() {
    this.activeId = null;
    this.turns = [];
    this.items = [];
    this.sending = false;
    this.highlightItem = null;
    app.view = 'chat';
    // Keep the last-used selection as the starting point for the new chat.
  }

  async send(text: string, attachments: Attachment[] = []) {
    text = text.trim();
    if (!text && attachments.length === 0) return;
    this.sending = true;
    try {
      let id = this.activeId;
      if (!id) {
        const t = await app.call<Thread>('thread/start', { channel: 'app', settings: this.selection });
        id = t.id;
        this.activeId = id;
        this.turns = [];
        this.items = [];
        this.upsertThread(t);
      }
      await app.call('turn/start', { threadId: id, text, attachments });
    } catch (e) {
      this.sending = false;
      app.toast('error', errMsg(e));
    }
  }

  async interrupt() {
    const t = this.running;
    if (t) await app.try('turn/interrupt', { turnId: t.id });
  }

  /** Change provider/model/complexity/key; saved on the thread when there is one. */
  async setSelection(sel: ModelSelection) {
    this.selection = sel;
    if (this.activeId) {
      await app.try('thread/setSettings', { threadId: this.activeId, settings: sel });
    }
  }

  async rename(id: string, title: string) {
    await app.try('thread/rename', { threadId: id, title });
  }

  async pin(t: Thread) {
    await app.try('thread/pin', { threadId: t.id, value: !t.pinned });
  }

  async archive(t: Thread) {
    await app.try('thread/archive', { threadId: t.id, value: !t.archived });
    if (this.activeId === t.id && !t.archived) this.newChat();
  }

  async remove(t: Thread) {
    const r = await app.try('thread/delete', { threadId: t.id });
    if (r === undefined) return;
    this.threads = this.threads.filter((x) => x.id !== t.id);
    if (this.activeId === t.id) this.newChat();
  }

  async fork(t: Thread, upToItemId?: string) {
    const r = await app.try<Thread>('thread/fork', { threadId: t.id, upToItemId });
    if (r) await this.open(r.id);
  }

  async exportMarkdown(t: Thread): Promise<string | undefined> {
    const r = await app.try<{ markdown: string }>('thread/export', { threadId: t.id });
    return r?.markdown;
  }

  async search(q: string) {
    this.query = q;
    if (!q.trim()) {
      this.hits = [];
      return;
    }
    try {
      const r = await app.call<{ hits: SearchHit[] }>('thread/search', { query: q, limit: 30 });
      if (this.query === q) this.hits = r.hits ?? [];
    } catch (e) {
      this.hits = [];
    }
  }

  async toggleArchived() {
    this.showArchived = !this.showArchived;
    await this.loadThreads();
  }
}

export const chat = new ChatState();
