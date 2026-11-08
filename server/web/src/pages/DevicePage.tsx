import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  AlarmClock,
  AlertTriangle,
  ArrowLeft,
  ArrowUpDown,
  BarChart3,
  Bed,
  Gauge,
  Leaf,
  Monitor,
  Power,
  RefreshCw,
  Rocket,
  Snowflake,
  Sparkles,
  SprayCan,
  Stethoscope,
  Timer,
  Volume2,
  VolumeX,
} from "lucide-react";
import { api, type Device, type Diagnostics, type Scene, type Timers } from "../api";
import { toast, upsertDevice, useStore } from "../store";
import { useControl } from "../control";
import { Badge, Button, Card, Link, Modal, Row, Segmented, Spinner, Stat, cx, navigate } from "../ui";
import { FAN_PRESETS, MODES, fanLabel, fmtAgo, fmtDateTime, fmtMoney, fmtNum, fmtTemp, modeColor, t } from "../i18n";
import Thermostat from "./Thermostat";
import { ModeIcon } from "./Dashboard";

function Tile({
  icon,
  label,
  on,
  disabled,
  why,
  onClick,
}: {
  icon: ReactNode;
  label: string;
  on: boolean | null | undefined;
  disabled?: boolean;
  why?: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      title={disabled ? why : undefined}
      disabled={disabled}
      onClick={onClick}
      className={cx(
        "rounded-2xl border px-3 py-3 flex flex-col items-start gap-2 text-left transition min-h-[78px] disabled:opacity-40",
        on ? "bg-accent text-on-accent border-accent" : "bg-surface2/60 border-line hover:border-accent/40",
      )}
    >
      {icon}
      <span className="text-[13px] font-medium leading-tight">{label}</span>
    </button>
  );
}

function FanControl({ value, disabled, onChange }: { value: number; disabled?: boolean; onChange: (v: number) => void }) {
  const [local, setLocal] = useState(value);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (timer.current === undefined) setLocal(value);
  }, [value]);
  const isAuto = local === 102;
  const set = (v: number, delay = 600) => {
    setLocal(v);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      timer.current = undefined;
      if (v !== value) onChange(v);
    }, delay);
  };
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="text-[14px] font-medium">{t("Lüfter")}</span>
        <span className="text-[14px] tabular text-muted">{fanLabel(local)}</span>
      </div>
      <input
        type="range"
        min={1}
        max={100}
        value={isAuto ? 50 : local}
        disabled={disabled}
        className={isAuto ? "opacity-40" : ""}
        onChange={(e) => set(Number(e.target.value))}
      />
      <div className="flex gap-1.5 flex-wrap">
        <Button className={cx("h-8 px-3 text-[12px]", isAuto && "!bg-accent !text-on-accent")} disabled={disabled} onClick={() => set(102, 0)}>
          {t("Auto")}
        </Button>
        {FAN_PRESETS.map((p) => (
          <Button key={p.v} className={cx("h-8 px-3 text-[12px]", local === p.v && "!bg-accent !text-on-accent")} disabled={disabled} onClick={() => set(p.v, 0)}>
            {t(p.label)}
          </Button>
        ))}
      </div>
    </div>
  );
}

