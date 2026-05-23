// Pages register actions; the palette shows them.
//
// We deliberately keep this lightweight — no React context, just a module-
// level registry that the palette subscribes to via the listener pattern.
// That sidesteps the "useEffect must be inside a Provider" hassle for pages
// that want to register commands during their own render.

export type Command = {
  id: string;
  label: string;
  group?: string;
  description?: string;
  shortcut?: string;
  // when=false hides the command from the palette but keeps it registered.
  when?: () => boolean;
  run: () => void | Promise<void>;
};

const commands = new Map<string, Command>();
const listeners = new Set<() => void>();

function notify() {
  for (const fn of listeners) fn();
}

export function registerCommand(cmd: Command): () => void {
  commands.set(cmd.id, cmd);
  notify();
  return () => {
    commands.delete(cmd.id);
    notify();
  };
}

export function listCommands(): Command[] {
  const all = Array.from(commands.values());
  return all.filter((c) => (c.when ? c.when() : true));
}

// subscribe re-fires when the registry changes (registrations, removals).
// Returns an unsubscribe.
export function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}
