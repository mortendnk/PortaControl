import type { Control } from "../api";
import { FAN_PRESETS, MODES, t } from "../i18n";
import { Toggle } from "../ui";

// Edits a partial control request: every field can be "unchanged".
type Key = keyof Control;

const BOOLS: { k: Key; label: string }[] = [
  { k: "swing", label: "Schwenken" },
  { k: "eco", label: "Eco" },
  { k: "turbo", label: "Turbo" },
  { k: "sleep", label: "Schlafmodus" },
  { k: "purifier", label: "Ionisierer" },
  { k: "outdoor_silent", label: "Außen leise" },
  { k: "ieco", label: "iECO" },
  { k: "freeze_protection", label: "Frostschutz 8 °C" },
  { k: "display_on", label: "Anzeige" },
];

export default function ControlEditor({ value, onChange, hidePower }: { value: Control; onChange: (c: Control) => void; hidePower?: boolean }) {
  const set = (k: Key, v: any) => {
    const n = { ...value } as any;
    if (v === undefined) delete n[k];
    else n[k] = v;
    onChange(n);
  };
  const has = (k: Key) => value[k] !== undefined;

  const line = (k: Key, label: string, input: React.ReactNode, def: any) => (
    <div className="flex items-center gap-3 py-2 border-b border-line last:border-0" key={k}>
      <input type="checkbox" checked={has(k)} onChange={(e) => set(k, e.target.checked ? def : undefined)} />
      <span className={"flex-1 text-[14px] " + (has(k) ? "" : "text-muted")}>{t(label)}</span>
      <div className={has(k) ? "" : "opacity-40 pointer-events-none"}>{input}</div>
    </div>
  );

  return (
    <div>
      {!hidePower && line("power", "Ein", <Toggle on={!!value.power} onChange={(v) => set("power", v)} />, true)}
      {line(
        "mode",
        "Modus",
        <select value={value.mode ?? "cool"} onChange={(e) => set("mode", e.target.value)}>
          {MODES.map((m) => (
            <option key={m.id} value={m.id}>
              {t(m.label)}
            </option>
          ))}
        </select>,
        "cool",
      )}
      {line(
        "target_temperature",
        "Soll",
        <select value={value.target_temperature ?? 24} onChange={(e) => set("target_temperature", Number(e.target.value))}>
          {Array.from({ length: 15 }, (_, i) => 16 + i).map((v) => (
            <option key={v} value={v}>
              {v} °C
            </option>
          ))}
        </select>,
        24,
      )}
      {line(
        "fan_speed",
        "Lüfter",
        <select value={value.fan_speed ?? 102} onChange={(e) => set("fan_speed", Number(e.target.value))}>
          <option value={102}>{t("Auto")}</option>
          {FAN_PRESETS.map((p) => (
            <option key={p.v} value={p.v}>
              {t(p.label)} ({p.v} %)
            </option>
          ))}
        </select>,
        102,
      )}
      {BOOLS.map((b) => line(b.k, b.label, <Toggle on={!!value[b.k]} onChange={(v) => set(b.k, v)} />, true))}
      {line(
        "rate_percent",
        "Leistungsbegrenzung",
        <select value={value.rate_percent ?? 100} onChange={(e) => set("rate_percent", Number(e.target.value))}>
          <option value={50}>50 %</option>
          <option value={75}>75 %</option>
          <option value={100}>{t("Keine")}</option>
        </select>,
        100,
      )}
    </div>
  );
}

export function describeControl(c: Control): string {
  const p: string[] = [];
  if (c.power === false) return t("Aus");
  if (c.mode) p.push(t(MODES.find((m) => m.id === c.mode)?.label ?? c.mode));
  if (c.target_temperature) p.push(`${c.target_temperature} °C`);
  if (c.fan_speed) p.push(c.fan_speed === 102 ? `${t("Lüfter")} ${t("Auto")}` : `${t("Lüfter")} ${c.fan_speed} %`);
  for (const b of BOOLS) if (c[b.k] !== undefined) p.push(`${t(b.label)} ${c[b.k] ? t("Ein").toLowerCase() : t("Aus").toLowerCase()}`);
  if (c.rate_percent) p.push(`${t("Leistung")} ${c.rate_percent} %`);
  if (p.length === 0 && c.power) return t("Ein");
  return p.join(" · ");
}
