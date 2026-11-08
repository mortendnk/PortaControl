import { useEffect, useMemo, useRef, useState } from "react";
import qrcode from "qrcode-generator";
import {
  Bell,
  Cloud,
  Cpu,
  Database,
  KeyRound,
  ListTree,
  Plug,
  Plus,
  RefreshCw,
  Search,
  Smartphone,
  SlidersHorizontal,
  Trash2,
  Wind,
} from "lucide-react";
import { api, type CloudAppliance, type Device, type Discovered, type LogEntry, type Settings } from "../api";
import { loadMe, sortedDevices, toast, useStore } from "../store";
import { Badge, Button, Card, Empty, Field, Modal, Row, Segmented, Spinner, Toggle, cx, navigate } from "../ui";
import { fmtDateTime, setLanguage, t } from "../i18n";

const SECTIONS = [
  { id: "devices", label: "Anlagen", icon: Wind },
  { id: "general", label: "Allgemein", icon: SlidersHorizontal },
  { id: "notify", label: "Benachrichtigungen", icon: Bell },
  { id: "integrations", label: "Integrationen", icon: Plug },
  { id: "access", label: "Zugriff", icon: KeyRound },
  { id: "backup", label: "Sicherung", icon: Database },
  { id: "log", label: "Protokoll", icon: ListTree },
  { id: "system", label: "System", icon: Cpu },
];

// ---- Add device wizard -------------------------------------------------------

