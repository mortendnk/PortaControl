import { useEffect, useRef, useState } from "react";
import { Minus, Plus } from "lucide-react";
import { fmtNum, fmtTemp, t } from "../i18n";

const MIN = 16;
const MAX = 30;
const START = 135; // degrees, 0 = right, clockwise
const SWEEP = 270;

function polar(cx: number, cy: number, r: number, deg: number) {
  const a = (deg * Math.PI) / 180;
  return { x: cx + r * Math.cos(a), y: cy + r * Math.sin(a) };
}

function arc(cx: number, cy: number, r: number, from: number, to: number) {
  const a = polar(cx, cy, r, from);
  const b = polar(cx, cy, r, to);
  const large = to - from > 180 ? 1 : 0;
  return `M ${a.x} ${a.y} A ${r} ${r} 0 ${large} 1 ${b.x} ${b.y}`;
}

// A round thermostat: drag on the ring or use +/-. The value is sent after
// the user stops changing it.
export default function Thermostat({
  value,
  indoor,
  color,
  disabled,
  min = MIN,
  max = MAX,
  label,
  onChange,
}: {
  value: number;
  indoor: number | null | undefined;
  color: string;
  disabled?: boolean;
  min?: number;
  max?: number;
  label?: string;
  onChange: (v: number) => void;
}) {
  const [local, setLocal] = useState(value);
  const dragging = useRef(false);
  const timer = useRef<number | undefined>(undefined);
  const svg = useRef<SVGSVGElement>(null);

  useEffect(() => {
    if (!dragging.current && timer.current === undefined) setLocal(value);
  }, [value]);

  const commit = (v: number) => {
    const c = Math.max(min, Math.min(max, Math.round(v)));
    setLocal(c);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      timer.current = undefined;
      if (c !== value) onChange(c);
    }, 700);
  };

  const fromPointer = (e: React.PointerEvent) => {
    const r = svg.current!.getBoundingClientRect();
    const x = e.clientX - (r.left + r.width / 2);
    const y = e.clientY - (r.top + r.height / 2);
    let deg = (Math.atan2(y, x) * 180) / Math.PI;
    if (deg < 0) deg += 360;
    let rel = deg - START;
    if (rel < 0) rel += 360;
    if (rel > SWEEP) rel = rel > SWEEP + (360 - SWEEP) / 2 ? 0 : SWEEP;
    return MIN + (rel / SWEEP) * (MAX - MIN);
  };

  const frac = (local - MIN) / (MAX - MIN);
  const knob = polar(100, 100, 82, START + frac * SWEEP);

  return (
    <div className="flex flex-col items-center select-none">
      <div className="relative w-[260px] max-w-full aspect-square">
        <svg
          ref={svg}
          viewBox="0 0 200 200"
          className={disabled ? "opacity-50" : "cursor-pointer touch-none"}
          onPointerDown={(e) => {
            if (disabled) return;
            dragging.current = true;
            (e.target as Element).setPointerCapture?.(e.pointerId);
            commit(fromPointer(e));
          }}
          onPointerMove={(e) => dragging.current && commit(fromPointer(e))}
          onPointerUp={() => (dragging.current = false)}
          onPointerCancel={() => (dragging.current = false)}
        >
          <path d={arc(100, 100, 82, START, START + SWEEP)} stroke="var(--surface2)" strokeWidth="14" fill="none" strokeLinecap="round" />
          {frac > 0.001 && <path d={arc(100, 100, 82, START, START + frac * SWEEP)} stroke={color} strokeWidth="14" fill="none" strokeLinecap="round" />}
          {Array.from({ length: MAX - MIN + 1 }, (_, i) => {
            const p1 = polar(100, 100, 66, START + (i / (MAX - MIN)) * SWEEP);
            const p2 = polar(100, 100, i % 2 === 0 ? 61 : 63, START + (i / (MAX - MIN)) * SWEEP);
            return <line key={i} x1={p1.x} y1={p1.y} x2={p2.x} y2={p2.y} stroke="var(--line)" strokeWidth="1.5" />;
          })}
          <circle cx={knob.x} cy={knob.y} r="11" fill="var(--surface)" stroke={color} strokeWidth="4" />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none">
          <div className="text-[12px] text-muted uppercase tracking-wide">{label ?? t("Soll")}</div>
          <div className="text-[56px] leading-none font-semibold tabular">
            {fmtNum(local, 0)}
            <span className="text-[24px] align-top">°</span>
          </div>
          <div className="text-[13px] text-muted mt-1">
            {t("Innen")} {fmtTemp(indoor)}
          </div>
        </div>
      </div>
      <div className="flex items-center gap-6 -mt-4">
        <button
          disabled={disabled || local <= min}
          onClick={() => commit(local - 1)}
          className="w-12 h-12 rounded-full bg-surface2 border border-line grid place-items-center disabled:opacity-40"
          aria-label="-1 °C"
        >
          <Minus size={20} />
        </button>
        <button
          disabled={disabled || local >= max}
          onClick={() => commit(local + 1)}
          className="w-12 h-12 rounded-full bg-surface2 border border-line grid place-items-center disabled:opacity-40"
          aria-label="+1 °C"
        >
          <Plus size={20} />
        </button>
      </div>
    </div>
  );
}