function TimersCard({ d, scenes }: { d: Device; scenes: Scene[] }) {
  const { send } = useControl(d.id);
  const [onTime, setOnTime] = useState("07:00");
  const [onScene, setOnScene] = useState("");
  const [offIn, setOffIn] = useState(60);
  const [devOn, setDevOn] = useState(60);
  const [devOff, setDevOff] = useState(120);
  const s = d.state;

  const save = async (tm: Timers) => {
    try {
      const out = await api<Timers>(`/devices/${d.id}/timers`, { method: "PUT", body: tm });
      upsertDevice({ ...d, timers: out });
    } catch (e: any) {
      toast(t("Fehler"), e.message, true);
    }
  };

  const nextAt = (hhmm: string) => {
    const [h, m] = hhmm.split(":").map(Number);
    const x = new Date();
    x.setHours(h, m, 0, 0);
    if (x.getTime() <= Date.now()) x.setDate(x.getDate() + 1);
    return x.toISOString();
  };

  return (
    <Card title={<span className="flex items-center gap-2"><Timer size={16} /> {t("Timer")}</span>}>
      <div className="text-[12px] text-muted mb-2">Vom Server ausgeführt – auch wenn kein Browser und keine App offen ist.</div>
      <Row label={t("Einschalten um")} hint={d.timers.on_at ? `${t("aktiv")}: ${fmtDateTime(d.timers.on_at)}` : undefined}>
        {d.timers.on_at ? (
          <Button onClick={() => save({ ...d.timers, on_at: null, on_scene_id: "" })}>{t("Löschen")}</Button>
        ) : (
          <>
            <input type="time" value={onTime} onChange={(e) => setOnTime(e.target.value)} className="w-[110px]" />
            <select value={onScene} onChange={(e) => setOnScene(e.target.value)} className="max-w-[130px]">
              <option value="">{t("Ein")}</option>
              {scenes.map((sc) => (
                <option key={sc.id} value={sc.id}>
                  {sc.name}
                </option>
              ))}
            </select>
            <Button variant="primary" onClick={() => save({ ...d.timers, on_at: nextAt(onTime), on_scene_id: onScene })}>
              OK
            </Button>
          </>
        )}
      </Row>
      <Row label={t("Ausschalten in")} hint={d.timers.off_at ? `${t("aktiv")}: ${fmtDateTime(d.timers.off_at)}` : undefined}>
        {d.timers.off_at ? (
          <Button onClick={() => save({ ...d.timers, off_at: null })}>{t("Löschen")}</Button>
        ) : (
          <>
            <select value={offIn} onChange={(e) => setOffIn(Number(e.target.value))}>
              {[15, 30, 45, 60, 90, 120, 180, 240, 360, 480].map((m) => (
                <option key={m} value={m}>
                  {m < 60 ? `${m} min` : `${m / 60} h`}
                </option>
              ))}
            </select>
            <Button variant="primary" onClick={() => save({ ...d.timers, off_at: new Date(Date.now() + offIn * 60000).toISOString() })}>
              OK
            </Button>
          </>
        )}
      </Row>
      <div className="text-[12px] text-muted mt-4 mb-1 font-medium uppercase tracking-wide">{t("Gerätetimer")}</div>
      <div className="text-[12px] text-muted mb-1">In der Anlage gespeichert (wie in der Midea-App), läuft auch ohne Server.</div>
      <Row label="Ein nach" hint={s?.on_timer_minutes ? `noch ${s.on_timer_minutes} min` : undefined}>
        {s?.on_timer_minutes ? (
          <Button onClick={() => send({ on_timer_minutes: 0 })}>{t("Löschen")}</Button>
        ) : (
          <>
            <select value={devOn} onChange={(e) => setDevOn(Number(e.target.value))}>
              {[30, 60, 120, 180, 240, 360, 480, 600, 720].map((m) => (
                <option key={m} value={m}>
                  {m < 60 ? `${m} min` : `${m / 60} h`}
                </option>
              ))}
            </select>
            <Button onClick={() => send({ on_timer_minutes: devOn })}>OK</Button>
          </>
        )}
      </Row>
      <Row label="Aus nach" hint={s?.off_timer_minutes ? `noch ${s.off_timer_minutes} min` : undefined}>
        {s?.off_timer_minutes ? (
          <Button onClick={() => send({ off_timer_minutes: 0 })}>{t("Löschen")}</Button>
        ) : (
          <>
            <select value={devOff} onChange={(e) => setDevOff(Number(e.target.value))}>
              {[30, 60, 120, 180, 240, 360, 480, 600, 720].map((m) => (
                <option key={m} value={m}>
                  {m < 60 ? `${m} min` : `${m / 60} h`}
                </option>
              ))}
            </select>
            <Button onClick={() => send({ off_timer_minutes: devOff })}>OK</Button>
          </>
        )}
      </Row>
    </Card>
  );
}

