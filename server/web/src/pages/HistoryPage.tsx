import { useEffect, useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Download } from "lucide-react";
import { api, type EnergyReply, type Sample } from "../api";
import { sortedDevices, useStore } from "../store";
import { Button, Card, Empty, Segmented, Spinner, Stat, navigate } from "../ui";
import { fmtMoney, fmtNum, language, t } from "../i18n";
import { BarChart, LineChart } from "./Charts";

const SPANS = [
  { value: 6, label: "6 h" },
  { value: 24, label: "24 h" },
  { value: 72, label: "3 d" },
  { value: 168, label: "7 d" },
  { value: 720, label: "30 d" },
];

export default function HistoryPage({ id }: { id?: string }) {
  const devices = useStore((s) => s.devices);
  const list = useMemo(() => sortedDevices(devices), [devices]);
  const dev = id ?? list[0]?.id;

  const [hours, setHours] = useState(24);
  const [samples, setSamples] = useState<Sample[] | null>(null);
  const [period, setPeriod] = useState<"day" | "week" | "month" | "year">("week");
  const [offset, setOffset] = useState(0);
  const [energy, setEnergy] = useState<EnergyReply | null>(null);

  useEffect(() => {
    if (!dev) return;
    setSamples(null);
    api<{ points: Sample[] }>(`/devices/${dev}/history?hours=${hours}&points=700`).then((r) => setSamples(r.points)).catch(() => setSamples([]));
  }, [dev, hours]);

  useEffect(() => {
    if (!dev) return;
    api<EnergyReply>(`/devices/${dev}/energy?period=${period}&offset=${offset}`).then(setEnergy).catch(() => setEnergy(null));
  }, [dev, period, offset]);

  if (!dev) return <Card><Empty>{t("Noch keine Anlage eingerichtet.")}</Empty></Card>;

  const xs = samples?.map((s) => s.t) ?? [];
  const loc = language() === "de" ? "de-DE" : "en-GB";
  const label = (iso: string) => {
    const d = new Date(iso);
    switch (period) {
      case "day":
        return `${d.getHours()}`;
      case "year":
        return d.toLocaleDateString(loc, { month: "short" });
      case "week":
        return d.toLocaleDateString(loc, { weekday: "short" });
      default:
        return `${d.getDate()}`;
    }
  };
  const title = () => {
    if (!energy) return "";
    const f = new Date(energy.from);
    switch (period) {
      case "day":
        return f.toLocaleDateString(loc, { weekday: "long", day: "2-digit", month: "long" });
      case "week":
        return `${f.toLocaleDateString(loc, { day: "2-digit", month: "2-digit" })} – ${new Date(new Date(energy.to).getTime() - 1).toLocaleDateString(loc, { day: "2-digit", month: "2-digit", year: "numeric" })}`;
      case "month":
        return f.toLocaleDateString(loc, { month: "long", year: "numeric" });
      default:
        return String(f.getFullYear());
    }
  };

  return (
    <div className="flex flex-col gap-4">
      {list.length > 1 && (
        <Segmented value={dev} onChange={(v) => navigate(`/history/${v}`, true)} options={list.map((d) => ({ value: d.id, label: d.name }))} />
      )}

      <Card
        title={t("Verbrauch")}
        action={
          <div className="flex items-center gap-1">
            <Button variant="ghost" className="h-8 px-2" onClick={() => setOffset(offset - 1)}>
              <ChevronLeft size={16} />
            </Button>
            <Button variant="ghost" className="h-8 px-2" disabled={offset >= 0} onClick={() => setOffset(offset + 1)}>
              <ChevronRight size={16} />
            </Button>
          </div>
        }
      >
        <Segmented
          className="mb-3"
          value={period}
          onChange={(p) => {
            setPeriod(p);
            setOffset(0);
          }}
          options={[
            { value: "day", label: t("Tag") },
            { value: "week", label: t("Woche") },
            { value: "month", label: t("Monat") },
            { value: "year", label: t("Jahr") },
          ]}
        />
        {!energy ? (
          <Spinner />
        ) : (
          <>
            <div className="text-[13px] text-muted mb-2">{title()}</div>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 mb-3">
              <Stat label={t("Verbrauch")} value={`${fmtNum(energy.total.kwh, 2)} kWh`} />
              <Stat label={t("Kosten")} value={fmtMoney(energy.total.cost, energy.total.currency)} />
              <Stat label={t("Laufzeit")} value={`${fmtNum(energy.total.on_hours, 1)} h`} />
              <Stat label="CO₂" value={`${fmtNum(energy.total.co2_kg, 1)} kg`} />
            </div>
            <BarChart
              labels={energy.buckets.map((b) => label(b.start))}
              values={energy.buckets.map((b) => b.kwh)}
              unit="kWh"
              sub={(i) => `${fmtMoney(energy.buckets[i].cost, energy.buckets[i].currency)} · ${fmtNum(energy.buckets[i].on_hours, 1)} h`}
            />
          </>
        )}
      </Card>

      <Card
        title={t("Temperaturen")}
        action={
          <a className="text-[13px] text-accent flex items-center gap-1" href={`/api/v1/devices/${dev}/history.csv?days=${Math.max(1, Math.ceil(hours / 24))}${language() === "de" ? "&comma=1" : ""}`}>
            <Download size={14} /> {t("CSV exportieren")}
          </a>
        }
      >
        <Segmented className="mb-3" value={hours} onChange={setHours} options={SPANS} />
        {!samples ? (
          <Spinner />
        ) : (
          <>
            <LineChart
              xs={xs}
              series={[
                { name: t("Innen"), color: "var(--accent)", values: samples.map((s) => s.indoor), unit: "°C" },
                { name: t("Außen"), color: "var(--heat)", values: samples.map((s) => s.outdoor), unit: "°C" },
                { name: t("Soll"), color: "var(--muted)", values: samples.map((s) => (s.on ? s.target : null)), unit: "°C", dashed: true, step: true },
              ]}
            />
            <div className="mt-5 text-[14px] font-semibold mb-2">{t("Leistung")}</div>
            <LineChart xs={xs} height={160} series={[{ name: t("Leistung"), color: "var(--fan)", values: samples.map((s) => s.watts), unit: "W" }]} />
          </>
        )}
      </Card>
    </div>
  );
}