function AddDevice({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [step, setStep] = useState<"start" | "found" | "cloud" | "manual">("start");
  const [found, setFound] = useState<Discovered[] | null>(null);
  const [target, setTarget] = useState("");
  const [cloudStatus, setCloudStatus] = useState<{ account: string; configured: boolean } | null>(null);
  const [account, setAccount] = useState("");
  const [password, setPassword] = useState("");
  const [appliances, setAppliances] = useState<CloudAppliance[] | null>(null);
  const [pick, setPick] = useState<Discovered | null>(null);
  const [name, setName] = useState("");
  const [room, setRoom] = useState("");
  const [manual, setManual] = useState({ ip: "", port: 6444, device_id: "", token: "", key: "" });
  const [err, setErr] = useState("");

  useEffect(() => {
    if (!open) return;
    setStep("start");
    setFound(null);
    setErr("");
    setPick(null);
    api("/setup/cloud").then(setCloudStatus).catch(() => {});
  }, [open]);

  const search = async (subnet = false) => {
    setErr("");
    setFound(null);
    setStep("found");
    try {
      setFound(await api<Discovered[]>("/setup/discover", { body: { target: target.trim(), subnet } }));
    } catch (e: any) {
      setErr(e.message);
      setFound([]);
    }
  };

  const login = async () => {
    setErr("");
    try {
      const r = await api<{ appliances: CloudAppliance[] }>("/setup/cloud/login", { body: { account, password } });
      setAppliances(r.appliances);
      setCloudStatus({ account, configured: true });
    } catch (e: any) {
      setErr(e.message);
    }
  };

  const add = async (body: any) => {
    setErr("");
    try {
      const d = await api<Device>("/devices", { body });
      toast(d.name, "eingerichtet");
      onClose();
      navigate(`/device/${d.id}`);
    } catch (e: any) {
      setErr(e.message);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title={t("Anlage hinzufügen")} wide>
      {step === "start" && (
        <div className="flex flex-col gap-4">
          <p className="text-[14px] text-muted">
            Die Anlage muss im selben Netz wie PortaControl sein (UDP 6445/20086 für die Suche, TCP 6444 für die Steuerung). Token und Schlüssel werden einmalig aus dem Midea-Konto (MSmartHome oder NetHome Plus) geholt; danach läuft alles lokal.
          </p>
          <div className="flex gap-2">
            <input className="flex-1" placeholder="IP-Adresse (optional, sonst ganzes Netz)" value={target} onChange={(e) => setTarget(e.target.value)} />
            <Button variant="primary" onClick={() => search(false)}>
              <Search size={16} /> {t("Suchen")}
            </Button>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => search(true)}>Netz-Scan (jede Adresse)</Button>
            <Button onClick={() => setStep("manual")}>Token/Schlüssel manuell</Button>
            <Button onClick={() => setStep("cloud")}>
              <Cloud size={15} /> Nur über Cloud
            </Button>
          </div>
        </div>
      )}

      {step === "found" && (
        <div className="flex flex-col gap-3">
          {found === null && <Spinner text="Suche im Netz (4 s) …" />}
          {found && found.length === 0 && (
            <Empty>
              Keine Anlage gefunden. Ist die Anlage eingeschaltet und im WLAN? Im LXC muss das Netz gebrückt sein (vmbr0). Alternativ die IP eingeben oder den Netz-Scan probieren.
            </Empty>
          )}
          {found?.map((d) => (
            <button
              key={d.device_id + d.ip}
              onClick={() => {
                setPick(d);
                setName(d.name.startsWith("net_") ? "PortaSplit" : d.name);
              }}
              className={cx("text-left rounded-2xl border px-4 py-3 flex items-center gap-3", pick?.device_id === d.device_id ? "border-accent bg-accent-soft" : "border-line")}
            >
              <Wind size={20} className="text-accent" />
              <div className="flex-1 min-w-0">
                <div className="font-medium">{d.name}</div>
                <div className="text-[12px] text-muted">
                  {d.ip} · ID {d.device_id} · V{d.version} {!d.air_conditioner && "· kein Klimagerät?"}
                </div>
              </div>
              {d.known && <Badge tone="ok">eingerichtet</Badge>}
            </button>
          ))}

          {pick && (
            <div className="rounded-2xl border border-line p-4 flex flex-col gap-3 mt-2">
              <div className="grid grid-cols-2 gap-2">
                <Field label={t("Name")}>
                  <input value={name} onChange={(e) => setName(e.target.value)} />
                </Field>
                <Field label={t("Raum")}>
                  <input value={room} onChange={(e) => setRoom(e.target.value)} placeholder="Schlafzimmer" />
                </Field>
              </div>
              {cloudStatus?.configured ? (
                <p className="text-[13px] text-muted">Midea-Konto: {cloudStatus.account}</p>
              ) : (
                <>
                  <p className="text-[13px] text-muted">Midea-Konto (MSmartHome/NetHome Plus) für den einmaligen Abruf von Token und Schlüssel:</p>
                  <div className="grid grid-cols-2 gap-2">
                    <input placeholder="E-Mail" value={account} onChange={(e) => setAccount(e.target.value)} autoComplete="username" />
                    <input type="password" placeholder={t("Passwort")} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
                  </div>
                  <Button onClick={login}>Konto prüfen</Button>
                </>
              )}
              <Button
                variant="primary"
                disabled={!cloudStatus?.configured}
                onClick={() => add({ from_cloud: true, ip: pick.ip, port: pick.port, device_id: pick.device_id, sn: pick.sn, name, room })}
              >
                Token holen und einrichten
              </Button>
              <Button onClick={() => {
                setManual({ ...manual, ip: pick.ip, port: pick.port, device_id: pick.device_id });
                setStep("manual");
              }}>
                Stattdessen Token/Schlüssel eingeben
              </Button>
            </div>
          )}
          <Button variant="ghost" onClick={() => setStep("start")}>
            Zurück
          </Button>
        </div>
      )}

      {step === "manual" && (
        <div className="flex flex-col gap-3">
          <p className="text-[13px] text-muted">
            Token (128 Hex-Zeichen) und Schlüssel (64 Hex-Zeichen) z. B. aus einer Klimacontrol-Sicherung, msmart-ng (<code>msmart-ng discover</code>) oder <code>portacontrol-cli token</code>.
          </p>
          <div className="grid grid-cols-3 gap-2">
            <Field label="IP">
              <input value={manual.ip} onChange={(e) => setManual({ ...manual, ip: e.target.value })} />
            </Field>
            <Field label="Port">
              <input type="number" value={manual.port} onChange={(e) => setManual({ ...manual, port: Number(e.target.value) })} />
            </Field>
            <Field label="Geräte-ID">
              <input value={manual.device_id} onChange={(e) => setManual({ ...manual, device_id: e.target.value })} />
            </Field>
          </div>
          <Field label="Token">
            <textarea rows={2} className="font-mono text-[12px]" value={manual.token} onChange={(e) => setManual({ ...manual, token: e.target.value.trim() })} />
          </Field>
          <Field label="Key">
            <input className="font-mono text-[12px]" value={manual.key} onChange={(e) => setManual({ ...manual, key: e.target.value.trim() })} />
          </Field>
          <div className="grid grid-cols-2 gap-2">
            <Field label={t("Name")}>
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder="PortaSplit" />
            </Field>
            <Field label={t("Raum")}>
              <input value={room} onChange={(e) => setRoom(e.target.value)} />
            </Field>
          </div>
          <div className="flex gap-2 justify-end">
            <Button onClick={() => setStep("start")}>Zurück</Button>
            <Button variant="primary" onClick={() => add({ ...manual, name, room })}>
              Prüfen und einrichten
            </Button>
          </div>
        </div>
      )}

      {step === "cloud" && (
        <div className="flex flex-col gap-3">
          <p className="text-[13px] text-muted">
            Ohne lokale Verbindung, nur über die Midea-Cloud (z. B. Anlage in einem anderen Netz). Langsamer, braucht Internet.
          </p>
          {!cloudStatus?.configured || !appliances ? (
            <>
              <div className="grid grid-cols-2 gap-2">
                <input placeholder="E-Mail" value={account} onChange={(e) => setAccount(e.target.value)} />
                <input type="password" placeholder={t("Passwort")} value={password} onChange={(e) => setPassword(e.target.value)} />
              </div>
              <Button variant="primary" onClick={login}>
                Anmelden und Anlagen laden
              </Button>
            </>
          ) : null}
          {appliances?.map((a) => (
            <div key={a.device_id} className="rounded-2xl border border-line px-4 py-3 flex items-center gap-3">
              <div className="flex-1">
                <div className="font-medium">{a.name}</div>
                <div className="text-[12px] text-muted">
                  ID {a.device_id} · {a.online ? "online" : "offline"}
                </div>
              </div>
              <Button onClick={() => add({ cloud_only: true, device_id: a.device_id, name: a.name, room })}>{t("Hinzufügen")}</Button>
            </div>
          ))}
          <Button variant="ghost" onClick={() => setStep("start")}>
            Zurück
          </Button>
        </div>
      )}

      {err && <div className="mt-3 text-danger text-[13px] rounded-xl bg-danger/10 p-3">{err}</div>}
    </Modal>
  );
}

function DeviceEditor({ d, onClose }: { d: Device; onClose: () => void }) {
  const [v, setV] = useState({ name: d.name, room: d.room, ip: d.ip, port: d.port, connection: d.connection, min_target: d.min_target ?? 0, max_target: d.max_target ?? 0, locked: !!d.locked });
  const [creds, setCreds] = useState<{ token: string; key: string } | null>(null);
  const [err, setErr] = useState("");

  const save = async () => {
    setErr("");
    try {
      await api(`/devices/${d.id}`, { method: "PATCH", body: v });
      onClose();
    } catch (e: any) {
      setErr(e.message);
    }
  };

  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-2 gap-2">
        <Field label={t("Name")}>
          <input value={v.name} onChange={(e) => setV({ ...v, name: e.target.value })} />
        </Field>
        <Field label={t("Raum")}>
          <input value={v.room} onChange={(e) => setV({ ...v, room: e.target.value })} />
        </Field>
        <Field label={t("Adresse")}>
          <input value={v.ip} onChange={(e) => setV({ ...v, ip: e.target.value })} />
        </Field>
        <Field label="Port">
          <input type="number" value={v.port} onChange={(e) => setV({ ...v, port: Number(e.target.value) })} />
        </Field>
      </div>
      <Field label={t("Verbindung")} hint={!d.has_cloud ? "Cloud-Rückfall braucht ein Midea-Konto (beim Hinzufügen anmelden)." : undefined}>
        <Segmented
          value={v.connection}
          onChange={(c) => setV({ ...v, connection: c })}
          options={[
            { value: "auto", label: t("Automatisch (WLAN zuerst)") },
            { value: "lan", label: t("Nur WLAN") },
            { value: "cloud", label: t("Nur Cloud") },
          ]}
        />
      </Field>
      <div className="grid grid-cols-2 gap-2">
        <Field label="Min. Soll (Kindersicherung)" hint="0 = ohne">
          <input type="number" min={0} max={30} value={v.min_target} onChange={(e) => setV({ ...v, min_target: Number(e.target.value) })} />
        </Field>
        <Field label="Max. Soll" hint="0 = ohne">
          <input type="number" min={0} max={30} value={v.max_target} onChange={(e) => setV({ ...v, max_target: Number(e.target.value) })} />
        </Field>
      </div>
      <Row label="Für Gäste sperren" hint="Ohne Admin-Anmeldung nur anzeigen, nicht steuern.">
        <Toggle on={v.locked} onChange={(x) => setV({ ...v, locked: x })} />
      </Row>
      <div className="flex flex-wrap gap-2">
        <Button
          onClick={async () => {
            try {
              await api(`/devices/${d.id}/relocate`, { method: "POST" });
              toast(d.name, "Adresse aktualisiert");
              onClose();
            } catch (e: any) {
              setErr(e.message);
            }
          }}
        >
          <RefreshCw size={15} /> Adresse neu suchen
        </Button>
        <Button onClick={async () => setCreds(await api(`/devices/${d.id}/credentials`))}>
          <KeyRound size={15} /> Token/Schlüssel zeigen
        </Button>
        <Button
          variant="danger"
          onClick={async () => {
            if (!confirm(`${d.name} entfernen? Der Verlauf bleibt erhalten, falls die Anlage erneut hinzugefügt wird.`)) return;
            await api(`/devices/${d.id}`, { method: "DELETE" });
            onClose();
          }}
        >
          <Trash2 size={15} /> {t("Löschen")}
        </Button>
      </div>
      {creds && (
        <pre className="text-[11px] font-mono break-all whitespace-pre-wrap bg-surface2 rounded-xl p-3">
          token: {creds.token || "–"}
          {"\n"}key: {creds.key || "–"}
        </pre>
      )}
      {err && <div className="text-danger text-[13px]">{err}</div>}
      <div className="flex gap-2 justify-end">
        <Button onClick={onClose}>{t("Abbrechen")}</Button>
        <Button variant="primary" onClick={save}>
          {t("Speichern")}
        </Button>
      </div>
    </div>
  );
}