function DiagnosticsView({ id }: { id: string }) {
  const [d, setD] = useState<Diagnostics | null>(null);
  const [err, setErr] = useState("");
  const load = async () => {
    setErr("");
    try {
      setD(await api<Diagnostics>(`/devices/${id}/diagnostics`));
    } catch (e: any) {
      setErr(e.message);
    }
  };
  useEffect(() => {
    load();
    const i = window.setInterval(load, 15000);
    return () => window.clearInterval(i);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const n = (v: number | null | undefined, unit: string, digits = 0) => (v == null ? "–" : `${fmtNum(v, digits)} ${unit}`);
  const rows: [string, string][] = d
    ? [
        ["Verdichter", d.compressor_hz == null ? "–" : `${d.compressor_hz} Hz (Ziel ${d.compressor_target_hz ?? "–"} Hz)`],
        ["Verdichter-Leistung", n(d.compressor_power_w, "W")],
        ["Strom / Spannung", `${n(d.current_a, "A")} / ${n(d.voltage_v, "V")}`],
        ["T1 Raumluft", n(d.t1_indoor_air, "°C", 1)],
        ["Zweiter Innenfühler", n(d.indoor_temp2, "°C", 2)],
        ["T2 Verdampfer", n(d.t2_indoor_coil, "°C", 1)],
        ["T3 Verflüssiger", n(d.t3_outdoor_coil, "°C", 1)],
        ["T4 Außenluft", n(d.t4_outdoor_air, "°C", 1)],
        ["TP Heißgas", n(d.tp_discharge, "°C")],
        ["Lüfter innen", d.indoor_fan_rpm == null ? "–" : `${d.indoor_fan_rpm} U/min (Ziel ${d.indoor_fan_target_rpm ?? "–"})`],
        ["Lüfter außen", n(d.outdoor_fan_rpm, "U/min")],
        ["Expansionsventil", n(d.eev_opening, "Schritte")],
        ["Kondensatpumpe", d.condensate_pump == null ? "–" : d.condensate_pump ? "läuft" : "aus"],
        ["Abtauen", d.defrost == null ? "–" : d.defrost ? "ja" : "nein"],
        ["Luftfeuchte", n(d.humidity, "%")],
        ["Verbrauch laufender Betrieb", n(d.current_run_kwh, "kWh", 2)],
        ["Zählerstand roh", d.energy_counter_raw == null ? "–" : String(d.energy_counter_raw)],
        ["Gerätekennung", d.identifier || "–"],
      ]
    : [];

  return (
    <Card
      title={<span className="flex items-center gap-2"><Stethoscope size={16} /> {t("Diagnose")}</span>}
      action={<Button onClick={load}><RefreshCw size={15} /></Button>}
    >
      {err && <div className="text-danger text-[13px] mb-2">{err}</div>}
      {!d && !err && <Spinner />}
      {d && (
        <>
          <div className="text-[12px] text-muted mb-2">Live aus der Anlage (Gruppenabfragen 1–7, Zuordnung nach msmart-ng/Klimacontrol). Aktualisiert alle 15 s.</div>
          {rows.map(([k, v]) => (
            <Row key={k} label={k}>
              <span className="tabular text-[14px]">{v}</span>
            </Row>
          ))}
          <details className="mt-3">
            <summary className="text-[13px] text-muted cursor-pointer">Rohdaten</summary>
            <pre className="text-[11px] whitespace-pre-wrap break-all mt-2 text-muted">
              {Object.entries(d.raw ?? {})
                .map(([g, h]) => `Gruppe ${g}: ${h}`)
                .join("\n")}
            </pre>
          </details>
        </>
      )}
    </Card>
  );
}

export default function DevicePage({ id, tab }: { id: string; tab?: string }) {
  const d = useStore((s) => s.devices[id]);
  const loaded = useStore((s) => s.loaded);
  const me = useStore((s) => s.me);
  const scenesVer = useStore((s) => s.versions.scenes ?? 0);
  const { send, busy } = useControl(id);
  const [scenes, setScenes] = useState<Scene[]>([]);
  const [confirmClean, setConfirmClean] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  useEffect(() => {
    api<Scene[]>("/scenes").then(setScenes).catch(() => {});
  }, [scenesVer]);

  if (!loaded) return <Spinner />;
  if (!d) return <Card>Unbekannte Anlage. <Link to="/" className="underline">{t("Übersicht")}</Link></Card>;

  const s = d.state;
  const on = !!s?.power;
  const av = d.available;
  const color = on ? modeColor(s?.mode) : "var(--muted)";
  const locked = d.locked && me?.role !== "admin";
  const dis = !s || locked;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <button onClick={() => navigate("/")} className="p-2 -ml-2 rounded-xl hover:bg-surface2" aria-label="Zurück">
          <ArrowLeft size={20} />
        </button>
        <div className="min-w-0 flex-1">
          <h1 className="text-[20px] font-semibold truncate">{d.name}</h1>
          <div className="text-[12px] text-muted flex gap-2 flex-wrap">
            {d.room && <span>{d.room}</span>}
            {d.online ? (
              <span className="text-ok">
                {d.path === "cloud" ? t("über Cloud") : t("über WLAN")} · {fmtAgo(d.last_seen)}
              </span>
            ) : (
              <span className="text-danger">{t("nicht erreichbar")}</span>
            )}
            {d.locked && <Badge tone="warn">gesperrt</Badge>}
          </div>
        </div>
        <Button
          variant="ghost"
          busy={refreshing}
          onClick={async () => {
            setRefreshing(true);
            try {
              upsertDevice(await api<Device>(`/devices/${id}/refresh`, { method: "POST" }));
            } catch (e: any) {
              toast(t("Fehler"), e.message, true);
            } finally {
              setRefreshing(false);
            }
          }}
          title={t("Aktualisieren")}
        >
          <RefreshCw size={16} />
        </Button>
      </div>

      <Segmented
        value={tab === "diagnostics" ? "diagnostics" : "control"}
        onChange={(v) => navigate(v === "control" ? `/device/${id}` : `/device/${id}/${v}`, true)}
        options={[
          { value: "control", label: t("Steuerung") },
          { value: "diagnostics", label: t("Diagnose") },
        ]}
      />

      {!d.online && d.error && (
        <div className="rounded-2xl bg-danger/10 border border-danger/30 text-[13px] p-3 flex gap-2">
          <AlertTriangle size={16} className="text-danger shrink-0 mt-0.5" />
          <div>
            {d.error}
            {me?.role === "admin" && (
              <div className="mt-1">
                <Link to="/settings/devices" className="underline">
                  Adresse neu suchen / Verbindung prüfen
                </Link>
              </div>
            )}
          </div>
        </div>
      )}
      {s?.filter_alert && (
        <div className="rounded-2xl bg-warn/10 border border-warn/30 text-[13px] p-3 flex gap-2">
          <AlertTriangle size={16} className="text-warn shrink-0" /> {t("Filter reinigen")}: Die Anlage meldet, dass der Filter gereinigt werden sollte.
        </div>
      )}
      {!!s?.error_code && (
        <div className="rounded-2xl bg-danger/10 border border-danger/30 text-[13px] p-3">
          {t("Fehlercode")} {s.error_code} – siehe Fehlertabelle der Bedienungsanleitung.
        </div>
      )}

      {tab === "diagnostics" ? (
        <DiagnosticsView id={id} />
      ) : (
        <>
          <div className="grid gap-4 lg:grid-cols-[1fr_1fr]">
            <Card>
              <div className="flex items-center justify-between mb-2">
                <Badge tone={on ? "accent" : "muted"}>
                  <ModeIcon mode={s?.mode} size={12} /> {on ? MODES.find((m) => m.id === s?.mode)?.label && t(MODES.find((m) => m.id === s?.mode)!.label) : t("Aus")}
                </Badge>
                <button
                  disabled={dis || busy}
                  onClick={() => send({ power: !on })}
                  className={cx(
                    "flex items-center gap-2 rounded-full px-4 h-10 font-semibold text-[14px] transition disabled:opacity-50",
                    on ? "bg-accent text-on-accent" : "bg-surface2 border border-line",
                  )}
                >
                  <Power size={18} /> {on ? t("Ein") : t("Aus")}
                </button>
              </div>
              <Thermostat
                value={s?.target_temperature ?? 24}
                indoor={s?.indoor_temperature}
                color={color}
                min={d.min_target || 16}
                max={d.max_target || 30}
                disabled={dis || !av.target}
                onChange={(v) => send({ target_temperature: v })}
              />
              <div className="mt-4">
                <Segmented
                  value={s?.mode}
                  disabled={dis}
                  onChange={(m) => send(on ? { mode: m } : { mode: m, power: true })}
                  options={MODES.map((m) => ({ value: m.id, label: <><ModeIcon mode={m.id} size={15} /><span className="hidden min-[420px]:inline">{t(m.label)}</span></>, color: m.color }))}
                />
              </div>
            </Card>

            <div className="flex flex-col gap-4">
              <Card>
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 mb-4">
                  <Stat label={t("Innen")} value={fmtTemp(s?.indoor_temperature)} />
                  <Stat label={t("Außen")} value={fmtTemp(s?.outdoor_temperature)} />
                  <Stat label={t("Leistung")} value={s?.power_watts != null ? `${fmtNum(s.power_watts, 0)} W` : "–"} sub={d.cost_per_hour != null ? `${fmtMoney(d.cost_per_hour, d.today?.currency)}/h` : undefined} />
                  <Stat label={t("Heute")} value={d.today ? `${fmtNum(d.today.kwh, 2)} kWh` : "–"} sub={d.today ? fmtMoney(d.today.cost, d.today.currency) : undefined} />
                  <Stat label={t("Gesamt")} value={s?.total_energy_kwh != null ? `${fmtNum(s.total_energy_kwh, 1)} kWh` : "–"} />
                  <Stat label={t("Laufzeit")} value={d.today ? `${fmtNum(d.today.on_hours, 1)} h` : "–"} sub={t("Heute")} />
                </div>
                <FanControl value={s?.fan_speed ?? 102} disabled={dis || !av.fan} onChange={(v) => send({ fan_speed: v })} />
                <div className="mt-3 flex justify-end">
                  <Link to={`/history/${id}`} className="text-[13px] text-accent flex items-center gap-1">
                    <BarChart3 size={14} /> {t("Verlauf")}
                  </Link>
                </div>
              </Card>
            </div>
          </div>

          <Card title={t("Komfort")}>
            <div className="grid grid-cols-2 sm:grid-cols-5 gap-2">
              <Tile icon={<ArrowUpDown size={20} />} label={t("Schwenken")} on={s?.swing} disabled={dis} onClick={() => send({ swing: !s?.swing })} />
              <Tile icon={<Leaf size={20} />} label={t("Eco")} on={s?.eco} disabled={dis || !av.eco} why="Eco nur beim Kühlen" onClick={() => send({ eco: !s?.eco })} />
              <Tile icon={<Rocket size={20} />} label={t("Turbo")} on={s?.turbo} disabled={dis || !av.turbo} why="Turbo nur beim Kühlen/Heizen" onClick={() => send({ turbo: !s?.turbo })} />
              <Tile icon={<Bed size={20} />} label={t("Schlafmodus")} on={s?.sleep} disabled={dis || !av.sleep} why="Schlafmodus nicht beim Lüften/Entfeuchten" onClick={() => send({ sleep: !s?.sleep })} />
              <Tile icon={<VolumeX size={20} />} label={t("Außen leise")} on={s?.outdoor_silent} disabled={dis || !av.outdoor_silent} why="Nur beim Kühlen/Heizen" onClick={() => send({ outdoor_silent: !s?.outdoor_silent })} />
            </div>
          </Card>

          <Card title={t("Weitere Funktionen")}>
            <div className="grid grid-cols-2 sm:grid-cols-6 gap-2">
              <Tile icon={<Gauge size={20} />} label={t("iECO")} on={s?.ieco} disabled={dis} onClick={() => send({ ieco: !s?.ieco })} />
              <Tile icon={<Sparkles size={20} />} label={t("Ionisierer")} on={s?.purifier} disabled={dis} onClick={() => send({ purifier: !s?.purifier })} />
              <Tile icon={<Snowflake size={20} />} label={t("Frostschutz 8 °C")} on={s?.freeze_protection} disabled={dis || !av.freeze_protection} why="Nur beim Heizen" onClick={() => send({ freeze_protection: !s?.freeze_protection })} />
              <Tile icon={<SprayCan size={20} />} label={t("Selbstreinigung")} on={s?.self_clean} disabled={dis} onClick={() => (s?.self_clean ? send({ self_clean: false }) : setConfirmClean(true))} />
              <Tile icon={<Monitor size={20} />} label={t("Anzeige")} on={s?.display_on} disabled={dis} onClick={() => send({ display_on: !s?.display_on })} />
              <Tile icon={s?.sound === false ? <VolumeX size={20} /> : <Volume2 size={20} />} label={t("Signalton")} on={s?.sound} disabled={dis} onClick={() => send({ sound: !s?.sound })} />
            </div>
            <Row label={t("Leistungsbegrenzung")} hint="Begrenzt die Leistungsaufnahme (PortaSplit: 50 % oder keine).">
              <Segmented
                value={s?.rate_percent ?? 100}
                disabled={dis}
                onChange={(v) => send({ rate_percent: v })}
                options={[
                  { value: 50, label: "50 %" },
                  { value: 75, label: "75 %" },
                  { value: 100, label: t("Keine") },
                ]}
              />
            </Row>
          </Card>

          <TimersCard d={d} scenes={scenes} />

          {scenes.length > 0 && (
            <Card title={t("Szenen")}>
              <div className="flex flex-wrap gap-2">
                {scenes.map((sc) => (
                  <Button
                    key={sc.id}
                    disabled={dis}
                    onClick={async () => {
                      try {
                        await api(`/scenes/${sc.id}/apply?device=${id}`, { method: "POST" });
                        toast(sc.name, t("erledigt"));
                      } catch (e: any) {
                        toast(sc.name, e.message, true);
                      }
                    }}
                  >
                    <Sparkles size={15} /> {sc.name}
                  </Button>
                ))}
              </div>
            </Card>
          )}

          <div className="text-[12px] text-muted text-center flex items-center justify-center gap-2">
            <AlarmClock size={12} /> ID {d.id} · {d.ip || "Cloud"}
          </div>
        </>
      )}

      <Modal open={confirmClean} onClose={() => setConfirmClean(false)} title={t("Selbstreinigung")}>
        <p className="text-[14px] mb-4">{t("Selbstreinigung starten? Sie dauert etwa 15 Minuten, die Anlage ist in dieser Zeit nicht nutzbar.")}</p>
        <div className="flex justify-end gap-2">
          <Button onClick={() => setConfirmClean(false)}>{t("Abbrechen")}</Button>
          <Button
            variant="primary"
            onClick={async () => {
              setConfirmClean(false);
              await send({ self_clean: true });
            }}
          >
            Starten
          </Button>
        </div>
      </Modal>
    </div>
  );
}
