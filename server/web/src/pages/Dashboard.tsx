import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, ChevronRight, Droplets, Fan, Flame, Plus, Power, Snowflake, Sparkles, Wind, Zap } from "lucide-react";
import { api, type Device, type Scene } from "../api";
import { sortedDevices, toast, useStore } from "../store";
import { useControl } from "../control";
import { Badge, Button, Card, Empty, Link, Spinner, Stat, cx, navigate } from "../ui";
import { fmtMoney, fmtNum, fmtTemp, modeColor, modeLabel, t } from "../i18n";

export function ModeIcon({ mode, size = 18 }: { mode?: string; size?: number }) {
  switch (mode) {
    case "cool":
      return <Snowflake size={size} />;
    case "heat":
      return <Flame size={size} />;
    case "dry":
      return <Droplets size={size} />;
    case "fan":
      return <Fan size={size} />;
    default:
      return <Wind size={size} />;
  }
}

function DeviceCard({ d }: { d: Device }) {
  const { send, busy } = useControl(d.id);
  const s = d.state;
  const on = !!s?.power;

  return (
    <div
      className="rounded-[20px] bg-surface border border-line p-4 flex flex-col gap-3 cursor-pointer hover:border-accent/50 transition"
      onClick={() => navigate(`/device/${d.id}`)}
    >
      <div className="flex items-start gap-3">
        <div
          className="grid place-items-center w-11 h-11 rounded-2xl shrink-0 transition"
          style={{ background: on ? modeColor(s?.mode) : "var(--surface2)", color: on ? "#fff" : "var(--muted)" }}
        >
          <ModeIcon mode={s?.mode} size={22} />
        </div>
        <div className="flex-1 min-w-0">
          <div className="font-semibold truncate">{d.name}</div>
          <div className="text-[12px] text-muted flex flex-wrap gap-x-2">
            {d.room && <span>{d.room}</span>}
            {!d.online ? (
              <span className="text-danger">{t("nicht erreichbar")}</span>
            ) : on ? (
              <span>
                {modeLabel(s?.mode)} · {fmtNum(s?.target_temperature, 0)} °C
              </span>
            ) : (
              <span>{t("Aus")}</span>
            )}
          </div>
        </div>
        <button
          disabled={busy || !s}
          onClick={(e) => {
            e.stopPropagation();
            send({ power: !on });
          }}
          className={cx(
            "grid place-items-center w-11 h-11 rounded-full transition shrink-0 disabled:opacity-50",
            on ? "bg-accent text-on-accent" : "bg-surface2 text-muted border border-line",
          )}
          aria-label={on ? t("Ausschalten") : t("Einschalten")}
        >
          <Power size={20} />
        </button>
      </div>

      <div className="grid grid-cols-3 gap-2">
        <Stat label={t("Innen")} value={fmtTemp(s?.indoor_temperature)} />
        <Stat label={t("Außen")} value={fmtTemp(s?.outdoor_temperature)} />
        <Stat label={t("Leistung")} value={s?.power_watts != null ? `${fmtNum(s.power_watts, 0)} W` : "–"} />
      </div>

      <div className="flex items-center gap-2 text-[12px] text-muted">
        {d.today && (
          <span>
            {t("Heute")}: {fmtNum(d.today.kwh, 2)} kWh · {fmtMoney(d.today.cost, d.today.currency)}
          </span>
        )}
        <span className="ml-auto flex gap-1">
          {s?.filter_alert && (
            <Badge tone="warn">
              <AlertTriangle size={12} /> Filter
            </Badge>
          )}
          {!!s?.error_code && <Badge tone="danger">E{s.error_code}</Badge>}
          {s?.turbo && <Badge tone="accent">Turbo</Badge>}
          {s?.eco && <Badge tone="ok">Eco</Badge>}
          {d.path === "cloud" && <Badge>Cloud</Badge>}
        </span>
        <ChevronRight size={16} />
      </div>
    </div>
  );
}

export default function Dashboard() {
  const devices = useStore((s) => s.devices);
  const loaded = useStore((s) => s.loaded);
  const me = useStore((s) => s.me);
  const ver = useStore((s) => s.versions.scenes ?? 0);
  const list = useMemo(() => sortedDevices(devices), [devices]);
  const [scenes, setScenes] = useState<Scene[]>([]);

  useEffect(() => {
    api<Scene[]>("/scenes").then(setScenes).catch(() => {});
  }, [ver]);

  if (!loaded) return <Spinner />;

  const on = list.filter((d) => d.state?.power);
  const watts = list.reduce((a, d) => a + (d.state?.power ? (d.state?.power_watts ?? 0) : 0), 0);
  const kwh = list.reduce((a, d) => a + (d.today?.kwh ?? 0), 0);
  const cost = list.reduce((a, d) => a + (d.today?.cost ?? 0), 0);
  const currency = list[0]?.today?.currency ?? "EUR";

  if (list.length === 0) {
    return (
      <Card>
        <Empty>
          <p className="mb-4">{t("Noch keine Anlage eingerichtet.")}</p>
          {me?.role === "admin" ? (
            <Button variant="primary" onClick={() => navigate("/settings/devices?add=1")}>
              <Plus size={16} /> {t("Jetzt einrichten")}
            </Button>
          ) : (
            <Link to="/login" className="underline">
              {t("Anmelden")}
            </Link>
          )}
        </Empty>
      </Card>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
        <Stat label={t("Aktiv")} value={`${on.length} / ${list.length}`} icon={<Power size={12} />} />
        <Stat label={t("Leistung")} value={`${fmtNum(watts, 0)} W`} icon={<Zap size={12} />} />
        <Stat label={t("Heute")} value={`${fmtNum(kwh, 2)} kWh`} />
        <Stat label={t("Kosten")} value={fmtMoney(cost, currency)} sub={t("Heute")} />
      </div>

      {scenes.length > 0 && (
        <div className="flex gap-2 scroll-x -mx-4 px-4">
          {scenes.map((s) => (
            <Button
              key={s.id}
              className="shrink-0"
              onClick={async () => {
                try {
                  await api(`/scenes/${s.id}/apply`, { method: "POST" });
                  toast(s.name, t("erledigt"));
                } catch (e: any) {
                  toast(s.name, e.message, true);
                }
              }}
            >
              <Sparkles size={15} /> {s.name}
            </Button>
          ))}
          {list.length > 1 && on.length > 0 && (
            <Button
              className="shrink-0"
              onClick={async () => {
                await Promise.all(on.map((d) => api(`/devices/${d.id}/control`, { body: { power: false } }).catch(() => {})));
              }}
            >
              <Power size={15} /> Alle aus
            </Button>
          )}
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        {list.map((d) => (
          <DeviceCard key={d.id} d={d} />
        ))}
      </div>
    </div>
  );
}