function DevicesSection() {
  const devices = useStore((s) => s.devices);
  const list = useMemo(() => sortedDevices(devices), [devices]);
  const [adding, setAdding] = useState(location.search.includes("add=1"));
  const [edit, setEdit] = useState<Device | null>(null);
  const [cloud, setCloud] = useState<{ account: string; configured: boolean } | null>(null);

  useEffect(() => {
    api("/setup/cloud").then(setCloud).catch(() => {});
  }, [adding]);

  return (
    <div className="flex flex-col gap-4">
      <Card title={t("Anlagen")} action={<Button variant="primary" onClick={() => setAdding(true)}><Plus size={16} /> {t("Hinzufügen")}</Button>}>
        {list.length === 0 && <Empty>{t("Noch keine Anlage eingerichtet.")}</Empty>}
        {list.map((d) => (
          <div key={d.id} className="flex items-center gap-3 py-3 border-b border-line last:border-0">
            <Wind size={18} className={d.online ? "text-accent" : "text-muted"} />
            <div className="flex-1 min-w-0">
              <div className="font-medium flex items-center gap-2">
                {d.name} {d.simulated && <Badge>Demo</Badge>}
              </div>
              <div className="text-[12px] text-muted">
                {d.ip || "Cloud"} · ID {d.id} · {d.connection} · {d.online ? `${t("verbunden")} (${d.path})` : t("nicht erreichbar")}
              </div>
            </div>
            <Button variant="ghost" onClick={() => setEdit(d)}>
              {t("Bearbeiten")}
            </Button>
          </div>
        ))}
      </Card>
      <Card title="Midea-Konto">
        {cloud?.configured ? (
          <Row label={cloud.account} hint="Für Token-Abruf, Cloud-Rückfall unterwegs und Anlagen ohne lokale Verbindung.">
            <Button
              onClick={async () => {
                await api("/setup/cloud/logout", { method: "POST" });
                setCloud({ account: "", configured: false });
              }}
            >
              {t("Abmelden")}
            </Button>
          </Row>
        ) : (
          <p className="text-[13px] text-muted">Nicht angemeldet. Die Anmeldung erfolgt beim Hinzufügen einer Anlage; nötig nur für den Token-Abruf und Cloud-Zugriff.</p>
        )}
      </Card>
      <AddDevice open={adding} onClose={() => setAdding(false)} />
      <Modal open={!!edit} onClose={() => setEdit(null)} title={edit?.name ?? ""} wide>
        {edit && <DeviceEditor d={edit} onClose={() => setEdit(null)} />}
      </Modal>
    </div>
  );
}

