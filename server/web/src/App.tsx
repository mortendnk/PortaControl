import { useEffect, useState } from "react";
import { BarChart3, CalendarClock, Home, Settings as Cog, Snowflake, WifiOff, X } from "lucide-react";
import { connectLive, dismiss, loadMe, useStore } from "./store";
import { cx, Link, navigate, Spinner, usePath } from "./ui";
import { t } from "./i18n";
import Dashboard from "./pages/Dashboard";
import DevicePage from "./pages/DevicePage";
import HistoryPage from "./pages/HistoryPage";
import AutomationPage from "./pages/AutomationPage";
import SettingsPage from "./pages/SettingsPage";
import LoginPage from "./pages/LoginPage";

const NAV = [
  { to: "/", label: "Übersicht", icon: Home },
  { to: "/automation", label: "Automatik", icon: CalendarClock },
  { to: "/history", label: "Verlauf", icon: BarChart3 },
  { to: "/settings", label: "Einstellungen", icon: Cog },
];

export default function App() {
  const me = useStore((s) => s.me);
  const connected = useStore((s) => s.connected);
  const notices = useStore((s) => s.notices);
  const path = usePath();
  const [booted, setBooted] = useState(false);

  useEffect(() => {
    loadMe().then((m) => {
      setBooted(true);
      if (m && m.role !== "none") connectLive();
      else navigate("/login", true);
    });
  }, []);

  if (!booted) return <Spinner />;

  if (path === "/login" || !me || me.role === "none") {
    return <LoginPage />;
  }

  const seg = path.split("/").filter(Boolean);
  let page;
  switch (seg[0]) {
    case "device":
      page = <DevicePage id={seg[1]} tab={seg[2]} />;
      break;
    case "automation":
      page = <AutomationPage tab={seg[1]} />;
      break;
    case "history":
      page = <HistoryPage id={seg[1]} />;
      break;
    case "settings":
      page = <SettingsPage section={seg[1]} />;
      break;
    default:
      page = <Dashboard />;
  }

  const active = (to: string) => (to === "/" ? path === "/" || seg[0] === "device" : path.startsWith(to));

  return (
    <div className="min-h-dvh pb-24 sm:pb-8">
      <header className="sticky top-0 z-30 bg-bg/85 backdrop-blur border-b border-line">
        <div className="max-w-5xl mx-auto flex items-center gap-3 px-4 h-14">
          <Link to="/" className="flex items-center gap-2 font-semibold tracking-tight text-[17px]">
            <span className="grid place-items-center w-8 h-8 rounded-xl bg-accent text-on-accent">
              <Snowflake size={18} />
            </span>
            PortaControl
          </Link>
          <nav className="hidden sm:flex items-center gap-1 ml-6">
            {NAV.map((n) => (
              <Link key={n.to} to={n.to} className={cx("px-3 h-9 rounded-xl flex items-center gap-2 text-[14px] font-medium", active(n.to) ? "bg-surface2 text-text" : "text-muted hover:text-text")}>
                <n.icon size={16} />
                {t(n.label)}
              </Link>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-2 text-[12px] text-muted">
            {!connected && (
              <span className="flex items-center gap-1 text-warn">
                <WifiOff size={14} /> live …
              </span>
            )}
            {me.role === "guest" && (
              <Link to="/login" className="underline">
                {t("Anmelden")}
              </Link>
            )}
          </div>
        </div>
      </header>

      <main className="max-w-5xl mx-auto px-4 pt-4">{page}</main>

      <nav className="sm:hidden fixed bottom-0 inset-x-0 z-30 bg-surface/95 backdrop-blur border-t border-line pb-[env(safe-area-inset-bottom)]">
        <div className="grid grid-cols-4">
          {NAV.map((n) => (
            <Link key={n.to} to={n.to} className={cx("flex flex-col items-center gap-0.5 py-2 text-[11px] font-medium", active(n.to) ? "text-accent" : "text-muted")}>
              <n.icon size={20} />
              {t(n.label)}
            </Link>
          ))}
        </div>
      </nav>

      <div className="fixed z-50 bottom-20 sm:bottom-6 right-4 left-4 sm:left-auto sm:w-96 flex flex-col gap-2">
        {notices.map((n) => (
          <div key={n.id} className={cx("rounded-2xl border px-4 py-3 shadow-lg bg-surface flex gap-3", n.error ? "border-danger/50" : "border-line")}>
            <div className="flex-1 min-w-0">
              <div className={cx("text-[14px] font-semibold", n.error && "text-danger")}>{n.title}</div>
              {n.text && <div className="text-[13px] text-muted break-words">{n.text}</div>}
            </div>
            <button onClick={() => dismiss(n.id)} className="text-muted self-start" aria-label={t("Schließen")}>
              <X size={16} />
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}
