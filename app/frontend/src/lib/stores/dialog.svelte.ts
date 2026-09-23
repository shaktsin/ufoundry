// In-app confirm/prompt dialogs. WKWebView does not show window.confirm/prompt
// unless the host implements them, so the app never relies on them.

interface Pending {
  kind: 'confirm' | 'prompt';
  message: string;
  okLabel: string;
  danger: boolean;
  secret: boolean;
  value: string;
  resolve: (v: string | boolean | null) => void;
}

class DialogState {
  current = $state<Pending | null>(null);

  confirm(message: string, opts: { okLabel?: string; danger?: boolean } = {}): Promise<boolean> {
    return new Promise((resolve) => {
      this.current = {
        kind: 'confirm', message, okLabel: opts.okLabel ?? 'OK', danger: opts.danger ?? false, secret: false, value: '',
        resolve: (v) => resolve(v === true),
      };
    });
  }

  prompt(message: string, opts: { okLabel?: string; secret?: boolean } = {}): Promise<string | null> {
    return new Promise((resolve) => {
      this.current = {
        kind: 'prompt', message, okLabel: opts.okLabel ?? 'OK', danger: false, secret: opts.secret ?? false, value: '',
        resolve: (v) => resolve(typeof v === 'string' ? v : null),
      };
    });
  }

  close(ok: boolean) {
    const c = this.current;
    if (!c) return;
    this.current = null;
    if (!ok) c.resolve(c.kind === 'confirm' ? false : null);
    else c.resolve(c.kind === 'confirm' ? true : c.value);
  }
}

export const dialog = new DialogState();