// ---- settings sections ------------------------------------------------------------

function useSettings() {
  const ver = useStore((s) => s.versions.settings ?? 0);
  const [data, setData] = useState<{ settings: Settings; mqtt_password_set: boolean; mqtt_status: string } | null>(null);
  useEffect(() => {
    api("/settings").then(setData).catch(() => {});
  }, [ver]);
  const save = async (s: Settings) => {
    try {
      setData(await api("/settings", { method: "PUT", body: s }));
      toast(t("Speichern"), t("erledigt"));
    } catch (e: any) {
      toast(t("Fehler"), e.message, true);
    }
  };
  return { data, save, setData };
}

function GeneralSection() {
  const { data, save } = useSettings();
  const [s, setS] = useState<Settings | null>(null);
  const [theme, setTheme] = useState(localStorage.getItem("pc.theme") ?? "auto");
  const [lang, setLang] = useState(localStorage.getItem("pc.lang") ?? "auto");
  useEffect(() => setS(data?.settings ?? null), [data]);
  if (!s) return <Spinner />;
  return (
    <div className="flex flex-col gap-4">
      <Card title={t("Allgemein")}>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
          <Field label={`${t("Strompreis")} / kWh`}>
            <input type="number" step="0.01" value={s.price_per_kwh} onChange={(e) => setS({ ...s, price_per_kwh: Number(e.target.value) })} />
          </Field>
          <Field label={t("Währung")}>
            <input value={s.currency} onChange={(e) => setS({ ...s, currency: e.target.value.toUpperCase().slice(0, 3) })} />
          </Field>
          <Field label="CO₂ g/kWh" hint="Strommix (DE ≈ 380)">
            <input type="number" value={s.co2_grams_per_kwh} onChange={(e) => setS({ ...s, co2_grams_per_kwh: Number(e.target.value) })} />
          </Field>
          <Field label={t("Abfrageintervall (s)")} hint="ohne offene Oberfläche">
            <input type="number" min={5} value={s.poll_seconds} onChange={(e) => setS({ ...s, poll_seconds: Number(e.target.value) })} />
          </Field>
          <Field label="Intervall live (s)" hint="solange jemand zuschaut">
            <input type="number" min={2} value={s.fast_poll_seconds} onChange={(e) => setS({ ...s, fast_poll_seconds: Number(e.target.value) })} />
          </Field>
          <Field label={t("Gerät trennen nach (s)")} hint="gibt die Anlage für die Midea-App frei">
            <input type="number" min={2} value={s.idle_disconnect_seconds} onChange={(e) => setS({ ...s, idle_disconnect_seconds: Number(e.target.value) })} />
          </Field>
          <Field label="Verlauf speichern (Tage)">
            <input type="number" min={7} value={s.history_days} onChange={(e) => setS({ ...s, history_days: Number(e.target.value) })} />
          </Field>
          <Field label="Messpunkt alle (s)">
            <input type="number" min={10} value={s.sample_seconds} onChange={(e) => setS({ ...s, sample_seconds: Number(e.target.value) })} />
          </Field>
        </div>
        <Row label={t("Signalton")} hint="Piepton der Anlage bei Befehlen">
          <Toggle on={s.beep} onChange={(v) => setS({ ...s, beep: v })} />
        </Row>
        <div className="flex justify-end mt-3">
          <Button variant="primary" onClick={() => save(s)}>
            {t("Speichern")}
          </Button>
        </div>
      </Card>
      <Card title="Darstellung (dieses Gerät)">
        <Row label={t("Design")}>
          <Segmented
            value={theme}
            onChange={(v) => {
              setTheme(v);
              localStorage.setItem("pc.theme", v);
              if (v === "auto") delete document.documentElement.dataset.theme;
              else document.documentElement.dataset.theme = v;
            }}
            options={[
              { value: "auto", label: "Auto" },
              { value: "light", label: t("Hell") },
              { value: "dark", label: t("Dunkel") },
            ]}
          />
        </Row>
        <Row label={t("Sprache")}>
          <Segmented
            value={lang}
            onChange={(v) => {
              setLang(v);
              localStorage.setItem("pc.lang", v);
              setLanguage(v);
              location.reload();
            }}
            options={[
              { value: "auto", label: "Auto" },
              { value: "de", label: "Deutsch" },
              { value: "en", label: "English" },
            ]}
          />
        </Row>
      </Card>
    </div>
  );
}

