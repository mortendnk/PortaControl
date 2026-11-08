// German is the source language; English via lookup. The language follows
// the setting (or the browser when "auto").

const en: Record<string, string> = {
  "Übersicht": "Overview",
  "Steuerung": "Control",
  "Aktiv": "Active",
  "Automatik": "Automation",
  "Verlauf": "History",
  "Einstellungen": "Settings",
  "Anlagen": "Devices",
  "Anlage": "Device",
  "Ein": "On",
  "Aus": "Off",
  "Kühlen": "Cool",
  "Heizen": "Heat",
  "Entfeuchten": "Dry",
  "Lüften": "Fan",
  "Auto": "Auto",
  "Modus": "Mode",
  "Lüfter": "Fan",
  "Leise": "Silent",
  "Niedrig": "Low",
  "Mittel": "Medium",
  "Hoch": "High",
  "Max": "Max",
  "Komfort": "Comfort",
  "Weitere Funktionen": "More functions",
  "Schwenken": "Swing",
  "Eco": "Eco",
  "Turbo": "Turbo",
  "Schlafmodus": "Sleep",
  "Außen leise": "Quiet outdoor",
  "iECO": "iECO",
  "Ionisierer": "Ionizer",
  "Frostschutz 8 °C": "Frost protection 8 °C",
  "Selbstreinigung": "Self clean",
  "Anzeige": "Display",
  "Signalton": "Beep",
  "Leistungsbegrenzung": "Power limit",
  "Keine": "None",
  "Innen": "Indoor",
  "Außen": "Outdoor",
  "Leistung": "Power",
  "Heute": "Today",
  "Gesamt": "Total",
  "Soll": "Target",
  "Timer": "Timers",
  "Einschalten um": "Switch on at",
  "Ausschalten in": "Switch off in",
  "Gerätetimer": "Device timers",
  "Szenen": "Scenes",
  "Wochenprogramm": "Weekly program",
  "Regeln": "Rules",
  "Diagnose": "Diagnostics",
  "Protokoll": "Log",
  "Speichern": "Save",
  "Abbrechen": "Cancel",
  "Löschen": "Delete",
  "Bearbeiten": "Edit",
  "Neu": "New",
  "Hinzufügen": "Add",
  "Anwenden": "Apply",
  "Schließen": "Close",
  "Aktualisieren": "Refresh",
  "nicht erreichbar": "unreachable",
  "verbunden": "connected",
  "über WLAN": "via LAN",
  "über Cloud": "via cloud",
  "Filter reinigen": "Clean the filter",
  "Fehlercode": "Error code",
  "Anmelden": "Log in",
  "Abmelden": "Log out",
  "Passwort": "Password",
  "Allgemein": "General",
  "Benachrichtigungen": "Notifications",
  "Integrationen": "Integrations",
  "Zugriff": "Access",
  "Sicherung": "Backup",
  "System": "System",
  "Strompreis": "Electricity price",
  "Währung": "Currency",
  "Anlage hinzufügen": "Add device",
  "Suchen": "Search",
  "Name": "Name",
  "Raum": "Room",
  "Adresse": "Address",
  "Verbindung": "Connection",
  "Automatisch (WLAN zuerst)": "Automatic (LAN first)",
  "Nur WLAN": "LAN only",
  "Nur Cloud": "Cloud only",
  "Tag": "Day",
  "Woche": "Week",
  "Monat": "Month",
  "Jahr": "Year",
  "Verbrauch": "Consumption",
  "Kosten": "Cost",
  "Laufzeit": "Runtime",
  "Temperaturen": "Temperatures",
  "Keine Daten": "No data",
  "CSV exportieren": "Export CSV",
  "Mo": "Mo",
  "Di": "Tu",
  "Mi": "We",
  "Do": "Th",
  "Fr": "Fr",
  "Sa": "Sa",
  "So": "Su",
  "täglich": "daily",
  "Aktion": "Action",
  "Einschalten": "Switch on",
  "Ausschalten": "Switch off",
  "Szene anwenden": "Apply scene",
  "Einstellen": "Set",
  "Benachrichtigen": "Notify",
  "Alle Anlagen": "All devices",
  "Uhrzeit": "Time",
  "Tage": "Days",
  "aktiv": "active",
  "Nächster Lauf": "Next run",
  "Einmal überspringen": "Skip once",
  "Jetzt ausführen": "Run now",
  "Wenn": "When",
  "über": "above",
  "unter": "below",
  "Innentemperatur": "Indoor temperature",
  "Außentemperatur": "Outdoor temperature",
  "Leistungsaufnahme": "Power draw",
  "Dann": "Then",
  "Zeitfenster": "Time window",
  "Pause danach (min)": "Cooldown (min)",
  "nur wenn aus": "only if off",
  "nur wenn an": "only if on",
  "Sprache": "Language",
  "Design": "Theme",
  "Hell": "Light",
  "Dunkel": "Dark",
  "System-Standard": "System default",
  "Android-App verbinden": "Connect Android app",
  "Aktualisierung verfügbar": "Update available",
  "Abfrageintervall (s)": "Poll interval (s)",
  "Gerät trennen nach (s)": "Disconnect after (s)",
  "Steuern ohne Anmeldung im Heimnetz": "Control without login on the home network",
  "Testnachricht senden": "Send test message",
  "erledigt": "done",
  "Fehler": "Error",
  "Laden …": "Loading …",
  "Noch keine Anlage eingerichtet.": "No device set up yet.",
  "Jetzt einrichten": "Set up now",
  "Wird gesendet …": "Sending …",
  "Selbstreinigung starten? Sie dauert etwa 15 Minuten, die Anlage ist in dieser Zeit nicht nutzbar.":
    "Start self cleaning? It takes about 15 minutes; the device cannot be used meanwhile.",
};

