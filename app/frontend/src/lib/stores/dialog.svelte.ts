// In-app confirm/prompt dialogs. WKWebView does not show window.confirm/prompt
// unless the host implements them, so the app never relies on them.

interface Pending {
  kind: 'confirm' | 'prompt' | 'project';
  message: string;
  okLabel: string;
  danger: boolean;
  secret: boolean;
  value: string;
  path: string;
  pathEditable: boolean;
  resolve: (v: string | boolean | { name: string; root: string } | null) => void;
}

class DialogState {
  current = $state<Pending | null>(null);

  confirm(message: string, opts: { okLabel?: string; danger?: boolean } = {}): Promise<boolean> {
    return new Promise((resolve) => {
      this.current = {
        kind: 'confirm', message, okLabel: opts.okLabel ?? 'OK', danger: opts.danger ?? false, secret: false, value: '', path: '', pathEditable: true,
        resolve: (v) => resolve(v === true),
      };
    });
  }

  prompt(message: string, opts: { okLabel?: string; secret?: boolean } = {}): Promise<string | null> {
    return new Promise((resolve) => {
      this.current = {
        kind: 'prompt', message, okLabel: opts.okLabel ?? 'OK', danger: false, secret: false, value: '', path: '', pathEditable: true,
        resolve: (v) => resolve(typeof v === 'string' ? v : null),
      };
    });
  }

  project(name = '', root = '', pathEditable = true): Promise<{ name: string; root: string } | null> {
    return new Promise((resolve) => {
      this.current = {
        kind: 'project', message: '', okLabel: 'Create project', danger: false, secret: false, value: name, path: root, pathEditable,
        resolve: (v) => resolve(v && typeof v === 'object' ? v : null),
      };
      if (name) this.current.okLabel = 'Save changes';
    });
  }

  close(ok: boolean) {
    const c = this.current;
    if (!c) return;
    this.current = null;
    if (!ok) c.resolve(c.kind === 'confirm' ? false : null);
    else if (c.kind === 'confirm') c.resolve(true);
    else if (c.kind === 'project') c.resolve({ name: c.value.trim(), root: c.path.trim() });
    else c.resolve(c.value);
  }
}

export const dialog = new DialogState();