function NotifySection() {
  const { data, save } = useSettings();
  const [s, setS] = useState<Settings | null>(null);
  useEffect(() => setS(data?.settings ?? null), [data]);
  if (!s) return <Spinner />;
  const n = s.notify;
  const set = (p: Partial<Settings["notify"]>) => setS({ ...s, notify: { ...n, ...p } });
  return (
    <Card title={t("Benachrichtigungen")}>
      <p className="text-[13px] text-muted mb-3">
        Meldungen erscheinen in der Oberfläche und der App. Für Push aufs Handy ein ntfy-Thema angeben (App „ntfy“, z. B. <code>https://ntfy.sh/mein-geheimes-thema</code> oder ein eigener ntfy-Server).
      </p>
      <Field label="ntfy-URL">
        <input value={n.ntfy_url} onChange={(e) => set({ ntfy_url: e.target.value })} placeholder="https://ntfy.sh/..." />
      </Field>
      <div className="mt-2">
        <Row label="Filter reinigen">
          <Toggle on={n.filter_alert} onChange={(v) => set({ filter_alert: v })} />
        </Row>
        <Row label="Fehlercodes">
          <Toggle on={n.error_code} onChange={(v) => set({ error_code: v })} />
        </Row>
        <Row label="Anlage nicht erreichbar / wieder da">
          <Toggle on={n.offline} onChange={(v) => set({ offline: v })} />
        </Row>
        <Row label="Automatik ausgeführt" hint="Timer, Wochenprogramm, Regeln (Fehler werden immer gemeldet)">
          <Toggle on={n.automation} onChange={(v) => set({ automation: v })} />
        </Row>
        <Row label="Temperatur-Alarm über (°C)" hint="leer = aus">
          <input type="number" className="w-24" value={n.temp_alarm_max ?? ""} onChange={(e) => set({ temp_alarm_max: e.target.value === "" ? null : Number(e.target.value) })} />
        </Row>
        <Row label="Temperatur-Alarm unter (°C)">
          <input type="number" className="w-24" value={n.temp_alarm_min ?? ""} onChange={(e) => set({ temp_alarm_min: e.target.value === "" ? null : Number(e.target.value) })} />
        </Row>
      </div>
      <div className="flex justify-end gap-2 mt-3">
        <Button onClick={() => api("/settings/test-notify", { method: "POST" })}>{t("Testnachricht senden")}</Button>
        <Button variant="primary" onClick={() => save(s)}>
          {t("Speichern")}
        </Button>
      </div>
    </Card>
  );
}

function IntegrationsSection() {
  const { data, save } = useSettings();
  const [s, setS] = useState<Settings | null>(null);
  useEffect(() => setS(data?.settings ?? null), [data]);
  if (!s) return <Spinner />;
  const m = s.mqtt;
  const setM = (p: Partial<Settings["mqtt"]>) => setS({ ...s, mqtt: { ...m, ...p } });
  const hooks = s.webhooks ?? [];
  return (
    <div className="flex flex-col gap-4">
      <Card title="MQTT / Home Assistant" action={<Badge tone={data?.mqtt_status.startsWith("verbunden") ? "ok" : "muted"}>{data?.mqtt_status}</Badge>}>
        <p className="text-[13px] text-muted mb-3">
          Mit Discovery erscheint jede Anlage in Home Assistant als Klimagerät mit Sensoren (Temperaturen, Leistung, Verbrauch), Schaltern (Außen leise, iECO, Ionisierer, Anzeige …), Leistungsbegrenzung und Selbstreinigung. Befehle: <code>{m.prefix}/&lt;id&gt;/set/&lt;feld&gt;</code> oder JSON an <code>{m.prefix}/&lt;id&gt;/set</code>.
        </p>
        <Row label="Aktiv">
          <Toggle on={m.enabled} onChange={(v) => setM({ enabled: v })} />
        </Row>
        <div className="grid grid-cols-2 gap-3 mt-2">
          <Field label="Broker">
            <input value={m.broker} onChange={(e) => setM({ broker: e.target.value })} placeholder="tcp://192.168.1.10:1883" />
          </Field>
          <Field label="Präfix">
            <input value={m.prefix} onChange={(e) => setM({ prefix: e.target.value })} />
          </Field>
          <Field label="Benutzer">
            <input value={m.username} onChange={(e) => setM({ username: e.target.value })} />
          </Field>
          <Field label={t("Passwort")} hint={data?.mqtt_password_set ? "gespeichert – leer lassen zum Behalten" : undefined}>
            <input type="password" value={m.password ?? ""} onChange={(e) => setM({ password: e.target.value })} />
          </Field>
        </div>
        <Row label="Home-Assistant-Discovery">
          <Toggle on={m.discovery} onChange={(v) => setM({ discovery: v })} />
        </Row>
      </Card>
      <Card title="Webhooks" action={<Button onClick={() => setS({ ...s, webhooks: [...hooks, { id: "", url: "", events: [], enabled: true }] })}><Plus size={15} /></Button>}>
        <p className="text-[13px] text-muted mb-3">
          HTTP POST mit JSON <code>{"{event, data, time}"}</code> bei Zustandsänderungen („state“) und Meldungen („notice“).
        </p>
        {hooks.length === 0 && <Empty>Keine Webhooks.</Empty>}
        {hooks.map((w, i) => (
          <div key={i} className="flex items-center gap-2 py-2">
            <Toggle on={w.enabled} onChange={(v) => setS({ ...s, webhooks: hooks.map((x, j) => (j === i ? { ...x, enabled: v } : x)) })} />
            <input className="flex-1" value={w.url} placeholder="https://…" onChange={(e) => setS({ ...s, webhooks: hooks.map((x, j) => (j === i ? { ...x, url: e.target.value } : x)) })} />
            <select
              value={(w.events ?? []).join(",")}
              onChange={(e) => setS({ ...s, webhooks: hooks.map((x, j) => (j === i ? { ...x, events: e.target.value ? e.target.value.split(",") : [] } : x)) })}
            >
              <option value="">alle</option>
              <option value="state">state</option>
              <option value="notice">notice</option>
            </select>
            <Button variant="ghost" onClick={() => setS({ ...s, webhooks: hooks.filter((_, j) => j !== i) })}>
              <Trash2 size={15} />
            </Button>
          </div>
        ))}
      </Card>
      <div className="flex justify-end">
        <Button variant="primary" onClick={() => save(s)}>
          {t("Speichern")}
        </Button>
      </div>
    </div>
  );
}

function QR({ text }: { text: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const q = qrcode(0, "M");
    q.addData(text);
    q.make();
    if (ref.current) ref.current.innerHTML = q.createSvgTag({ cellSize: 5, margin: 2, scalable: true });
  }, [text]);
  return <div ref={ref} className="w-56 h-56 bg-white rounded-xl p-1 [&>svg]:w-full [&>svg]:h-full" />;
}

