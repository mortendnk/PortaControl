import { useMemo, useRef, useState } from "react";
import { fmtNum, t } from "../i18n";

export interface Series {
  name: string;
  color: string;
  values: (number | null)[];
  dashed?: boolean;
  unit: string;
  step?: boolean;
}

// Line chart with shared x axis (unix seconds), hover read-out and nice ticks.
export function LineChart({ xs, series, height = 220 }: { xs: number[]; series: Series[]; height?: number }) {
  const W = 560;
  const H = height;
  const pad = { l: 40, r: 12, t: 12, b: 26 };
  const ref = useRef<SVGSVGElement>(null);
  const [hover, setHover] = useState<number | null>(null);

  const { min, max, x0, x1 } = useMemo(() => {
    let min = Infinity;
    let max = -Infinity;
    for (const s of series) for (const v of s.values) if (v != null) ((min = Math.min(min, v)), (max = Math.max(max, v)));
    if (!Number.isFinite(min)) ((min = 0), (max = 1));
    if (max - min < 2) ((min -= 1), (max += 1));
    const span = max - min;
    return { min: Math.floor(min - span * 0.05), max: Math.ceil(max + span * 0.05), x0: xs[0] ?? 0, x1: xs[xs.length - 1] ?? 1 };
  }, [xs, series]);

  if (xs.length < 2) return <div className="text-muted text-[13px] text-center py-10">{t("Keine Daten")}</div>;

  const X = (x: number) => pad.l + ((x - x0) / Math.max(1, x1 - x0)) * (W - pad.l - pad.r);
  const Y = (y: number) => pad.t + (1 - (y - min) / (max - min)) * (H - pad.t - pad.b);

  const ticks = 4;
  const yTicks = Array.from({ length: ticks + 1 }, (_, i) => min + ((max - min) * i) / ticks);
  const spanH = (x1 - x0) / 3600;
  const xTickCount = 6;
  const xTicks = Array.from({ length: xTickCount }, (_, i) => x0 + ((x1 - x0) * i) / (xTickCount - 1));
  const fmtX = (x: number) => {
    const d = new Date(x * 1000);
    return spanH > 48 ? d.toLocaleDateString(undefined, { day: "2-digit", month: "2-digit" }) : d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  };

  const path = (s: Series) => {
    let d = "";
    let pen = false;
    s.values.forEach((v, i) => {
      if (v == null || (i > 0 && xs[i] - xs[i - 1] > 1800)) {
        pen = false;
        if (v == null) return;
      }
      const px = X(xs[i]);
      const py = Y(v);
      if (!pen) d += `M${px},${py}`;
      else if (s.step) d += `H${px}V${py}`;
      else d += `L${px},${py}`;
      pen = true;
    });
    return d;
  };

  const onMove = (e: React.PointerEvent) => {
    const r = ref.current!.getBoundingClientRect();
    const x = ((e.clientX - r.left) / r.width) * W;
    const tx = x0 + ((x - pad.l) / (W - pad.l - pad.r)) * (x1 - x0);
    let best = 0;
    for (let i = 1; i < xs.length; i++) if (Math.abs(xs[i] - tx) < Math.abs(xs[best] - tx)) best = i;
    setHover(best);
  };

  return (
    <div className="relative">
      <svg ref={ref} viewBox={`0 0 ${W} ${H}`} className="w-full h-auto touch-pan-y" onPointerMove={onMove} onPointerLeave={() => setHover(null)}>
        {yTicks.map((v, i) => (
          <g key={i}>
            <line x1={pad.l} x2={W - pad.r} y1={Y(v)} y2={Y(v)} stroke="var(--line)" strokeWidth="1" />
            <text x={pad.l - 6} y={Y(v) + 4} textAnchor="end" fontSize="12" fill="var(--muted)">
              {fmtNum(v, 0)}
            </text>
          </g>
        ))}
        {xTicks.map((x, i) => (
          <text key={i} x={X(x)} y={H - 6} textAnchor={i === 0 ? "start" : i === xTickCount - 1 ? "end" : "middle"} fontSize="12" fill="var(--muted)">
            {fmtX(x)}
          </text>
        ))}
        {series.map((s) => (
          <path key={s.name} d={path(s)} fill="none" stroke={s.color} strokeWidth="2" strokeDasharray={s.dashed ? "5 4" : undefined} strokeLinejoin="round" />
        ))}
        {hover != null && <line x1={X(xs[hover])} x2={X(xs[hover])} y1={pad.t} y2={H - pad.b} stroke="var(--muted)" strokeWidth="1" />}
      </svg>
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-[12px] mt-1">
        {series.map((s) => (
          <span key={s.name} className="flex items-center gap-1.5">
            <span className="w-3 h-[3px] rounded" style={{ background: s.color }} />
            {s.name}
            {hover != null && <b className="tabular">{s.values[hover] == null ? "–" : `${fmtNum(s.values[hover]!, 1)} ${s.unit}`}</b>}
          </span>
        ))}
        {hover != null && <span className="text-muted ml-auto">{new Date(xs[hover] * 1000).toLocaleString()}</span>}
      </div>
    </div>
  );
}

// Bar chart for energy buckets.
export function BarChart({ labels, values, unit, color = "var(--accent)", height = 200, sub }: { labels: string[]; values: number[]; unit: string; color?: string; height?: number; sub?: (i: number) => string }) {
  const W = 560;
  const H = height;
  const pad = { l: 40, r: 8, t: 10, b: 24 };
  const [hover, setHover] = useState<number | null>(null);
  const max = Math.max(0.001, ...values) * 1.1;
  const bw = (W - pad.l - pad.r) / Math.max(1, values.length);
  const Y = (v: number) => pad.t + (1 - v / max) * (H - pad.t - pad.b);
  const every = Math.ceil(values.length / 12);

  return (
    <div>
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-auto" onPointerLeave={() => setHover(null)}>
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line x1={pad.l} x2={W - pad.r} y1={Y(max * f)} y2={Y(max * f)} stroke="var(--line)" />
            <text x={pad.l - 6} y={Y(max * f) + 4} textAnchor="end" fontSize="12" fill="var(--muted)">
              {fmtNum(max * f, max < 2 ? 2 : 1)}
            </text>
          </g>
        ))}
        {values.map((v, i) => (
          <g key={i} onPointerEnter={() => setHover(i)}>
            <rect x={pad.l + i * bw} y={pad.t} width={bw} height={H - pad.t - pad.b} fill="transparent" />
            <rect
              x={pad.l + i * bw + bw * 0.15}
              y={Y(v)}
              width={bw * 0.7}
              height={Math.max(0, H - pad.b - Y(v))}
              rx={Math.min(4, bw * 0.2)}
              fill={color}
              opacity={hover == null || hover === i ? 1 : 0.5}
            />
            {i % every === 0 && (
              <text x={pad.l + i * bw + bw / 2} y={H - 6} textAnchor="middle" fontSize="12" fill="var(--muted)">
                {labels[i]}
              </text>
            )}
          </g>
        ))}
      </svg>
      <div className="text-[12px] text-muted h-5">
        {hover != null && (
          <>
            <b className="text-text">{labels[hover]}</b>: {fmtNum(values[hover], 2)} {unit}
            {sub && ` · ${sub(hover)}`}
          </>
        )}
      </div>
    </div>
  );
}
