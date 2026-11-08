import { useState } from "react";
import { Snowflake } from "lucide-react";
import { api } from "../api";
import { connectLive, loadMe, useStore } from "../store";
import { Button, Field, navigate } from "../ui";
import { t } from "../i18n";

export default function LoginPage() {
  const me = useStore((s) => s.me);
  const [pw, setPw] = useState("");
  const [err, setErr] = useState("");

  const submit = async () => {
    setErr("");
    try {
      await api("/auth/login", { body: { password: pw } });
      await loadMe();
      connectLive();
      navigate("/", true);
    } catch (e: any) {
      setErr(e.message);
    }
  };

  return (
    <div className="min-h-dvh grid place-items-center p-4">
      <form
        className="w-full max-w-sm rounded-[22px] bg-surface border border-line p-6 flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <div className="flex items-center gap-2 font-semibold text-[18px]">
          <span className="grid place-items-center w-9 h-9 rounded-xl bg-accent text-on-accent">
            <Snowflake size={20} />
          </span>
          PortaControl
        </div>
        <p className="text-[14px] text-muted">
          {me?.role === "guest"
            ? "Als Administrator anmelden, um Einstellungen, Anlagen und Automatik zu ändern."
            : "Bitte mit dem Passwort aus der Installation anmelden (portacontrol-cli password)."}
        </p>
        <Field label={t("Passwort")}>
          <input type="password" autoFocus value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="current-password" />
        </Field>
        {err && <div className="text-danger text-[13px]">{err}</div>}
        <Button type="submit" variant="primary">
          {t("Anmelden")}
        </Button>
        {me?.role === "guest" && (
          <Button variant="ghost" onClick={() => navigate("/")}>
            {t("Abbrechen")}
          </Button>
        )}
      </form>
    </div>
  );
}