function AccessSection() {
  const me = useStore((s) => s.me);
  const { data, save } = useSettings();
  const [pw, setPw] = useState("");
  const [link, setLink] = useState<{ url: string; api_key: string; link: string } | null>(null);
  return (
    <div className="flex flex-col gap-4">
      <Card title={t("Android-App verbinden")} action={<Smartphone size={18} className="text-muted" />}>
        <p className="text-[13px] text-muted mb-3">
          PortaControl-App installieren (APK aus den GitHub-Releases), „QR-Code scannen“ wählen. Die App findet den Server im Heimnetz auch selbst (mDNS).
        </p>
        {link ? (
          <div className="flex flex-col sm:flex-row gap-4 items-start">
            <QR text={link.link} />
            <div className="text-[13px] flex flex-col gap-2 min-w-0">
              <div>
                <b>URL</b>
                <div className="font-mono break-all">{link.url}</div>
              </div>
              <div>
                <b>API-Key</b>
                <div className="font-mono break-all">{link.api_key}</div>
              </div>
              <Button
                onClick={async () => {
                  if (!confirm("Neuen API-Key erzeugen? Verbundene Apps müssen neu gekoppelt werden.")) return;
                  setLink(await api("/system/api-key/rotate", { method: "POST" }));
                }}
              >
                Neuen Key erzeugen
              </Button>
            </div>
          </div>
        ) : (
          <Button variant="primary" onClick={async () => setLink(await api("/system/app-link"))}>
            QR-Code anzeigen
          </Button>
        )}
      </Card>
      <Card title={t("Zugriff")}>
        {data && (
          <Row label={t("Steuern ohne Anmeldung im Heimnetz")} hint="Alle im Heimnetz dürfen bedienen; Einstellungen nur mit Passwort. Aus = immer Anmeldung nötig.">
            <Toggle on={data.settings.guest_control} onChange={(v) => save({ ...data.settings, guest_control: v })} />
          </Row>
        )}
        <div className="mt-3 flex flex-col gap-2">
          <div className="text-[14px] font-medium">Administrator-Passwort {me?.password_set ? "ändern" : "festlegen"}</div>
          {!me?.password_set && <div className="text-[13px] text-warn">Noch kein Passwort: alle im Heimnetz haben Administratorrechte.</div>}
          <div className="flex gap-2">
            <input type="password" className="flex-1" value={pw} onChange={(e) => setPw(e.target.value)} placeholder="mind. 6 Zeichen" autoComplete="new-password" />
            <Button
              variant="primary"
              disabled={pw.length < 6}
              onClick={async () => {
                try {
                  await api("/auth/password", { body: { password: pw } });
                  setPw("");
                  await loadMe();
                  toast(t("Passwort"), t("erledigt"));
                } catch (e: any) {
                  toast(t("Fehler"), e.message, true);
                }
              }}
            >
              {t("Speichern")}
            </Button>
          </div>
          <Button
            variant="ghost"
            onClick={async () => {
              await api("/auth/logout", { method: "POST" });
              await loadMe();
              navigate("/");
            }}
          >
            {t("Abmelden")}
          </Button>
        </div>
      </Card>
    </div>
  );
}

