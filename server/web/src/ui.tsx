import { useEffect, useState, useSyncExternalStore, type ReactNode } from "react";
import { Loader2, X } from "lucide-react";
import { t } from "./i18n";

// ---- tiny router ---------------------------------------------------------

const routeListeners = new Set<() => void>();
window.addEventListener("popstate", () => routeListeners.forEach((l) => l()));

export function navigate(path: string, replace = false) {
  if (replace) history.replaceState(null, "", path);
  else history.pushState(null, "", path);
  routeListeners.forEach((l) => l());
  window.scrollTo({ top: 0 });
}

export function usePath() {
  return useSyncExternalStore(
    (cb) => {
      routeListeners.add(cb);
      return () => routeListeners.delete(cb);
    },
    () => location.pathname,
  );
}

export function Link({ to, children, className }: { to: string; children: ReactNode; className?: string }) {
  return (
    <a
      href={to}
      className={className}
      onClick={(e) => {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
        e.preventDefault();
        navigate(to);
      }}
    >
      {children}
    </a>
  );
}

// ---- building blocks -------------------------------------------------------

export function cx(...c: (string | false | null | undefined)[]) {
  return c.filter(Boolean).join(" ");
}

export function Card({ children, className, title, action }: { children: ReactNode; className?: string; title?: ReactNode; action?: ReactNode }) {
  return (
    <section className={cx("rounded-[20px] bg-surface border border-line p-4 sm:p-5", className)}>
      {(title || action) && (
        <div className="flex items-center justify-between gap-2 mb-3">
          {title && <h2 className="text-[15px] font-semibold tracking-tight">{title}</h2>}
          {action}
        </div>
      )}
      {children}
    </section>
  );
}

export function Button({
  children,
  onClick,
  variant = "default",
  disabled,
  busy,
  className,
  type = "button",
  title,
}: {
  children: ReactNode;
  onClick?: () => unknown;
  variant?: "default" | "primary" | "danger" | "ghost";
  disabled?: boolean;
  busy?: boolean;
  className?: string;
  type?: "button" | "submit";
  title?: string;
}) {
  const [running, setRunning] = useState(false);
  const base = "inline-flex items-center justify-center gap-2 rounded-xl px-3.5 h-10 text-[14px] font-medium transition active:scale-[.98] disabled:opacity-50 disabled:pointer-events-none whitespace-nowrap";
  const styles = {
    default: "bg-surface2 hover:brightness-95 text-text border border-line",
    primary: "bg-accent text-on-accent hover:brightness-110",
    danger: "bg-danger text-white hover:brightness-110",
    ghost: "hover:bg-surface2 text-text",
  }[variant];
  return (
    <button
      type={type}
      title={title}
      className={cx(base, styles, className)}
      disabled={disabled || busy || running}
      onClick={async () => {
        if (!onClick) return;
        const r = onClick();
        if (r instanceof Promise) {
          setRunning(true);
          try {
            await r;
          } finally {
            setRunning(false);
          }
        }
      }}
    >
      {(busy || running) && <Loader2 size={16} className="animate-spin" />}
      {children}
    </button>
  );
}

export function Toggle({ on, onChange, disabled, label }: { on: boolean; onChange: (v: boolean) => void; disabled?: boolean; label?: string }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!on)}
      className={cx(
        "relative w-12 h-7 rounded-full transition shrink-0 disabled:opacity-40",
        on ? "bg-accent" : "bg-surface2 border border-line",
      )}
    >
      <span className={cx("absolute top-1 h-5 w-5 rounded-full bg-white shadow transition-all", on ? "left-6" : "left-1")} />
    </button>
  );
}

export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  disabled,
  className,
}: {
  value: T | undefined;
  options: { value: T; label: ReactNode; color?: string }[];
  onChange: (v: T) => void;
  disabled?: boolean;
  className?: string;
}) {
  return (
    <div className={cx("flex rounded-xl bg-surface2 p-1 gap-1 border border-line scroll-x", className)}>
      {options.map((o) => {
        const active = o.value === value;
        return (
          <button
            key={String(o.value)}
            type="button"
            disabled={disabled}
            onClick={() => onChange(o.value)}
            style={active && o.color ? { background: o.color, color: "#fff" } : undefined}
            className={cx(
              "flex-1 min-w-fit px-3 h-9 rounded-lg text-[13px] font-medium transition flex items-center justify-center gap-1.5 disabled:opacity-50",
              active ? (o.color ? "" : "bg-surface shadow-sm") : "text-muted hover:text-text",
            )}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

export function Field({ label, children, hint }: { label: ReactNode; children: ReactNode; hint?: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5 text-[13px]">
      <span className="text-muted font-medium">{label}</span>
      {children}
      {hint && <span className="text-[12px] text-muted">{hint}</span>}
    </label>
  );
}

export function Row({ label, children, hint }: { label: ReactNode; children: ReactNode; hint?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 py-2.5 border-b border-line last:border-0">
      <div className="min-w-0">
        <div className="text-[14px]">{label}</div>
        {hint && <div className="text-[12px] text-muted">{hint}</div>}
      </div>
      <div className="shrink-0 flex items-center gap-2">{children}</div>
    </div>
  );
}

export function Spinner({ text }: { text?: string }) {
  return (
    <div className="flex items-center gap-2 text-muted text-sm py-6 justify-center">
      <Loader2 className="animate-spin" size={18} /> {text ?? t("Laden …")}
    </div>
  );
}

export function Modal({ open, onClose, title, children, wide }: { open: boolean; onClose: () => void; title: ReactNode; children: ReactNode; wide?: boolean }) {
  useEffect(() => {
    if (!open) return;
    const k = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", k);
    return () => window.removeEventListener("keydown", k);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-black/40 backdrop-blur-[2px] p-0 sm:p-4" onClick={onClose}>
      <div
        className={cx("bg-surface w-full rounded-t-[22px] sm:rounded-[22px] border border-line max-h-[92dvh] overflow-y-auto", wide ? "sm:max-w-2xl" : "sm:max-w-lg")}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-5 pt-4 pb-2 sticky top-0 bg-surface z-10">
          <h3 className="font-semibold text-[16px]">{title}</h3>
          <button className="p-2 -mr-2 rounded-lg hover:bg-surface2" onClick={onClose} aria-label={t("Schließen")}>
            <X size={18} />
          </button>
        </div>
        <div className="px-5 pb-5">{children}</div>
      </div>
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="text-center text-muted text-[14px] py-8">{children}</div>;
}

export function Badge({ children, tone = "muted" }: { children: ReactNode; tone?: "muted" | "ok" | "warn" | "danger" | "accent" }) {
  const c = {
    muted: "bg-surface2 text-muted",
    ok: "bg-ok/15 text-ok",
    warn: "bg-warn/15 text-warn",
    danger: "bg-danger/15 text-danger",
    accent: "bg-accent-soft text-accent",
  }[tone];
  return <span className={cx("inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-semibold", c)}>{children}</span>;
}

export function Stat({ label, value, sub, icon }: { label: string; value: ReactNode; sub?: ReactNode; icon?: ReactNode }) {
  return (
    <div className="rounded-2xl bg-surface2/70 px-3 py-2.5 min-w-0">
      <div className="text-[11px] uppercase tracking-wide text-muted flex items-center gap-1">
        {icon}
        {label}
      </div>
      <div className="text-[18px] font-semibold tabular truncate">{value}</div>
      {sub && <div className="text-[11px] text-muted truncate">{sub}</div>}
    </div>
  );
}