let lang: "de" | "en" = "de";

export function setLanguage(l: string) {
  if (l === "auto" || !l) {
    lang = navigator.language?.toLowerCase().startsWith("de") ? "de" : "en";
  } else {
    lang = l === "en" ? "en" : "de";
  }
  document.documentElement.lang = lang;
}

export function language() {
  return lang;
}

export function t(s: string): string {
  return lang === "en" ? (en[s] ?? s) : s;
}

const locale = () => (lang === "de" ? "de-DE" : "en-GB");

export function fmtNum(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined || Number.isNaN(v)) return "–";
  return v.toLocaleString(locale(), { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

export function fmtTemp(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined) return "–";
  return fmtNum(v, digits) + " °C";
}

export function fmtMoney(v: number | null | undefined, currency = "EUR"): string {
  if (v === null || v === undefined) return "–";
  try {
    return v.toLocaleString(locale(), { style: "currency", currency });
  } catch {
    return fmtNum(v, 2) + " " + currency;
  }
}

export function fmtTime(iso: string | number | Date | null | undefined): string {
  if (!iso) return "–";
  const d = new Date(iso);
  return d.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit" });
}

export function fmtDateTime(iso: string | number | Date | null | undefined): string {
  if (!iso) return "–";
  const d = new Date(iso);
  const today = new Date();
  const sameDay = d.toDateString() === today.toDateString();
  const tomorrow = new Date(today.getTime() + 86400000).toDateString() === d.toDateString();
  const time = fmtTime(d);
  if (sameDay) return (lang === "de" ? "heute " : "today ") + time;
  if (tomorrow) return (lang === "de" ? "morgen " : "tomorrow ") + time;
  return d.toLocaleDateString(locale(), { weekday: "short", day: "2-digit", month: "2-digit" }) + " " + time;
}

export function fmtAgo(iso: string | null | undefined): string {
  if (!iso) return "–";
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return lang === "de" ? `vor ${s} s` : `${s} s ago`;
  const m = Math.round(s / 60);
  if (m < 60) return lang === "de" ? `vor ${m} min` : `${m} min ago`;
  return fmtDateTime(iso);
}

export const MODES: { id: "cool" | "heat" | "dry" | "fan" | "auto"; label: string; color: string }[] = [
  { id: "cool", label: "Kühlen", color: "var(--cool)" },
  { id: "heat", label: "Heizen", color: "var(--heat)" },
  { id: "dry", label: "Entfeuchten", color: "var(--dry)" },
  { id: "fan", label: "Lüften", color: "var(--fan)" },
  { id: "auto", label: "Auto", color: "var(--auto)" },
];

export function modeColor(m?: string) {
  return MODES.find((x) => x.id === m)?.color ?? "var(--muted)";
}

export function modeLabel(m?: string) {
  return t(MODES.find((x) => x.id === m)?.label ?? "Auto");
}

export const FAN_PRESETS = [
  { v: 20, label: "Leise" },
  { v: 40, label: "Niedrig" },
  { v: 60, label: "Mittel" },
  { v: 80, label: "Hoch" },
  { v: 100, label: "Max" },
];

export function fanLabel(v: number) {
  if (v === 102) return t("Auto");
  const p = FAN_PRESETS.find((x) => x.v === v);
  return p ? t(p.label) : `${v} %`;
}

export const WEEKDAYS = [
  { d: 1, l: "Mo" },
  { d: 2, l: "Di" },
  { d: 3, l: "Mi" },
  { d: 4, l: "Do" },
  { d: 5, l: "Fr" },
  { d: 6, l: "Sa" },
  { d: 0, l: "So" },
];