function BackupSection() {
  const fileRef = useRef<HTMLInputElement>(null);
  return (
    <Card title={t("Sicherung")}>
      <p className="text-[13px] text-muted mb-3">
        Enthält Anlagen mit Token/Schlüssel, Szenen, Wochenprogramm, Regeln und Einstellungen (ohne Verlauf). Das Format ist kompatibel mit Klimacontrol – eine Klimacontrol-Sicherung („Zugangsdaten sichern …“) kann hier eingelesen werden. Die Datei gehört nicht in einen Cloud-Ordner.
      </p>
      <div className="flex flex-wrap gap-2">
        <a href="/api/v1/backup">
          <Button variant="primary">Sicherung herunterladen</Button>
        </a>
        <Button onClick={() => fileRef.current?.click()}>Sicherung einlesen</Button>
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          onChange={async (e) => {
            const f = e.target.files?.[0];
            if (!f) return;
            try {
              const r = await api<{ devices: number; scenes: number }>("/setup/import", { body: await f.text(), raw: true });
              toast("Import", `${r.devices} Anlage(n), ${r.scenes} Szene(n)`);
            } catch (err: any) {
              toast(t("Fehler"), err.message, true);
            }
            e.target.value = "";
          }}
        />
      </div>
    </Card>
  );
}

