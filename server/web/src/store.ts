// Global live state: devices from the SSE stream, login role, notices and
// "versions" that tell pages to reload lists after changes elsewhere.
import { useSyncExternalStore } from "react";
import { api, type Device, type Me } from "./api";

export interface Notice {
  id: number;
  kind: string;
  title: string;
  text?: string;
  error?: boolean;
}

interface State {
  devices: Record<string, Device>;
  loaded: boolean;
  connected: boolean;
  me: Me | null;
  notices: Notice[];
  versions: Record<string, number>;
}

let state: State = { devices: {}, loaded: false, connected: false, me: null, notices: [], versions: {} };
const listeners = new Set<() => void>();

function set(patch: Partial<State>) {
  state = { ...state, ...patch };
  listeners.forEach((l) => l());
}

export function useStore<T>(sel: (s: State) => T): T {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => sel(state),
  );
}

export function getState() {
  return state;
}

export function upsertDevice(d: Device) {
  set({ devices: { ...state.devices, [d.id]: d } });
}

export function bump(key: string) {
  set({ versions: { ...state.versions, [key]: (state.versions[key] ?? 0) + 1 } });
}

let noticeId = 1;
export function toast(title: string, text?: string, error = false, kind = "info") {
  const n: Notice = { id: noticeId++, kind, title, text, error };
  set({ notices: [...state.notices, n].slice(-4) });
  setTimeout(() => set({ notices: state.notices.filter((x) => x.id !== n.id) }), error ? 8000 : 4500);
}

export function dismiss(id: number) {
  set({ notices: state.notices.filter((x) => x.id !== id) });
}

export async function loadMe() {
  try {
    const me = await api<Me>("/auth/me");
    set({ me });
    return me;
  } catch {
    return null;
  }
}

export async function loadDevices() {
  try {
    const list = await api<Device[]>("/devices");
    const devices: Record<string, Device> = {};
    list.forEach((d) => (devices[d.id] = d));
    set({ devices, loaded: true });
  } catch {
    /* role none: handled by login */
  }
}

let es: EventSource | null = null;

export function connectLive() {
  es?.close();
  es = new EventSource("/api/v1/events");
  es.onopen = () => set({ connected: true });
  es.onerror = () => set({ connected: false });
  es.onmessage = (m) => {
    let e: any;
    try {
      e = JSON.parse(m.data);
    } catch {
      return;
    }
    switch (e.type) {
      case "snapshot": {
        const devices: Record<string, Device> = {};
        (e.data as Device[]).forEach((d) => (devices[d.id] = d));
        set({ devices, loaded: true, connected: true });
        break;
      }
      case "device":
        upsertDevice(e.data as Device);
        break;
      case "devices":
        loadDevices();
        break;
      case "notice":
        toast(e.data.title, e.data.text, e.data.kind === "error" || e.data.kind === "offline", e.data.kind);
        bump("log");
        break;
      case "log":
        bump("log");
        break;
      default:
        bump(e.type);
    }
  };
}

export function sortedDevices(devs: Record<string, Device>) {
  return Object.values(devs).sort((a, b) => a.order - b.order || a.name.localeCompare(b.name));
}
