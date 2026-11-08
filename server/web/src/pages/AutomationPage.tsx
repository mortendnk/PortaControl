import { useEffect, useMemo, useState } from "react";
import { CalendarClock, Pencil, Play, Plus, SkipForward, Sparkles, Thermometer, Trash2 } from "lucide-react";
import { api, type Action, type Rule, type Scene, type Schedule } from "../api";
import { sortedDevices, toast, useStore } from "../store";
import { Badge, Button, Card, Empty, Field, Modal, Segmented, Spinner, Toggle, cx, navigate } from "../ui";
import { WEEKDAYS, fmtDateTime, t } from "../i18n";
import ControlEditor, { describeControl } from "./ControlEditor";

function useList<T>(path: string, ver: number) {
  const [list, setList] = useState<T[] | null>(null);
  const reload = () => api<T[]>(path).then(setList).catch(() => setList([]));
  useEffect(() => {
    reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, ver]);
  return { list, reload };
}

function DevicePicker({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const devices = useStore((s) => s.devices);
  const list = useMemo(() => sortedDevices(devices), [devices]);
  if (list.length <= 1) return null;
  return (
    <Field label={t("Anlagen")} hint={value.length === 0 ? t("Alle Anlagen") : undefined}>
      <div className="flex flex-wrap gap-2">
        {list.map((d) => {
          const on = value.includes(d.id);
          return (
            <button
              key={d.id}
              type="button"
              onClick={() => onChange(on ? value.filter((x) => x !== d.id) : [...value, d.id])}
              className={cx("px-3 h-8 rounded-full text-[13px] border", on ? "bg-accent text-on-accent border-accent" : "bg-surface2 border-line")}
            >
              {d.name}
            </button>
          );
        })}
      </div>
    </Field>
  );
}

function ActionEditor({ value, onChange, scenes }: { value: Action; onChange: (a: Action) => void; scenes: Scene[] }) {
  return (
    <div className="flex flex-col gap-3">
      <Field label={t("Aktion")}>
        <select value={value.kind} onChange={(e) => onChange({ kind: e.target.value as Action["kind"], scene_id: value.scene_id, control: value.control ?? { power: true } })}>
          <option value="on">{t("Einschalten")}</option>
          <option value="off">{t("Ausschalten")}</option>
          <option value="scene">{t("Szene anwenden")}</option>
          <option value="control">{t("Einstellen")}</option>
          <option value="notify">{t("Benachrichtigen")}</option>
        </select>
      </Field>
      {value.kind === "scene" && (
        <Field label={t("Szenen")}>
          <select value={value.scene_id ?? ""} onChange={(e) => onChange({ ...value, scene_id: e.target.value })}>
            <option value="">–</option>
            {scenes.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </Field>
      )}
      {(value.kind === "control" || value.kind === "on") && (
        <div className="rounded-xl border border-line px-3">
          <ControlEditor value={value.control ?? {}} onChange={(c) => onChange({ ...value, control: c })} hidePower={value.kind === "on"} />
        </div>
      )}
      {value.kind === "notify" && (
        <Field label="Text">
          <input value={value.text ?? ""} onChange={(e) => onChange({ ...value, text: e.target.value })} />
        </Field>
      )}
    </div>
  );
}

function describeAction(a: Action, scenes: Scene[]) {
  switch (a.kind) {
    case "on":
      return a.control && Object.keys(a.control).length > 1 ? `${t("Ein")}: ${describeControl(a.control)}` : t("Einschalten");
    case "off":
      return t("Ausschalten");
    case "scene":
      return `${t("Szene anwenden")}: ${scenes.find((s) => s.id === a.scene_id)?.name ?? "?"}`;
    case "control":
      return describeControl(a.control ?? {});
    case "notify":
      return `${t("Benachrichtigen")}: ${a.text ?? ""}`;
  }
}

function deviceNames(ids: string[] | null, devs: Record<string, { name: string }>) {
  if (!ids || ids.length === 0) return t("Alle Anlagen");
  return ids.map((i) => devs[i]?.name ?? i).join(", ");
}

// ---- Scenes ---------------------------------------------------------------

function ScenesTab({ admin }: { admin: boolean }) {
  const ver = useStore((s) => s.versions.scenes ?? 0);
  const devices = useStore((s) => s.devices);
  const { list, reload } = useList<Scene>("/scenes", ver);
  const [edit, setEdit] = useState<Scene | null>(null);

  const save = async () => {
    if (!edit) return;
    try {
      await api(edit.id ? `/scenes/${edit.id}` : "/scenes", { method: edit.id ? "PUT" : "POST", body: edit });
      setEdit(null);
      reload();
    } catch (e: any) {
      toast(t("Fehler"), e.message, true);
    }
  };

  if (!list) return <Spinner />;
  return (
    <Card
      title={t("Szenen")}
      action={admin && <Button onClick={() => setEdit({ id: "", name: "", icon: "sparkles", devices: [], control: { power: true, mode: "cool", target_temperature: 24 }, order: list.length })}><Plus size={16} /> {t("Neu")}</Button>}
    >
      <p className="text-[13px] text-muted mb-3">Gespeicherte Einstellungen, die mit einem Tipp (oder per Wochenprogramm, Regel, MQTT, App-Widget) angewendet werden.</p>
      {list.length === 0 && <Empty>Noch keine Szenen. Beispiel: „Nachts“ = Kühlen 25 °C, Lüfter leise, Schlafmodus.</Empty>}
      {list.map((s) => (
        <div key={s.id} className="flex items-center gap-3 py-3 border-b border-line last:border-0">
          <Sparkles size={18} className="text-accent shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="font-medium">{s.name}</div>
            <div className="text-[12px] text-muted truncate">{describeControl(s.control)} · {deviceNames(s.devices, devices)}</div>
          </div>
          <Button
            onClick={async () => {
              try {
                await api(`/scenes/${s.id}/apply`, { method: "POST" });
                toast(s.name, t("erledigt"));
              } catch (e: any) {
                toast(s.name, e.message, true);
              }
            }}
          >
            <Play size={15} />
          </Button>
          {admin && (
            <Button variant="ghost" onClick={() => setEdit(s)}>
              <Pencil size={15} />
            </Button>
          )}
        </div>
      ))}
      <Modal open={!!edit} onClose={() => setEdit(null)} title={edit?.id ? t("Bearbeiten") : t("Neu")}>
        {edit && (
          <div className="flex flex-col gap-3">
            <Field label={t("Name")}>
              <input value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} autoFocus />
            </Field>
            <DevicePicker value={edit.devices ?? []} onChange={(v) => setEdit({ ...edit, devices: v })} />
            <div className="rounded-xl border border-line px-3">
              <ControlEditor value={edit.control} onChange={(c) => setEdit({ ...edit, control: c })} />
            </div>
            <div className="flex gap-2 justify-between mt-2">
              {edit.id ? (
                <Button
                  variant="danger"
                  onClick={async () => {
                    await api(`/scenes/${edit.id}`, { method: "DELETE" });
                    setEdit(null);
                    reload();
                  }}
                >
                  <Trash2 size={15} />
                </Button>
              ) : (
                <span />
              )}
              <div className="flex gap-2">
                <Button onClick={() => setEdit(null)}>{t("Abbrechen")}</Button>
                <Button variant="primary" onClick={save}>
                  {t("Speichern")}
                </Button>
              </div>
            </div>
          </div>
        )}
      </Modal>
    </Card>
  );
}

// ---- Schedules --------------------------------------------------------------

function SchedulesTab({ admin }: { admin: boolean }) {
  const ver = useStore((s) => s.versions.automation ?? 0);
  const sver = useStore((s) => s.versions.scenes ?? 0);
  const devices = useStore((s) => s.devices);
  const { list, reload } = useList<Schedule>("/schedules", ver);
  const scenes = useList<Scene>("/scenes", sver).list ?? [];
  const [edit, setEdit] = useState<Schedule | null>(null);

  const save = async (s: Schedule) => {
    try {
      await api(s.id ? `/schedules/${s.id}` : "/schedules", { method: s.id ? "PUT" : "POST", body: s });
      setEdit(null);
      reload();
    } catch (e: any) {
      toast(t("Fehler"), e.message, true);
    }
  };

  const days = (d: number[] | null) => {
    if (!d || d.length === 0 || d.length === 7) return t("täglich");
    if (d.length === 5 && [1, 2, 3, 4, 5].every((x) => d.includes(x))) return "Mo–Fr";
    if (d.length === 2 && d.includes(0) && d.includes(6)) return "Sa, So";
    return WEEKDAYS.filter((w) => d.includes(w.d)).map((w) => t(w.l)).join(", ");
  };

  if (!list) return <Spinner />;
  return (
    <Card
      title={t("Wochenprogramm")}
      action={admin && <Button onClick={() => setEdit({ id: "", name: "", enabled: true, devices: [], days: [1, 2, 3, 4, 5], time: "07:00", action: { kind: "on" }, skip_next: false })}><Plus size={16} /> {t("Neu")}</Button>}
    >
      <p className="text-[13px] text-muted mb-3">Läuft auf dem Server – auch wenn kein Gerät mit der App offen ist.</p>
      {list.length === 0 && <Empty>Kein Programm. Beispiel: Mo–Fr 16:30 „Kühlen 24 °C“, täglich 23:00 „Aus“.</Empty>}
      {list.map((s) => (
        <div key={s.id} className="flex items-center gap-3 py-3 border-b border-line last:border-0">
          <div className="text-[22px] font-semibold tabular w-16 shrink-0">{s.time}</div>
          <div className="flex-1 min-w-0">
            <div className="font-medium flex items-center gap-2">
              {s.name} {s.skip_next && <Badge tone="warn">einmal aussetzen</Badge>}
            </div>
            <div className="text-[12px] text-muted truncate">
              {days(s.days)} · {describeAction(s.action, scenes)} · {deviceNames(s.devices, devices)}
            </div>
            {s.enabled && s.next_run && (
              <div className="text-[12px] text-muted">
                {t("Nächster Lauf")}: {fmtDateTime(s.next_run)}
              </div>
            )}
          </div>
          {admin && (
            <>
              <Toggle on={s.enabled} onChange={(v) => save({ ...s, enabled: v })} />
              <Button variant="ghost" title={t("Einmal überspringen")} onClick={() => save({ ...s, skip_next: !s.skip_next })}>
                <SkipForward size={15} />
              </Button>
              <Button variant="ghost" onClick={() => setEdit(s)}>
                <Pencil size={15} />
              </Button>
            </>
          )}
        </div>
      ))}
      <Modal open={!!edit} onClose={() => setEdit(null)} title={edit?.id ? t("Bearbeiten") : t("Neu")}>
        {edit && (
          <div className="flex flex-col gap-3">
            <Field label={t("Name")}>
              <input value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
            </Field>
            <div className="flex gap-3">
              <Field label={t("Uhrzeit")}>
                <input type="time" value={edit.time} onChange={(e) => setEdit({ ...edit, time: e.target.value })} />
              </Field>
            </div>
            <Field label={t("Tage")}>
              <div className="flex gap-1.5 flex-wrap">
                {WEEKDAYS.map((w) => {
                  const on = (edit.days ?? []).includes(w.d);
                  return (
                    <button
                      key={w.d}
                      type="button"
                      onClick={() => setEdit({ ...edit, days: on ? (edit.days ?? []).filter((x) => x !== w.d) : [...(edit.days ?? []), w.d] })}
                      className={cx("w-10 h-10 rounded-full text-[13px] font-medium border", on ? "bg-accent text-on-accent border-accent" : "bg-surface2 border-line")}
                    >
                      {t(w.l)}
                    </button>
                  );
                })}
              </div>
            </Field>
            <DevicePicker value={edit.devices ?? []} onChange={(v) => setEdit({ ...edit, devices: v })} />
            <ActionEditor value={edit.action} scenes={scenes} onChange={(a) => setEdit({ ...edit, action: a })} />
            <div className="flex gap-2 justify-between mt-2">
              {edit.id ? (
                <div className="flex gap-2">
                  <Button
                    variant="danger"
                    onClick={async () => {
                      await api(`/schedules/${edit.id}`, { method: "DELETE" });
                      setEdit(null);
                      reload();
                    }}
                  >
                    <Trash2 size={15} />
                  </Button>
                  <Button
                    onClick={async () => {
                      try {
                        await api(`/schedules/${edit.id}/run`, { method: "POST" });
                        toast(edit.name, t("erledigt"));
                      } catch (e: any) {
                        toast(t("Fehler"), e.message, true);
                      }
                    }}
                  >
                    <Play size={15} /> {t("Jetzt ausführen")}
                  </Button>
                </div>
              ) : (
                <span />
              )}
              <div className="flex gap-2">
                <Button onClick={() => setEdit(null)}>{t("Abbrechen")}</Button>
                <Button variant="primary" onClick={() => save(edit)}>
                  {t("Speichern")}
                </Button>
              </div>
            </div>
          </div>
        )}
      </Modal>
    </Card>
  );
}

// ---- Rules ------------------------------------------------------------------

const SENSORS = { indoor: "Innentemperatur", outdoor: "Außentemperatur", power_watts: "Leistungsaufnahme" } as const;

function RulesTab({ admin }: { admin: boolean }) {
  const ver = useStore((s) => s.versions.automation ?? 0);
  const sver = useStore((s) => s.versions.scenes ?? 0);
  const devices = useStore((s) => s.devices);
  const devList = useMemo(() => sortedDevices(devices), [devices]);
  const { list, reload } = useList<Rule>("/rules", ver);
  const scenes = useList<Scene>("/scenes", sver).list ?? [];
  const [edit, setEdit] = useState<Rule | null>(null);

  const save = async (r: Rule) => {
    try {
      await api(r.id ? `/rules/${r.id}` : "/rules", { method: r.id ? "PUT" : "POST", body: r });
      setEdit(null);
      reload();
    } catch (e: any) {
      toast(t("Fehler"), e.message, true);
    }
  };

  if (!list) return <Spinner />;
  return (
    <Card
      title={t("Regeln")}
      action={
        admin &&
        devList.length > 0 && (
          <Button
            onClick={() =>
              setEdit({ id: "", name: "", enabled: true, device: devList[0].id, sensor: "indoor", op: "above", value: 27, hysteresis: 1, action: { kind: "on", control: { power: true, mode: "cool", target_temperature: 24 } }, cooldown_minutes: 30, only_if_off: true })
            }
          >
            <Plus size={16} /> {t("Neu")}
          </Button>
        )
      }
    >
      <p className="text-[13px] text-muted mb-3">Klima-Automatik: wird ausgelöst, wenn ein Messwert die Schwelle überschreitet, und erst wieder scharf, wenn er um die Hysterese zurückgegangen ist.</p>
      {list.length === 0 && <Empty>Keine Regel. Beispiel: „Wenn innen über 27 °C (10–22 Uhr) → Kühlen 24 °C“.</Empty>}
      {list.map((r) => (
        <div key={r.id} className="flex items-center gap-3 py-3 border-b border-line last:border-0">
          <Thermometer size={18} className="text-accent shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="font-medium">{r.name}</div>
            <div className="text-[12px] text-muted">
              {devices[r.device]?.name ?? r.device}: {t("Wenn")} {t(SENSORS[r.sensor])} {t(r.op === "above" ? "über" : "unter")} {r.value}
              {r.sensor === "power_watts" ? " W" : " °C"}
              {r.from && r.to && ` (${r.from}–${r.to})`} → {describeAction(r.action, scenes)}
            </div>
            {r.last_fired && !r.last_fired.startsWith("0001") && <div className="text-[12px] text-muted">zuletzt {fmtDateTime(r.last_fired)}</div>}
          </div>
          {admin && (
            <>
              <Toggle on={r.enabled} onChange={(v) => save({ ...r, enabled: v })} />
              <Button variant="ghost" onClick={() => setEdit(r)}>
                <Pencil size={15} />
              </Button>
            </>
          )}
        </div>
      ))}
      <Modal open={!!edit} onClose={() => setEdit(null)} title={edit?.id ? t("Bearbeiten") : t("Neu")}>
        {edit && (
          <div className="flex flex-col gap-3">
            <Field label={t("Name")}>
              <input value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
            </Field>
            <Field label={t("Anlage")}>
              <select value={edit.device} onChange={(e) => setEdit({ ...edit, device: e.target.value })}>
                {devList.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </select>
            </Field>
            <div className="grid grid-cols-3 gap-2">
              <Field label={t("Wenn")}>
                <select value={edit.sensor} onChange={(e) => setEdit({ ...edit, sensor: e.target.value as Rule["sensor"] })}>
                  {Object.entries(SENSORS).map(([k, v]) => (
                    <option key={k} value={k}>
                      {t(v)}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label={"00a0"}>
                <select value={edit.op} onChange={(e) => setEdit({ ...edit, op: e.target.value as Rule["op"] })}>
                  <option value="above">{t("über")}</option>
                  <option value="below">{t("unter")}</option>
                </select>
              </Field>
              <Field label={edit.sensor === "power_watts" ? "W" : "°C"}>
                <input type="number" step="0.5" value={edit.value} onChange={(e) => setEdit({ ...edit, value: Number(e.target.value) })} />
              </Field>
            </div>
            <div className="grid grid-cols-3 gap-2">
              <Field label="Hysterese">
                <input type="number" step="0.5" min="0" value={edit.hysteresis} onChange={(e) => setEdit({ ...edit, hysteresis: Number(e.target.value) })} />
              </Field>
              <Field label={t("Zeitfenster")}>
                <input type="time" value={edit.from ?? ""} onChange={(e) => setEdit({ ...edit, from: e.target.value })} />
              </Field>
              <Field label="bis">
                <input type="time" value={edit.to ?? ""} onChange={(e) => setEdit({ ...edit, to: e.target.value })} />
              </Field>
            </div>
            <div className="flex gap-4 flex-wrap text-[14px]">
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={!!edit.only_if_off} onChange={(e) => setEdit({ ...edit, only_if_off: e.target.checked, only_if_on: false })} />
                {t("nur wenn aus")}
              </label>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={!!edit.only_if_on} onChange={(e) => setEdit({ ...edit, only_if_on: e.target.checked, only_if_off: false })} />
                {t("nur wenn an")}
              </label>
              <label className="flex items-center gap-2">
                {t("Pause danach (min)")}
                <input type="number" className="w-20" value={edit.cooldown_minutes} onChange={(e) => setEdit({ ...edit, cooldown_minutes: Number(e.target.value) })} />
              </label>
            </div>
            <div className="text-[13px] font-medium">{t("Dann")}</div>
            <ActionEditor value={edit.action} scenes={scenes} onChange={(a) => setEdit({ ...edit, action: a })} />
            <div className="flex gap-2 justify-between mt-2">
              {edit.id ? (
                <Button
                  variant="danger"
                  onClick={async () => {
                    await api(`/rules/${edit.id}`, { method: "DELETE" });
                    setEdit(null);
                    reload();
                  }}
                >
                  <Trash2 size={15} />
                </Button>
              ) : (
                <span />
              )}
              <div className="flex gap-2">
                <Button onClick={() => setEdit(null)}>{t("Abbrechen")}</Button>
                <Button variant="primary" onClick={() => save(edit)}>
                  {t("Speichern")}
                </Button>
              </div>
            </div>
          </div>
        )}
      </Modal>
    </Card>
  );
}

export default function AutomationPage({ tab }: { tab?: string }) {
  const me = useStore((s) => s.me);
  const admin = me?.role === "admin";
  const cur = tab ?? "schedules";
  return (
    <div className="flex flex-col gap-4">
      <Segmented
        value={cur}
        onChange={(v) => navigate(`/automation/${v}`, true)}
        options={[
          { value: "schedules", label: <><CalendarClock size={15} /> {t("Wochenprogramm")}</> },
          { value: "scenes", label: <><Sparkles size={15} /> {t("Szenen")}</> },
          { value: "rules", label: <><Thermometer size={15} /> {t("Regeln")}</> },
        ]}
      />
      {cur === "scenes" ? <ScenesTab admin={admin} /> : cur === "rules" ? <RulesTab admin={admin} /> : <SchedulesTab admin={admin} />}
      {!admin && <p className="text-[12px] text-muted text-center">Ändern nur als Administrator.</p>}
    </div>
  );
}
