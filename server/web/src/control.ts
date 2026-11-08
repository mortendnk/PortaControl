import { useState } from "react";
import { ApiError, control, type Control, type Device } from "./api";
import { getState, toast, upsertDevice } from "./store";
import { t } from "./i18n";

// Optimistic control: the UI shows the wish at once, the server answers
// with the state the device really applied.
export function useControl(id: string) {
  const [busy, setBusy] = useState(false);

  const send = async (c: Control) => {
    const cur = getState().devices[id];
    if (cur?.state) {
      const s = { ...cur.state } as any;
      for (const [k, v] of Object.entries(c)) {
        const map: Record<string, string> = { swing: "swing", display_on: "display_on", rate_percent: "rate_percent" };
        s[map[k] ?? k] = v;
      }
      upsertDevice({ ...cur, state: s });
    }
    setBusy(true);
    try {
      const d = await control(id, c);
      upsertDevice(d);
      return d;
    } catch (e: any) {
      if (e instanceof ApiError && e.status === 409 && e.body?.state && cur) {
        upsertDevice({ ...cur, state: e.body.state } as Device);
      } else if (cur) {
        upsertDevice(cur);
      }
      toast(t("Fehler"), e.message, true);
      return null;
    } finally {
      setBusy(false);
    }
  };

  return { send, busy };
}