function LogSection() {
  const ver = useStore((s) => s.versions.log ?? 0);
  const devices = useStore((s) => s.devices);
  const [list, setList] = useState<LogEntry[] | null>(null);
  useEffect(() => {
    api<LogEntry[]>("/log?limit=300").then(setList).catch(() => setList([]));
  }, [ver]);
  if (!list) return <Spinner />;
  return (
    <Card title={t("Protokoll")}>
      {list.length === 0 && <Empty>–</Empty>}
      <div className="flex flex-col">
        {list.map((e) => (
          <div key={e.id} className="flex gap-3 py-1.5 text-[13px] border-b border-line last:border-0">
            <span className="text-muted tabular shrink-0 w-[92px]">{fmtDateTime(e.time)}</span>
            <span className={cx("min-w-0 break-words", e.level === "error" ? "text-danger" : e.level === "warn" ? "text-warn" : "")}>
              {e.device && devices[e.device] && !e.text.startsWith(devices[e.device].name) ? `${devices[e.device].name}: ` : ""}
              {e.text}
            </span>
          </div>
        ))}
      </div>
    </Card>
  );
}

function SystemSection() {
  const [info, setInfo] = useState<any>(null);
  const [upd, setUpd] = useState<any>(null);
  useEffect(() => {
    api("/system").then(setInfo).catch(() => {});
    api("/system/update").then(setUpd).catch(() => {});
  }, []);
  if (!info) return <Spinner />;
  return (
    <Card title={t("System")}>
      <Row label="Version">
        {info.version} <span className="text-muted text-[12px]">({info.commit})</span>
      </Row>
      {upd?.available && (
        <Row label={t("Aktualisierung verfügbar")} hint="Im Container: portacontrol-cli update – oder das Installationsskript erneut ausführen.">
          <a className="text-accent underline" href={upd.url} target="_blank" rel="noreferrer">
            {upd.latest}
          </a>
        </Row>
      )}
      <Row label="Adresse">{info.public_url}</Row>
      <Row label="Zeitzone">{info.time_zone}</Row>
      <Row label="MQTT">{info.mqtt}</Row>
      <Row label="Laufzeit">{Math.round(info.uptime_seconds / 3600)} h</Row>
      {info.simulate && <Row label="Demo-Modus">Simulierte PortaSplit aktiv</Row>}
      <div className="text-[12px] text-muted mt-4">
        PortaControl ist ein privates Projekt und kein Produkt von Midea. Midea, MSmartHome, NetHome Plus und PortaSplit sind Marken ihrer jeweiligen Inhaber. Protokoll nach Klimacontrol, msmart-ng, midea-beautiful-air und midea_ac_lan.
      </div>
    </Card>
  );
}

export default function SettingsPage({ section }: { section?: string }) {
  const me = useStore((s) => s.me);
  const cur = section ?? "devices";
  if (me?.role !== "admin" && cur !== "log") {
    return (
      <Card>
        <Empty>
          <p className="mb-3">Einstellungen nur für den Administrator.</p>
          <Button variant="primary" onClick={() => navigate("/login")}>
            {t("Anmelden")}
          </Button>
        </Empty>
      </Card>
    );
  }
  return (
    <div className="grid gap-4 sm:grid-cols-[200px_1fr]">
      <nav className="flex sm:flex-col gap-1 scroll-x -mx-4 px-4 sm:mx-0 sm:px-0">
        {SECTIONS.map((s) => (
          <button
            key={s.id}
            onClick={() => navigate(`/settings/${s.id}`, true)}
            className={cx("flex items-center gap-2 px-3 h-10 rounded-xl text-[14px] whitespace-nowrap shrink-0", cur === s.id ? "bg-surface2 font-medium" : "text-muted hover:text-text")}
          >
            <s.icon size={16} /> {t(s.label)}
          </button>
        ))}
      </nav>
      <div className="min-w-0">
        {cur === "devices" && <DevicesSection />}
        {cur === "general" && <GeneralSection />}
        {cur === "notify" && <NotifySection />}
        {cur === "integrations" && <IntegrationsSection />}
        {cur === "access" && <AccessSection />}
        {cur === "backup" && <BackupSection />}
        {cur === "log" && <LogSection />}
        {cur === "system" && <SystemSection />}
      </div>
    </div>
  );
}
