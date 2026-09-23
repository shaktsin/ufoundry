import { describe, it, expect, vi } from 'vitest';
import { RpcClient, RpcError } from './rpc';

// Minimal WebSocket stand-in driven by the test.
class FakeSocket {
  static OPEN = 1;
  readyState = 0;
  sent: any[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(public url: string) {}
  send(s: string) { this.sent.push(JSON.parse(s)); }
  close() { this.readyState = 3; this.onclose?.(); }
  open() { this.readyState = 1; this.onopen?.(); }
  reply(id: number, result: unknown) { this.onmessage?.({ data: JSON.stringify({ jsonrpc: '2.0', id, result }) }); }
  fail(id: number, code: number, message: string) { this.onmessage?.({ data: JSON.stringify({ jsonrpc: '2.0', id, error: { code, message } }) }); }
  notify(method: string, params: unknown) { this.onmessage?.({ data: JSON.stringify({ jsonrpc: '2.0', method, params }) }); }
}

(globalThis as any).WebSocket = FakeSocket;
const flush = () => new Promise((r) => setTimeout(r, 0));

describe('RpcClient', () => {
  it('handshakes, calls, dispatches notifications and maps errors', async () => {
    const sockets: FakeSocket[] = [];
    const states: string[] = [];
    const c = new RpcClient(
      async () => ({ url: 'ws://x/ws', token: 'a b' }),
      async (cl) => { await cl.call('initialize', { admin: true }); },
      (u) => { const s = new FakeSocket(u); sockets.push(s); return s as unknown as WebSocket; },
    );
    c.onState((s) => states.push(s));
    c.start();
    await flush();
    expect(sockets[0].url).toBe('ws://x/ws?token=a%20b');
    sockets[0].open();
    expect(sockets[0].sent[0].method).toBe('initialize');
    sockets[0].reply(sockets[0].sent[0].id, {});
    await flush();
    expect(c.state).toBe('open');

    const p = c.call('thread/list', { limit: 5 });
    const req = sockets[0].sent[1];
    expect(req).toMatchObject({ jsonrpc: '2.0', method: 'thread/list', params: { limit: 5 } });
    sockets[0].reply(req.id, { threads: [] });
    await expect(p).resolves.toEqual({ threads: [] });

    const bad = c.call('thread/read', { threadId: 'nope' });
    sockets[0].fail(sockets[0].sent[2].id, -32602, 'thread not found');
    await expect(bad).rejects.toMatchObject({ code: -32602, message: 'thread not found' });

    const seen = vi.fn();
    c.on('item/delta', seen);
    sockets[0].notify('item/delta', { text: 'hi' });
    expect(seen).toHaveBeenCalledWith({ text: 'hi' });
    c.stop();
  });

  it('fails pending calls and reconnects when the socket drops', async () => {
    vi.useFakeTimers();
    const sockets: FakeSocket[] = [];
    const c = new RpcClient(
      async () => ({ url: 'ws://x/ws', token: 't' }),
      async () => {},
      (u) => { const s = new FakeSocket(u); sockets.push(s); return s as unknown as WebSocket; },
    );
    c.start();
    await vi.advanceTimersByTimeAsync(0);
    sockets[0].open();
    await vi.advanceTimersByTimeAsync(0);
    const p = c.call('engine/status');
    sockets[0].close();
    await expect(p).rejects.toBeInstanceOf(RpcError);
    expect(c.state).toBe('closed');
    await vi.advanceTimersByTimeAsync(600);
    expect(sockets.length).toBe(2);
    c.stop();
    vi.useRealTimers();
  });

  it('rejects calls while disconnected', async () => {
    const c = new RpcClient(async () => ({ url: 'ws://x', token: 't' }), async () => {});
    await expect(c.call('engine/status')).rejects.toThrow('not connected');
  });
});
