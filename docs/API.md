# API

REST unter `/api/v1`, JSON. Live-Updates als Server-Sent Events unter `/api/v1/events`.

## Zugriff

| Rolle | Wer | Darf |
|---|---|---|
| admin | Header `X-API-Key: <key>` (oder `Authorization: Bearer <key>`), Login-Cookie, oder Heimnetz solange kein Passwort gesetzt ist | alles |
| guest | Heimnetz (private Adressen), wenn „Steuern ohne Anmeldung“ an ist | lesen, steuern, Szenen anwenden, Timer |

API-Key: Weboberfläche › Einstellungen › Zugriff oder `portacontrol-cli apikey`.

## Wichtige Endpunkte

| Methode | Pfad | Zweck |
|---|---|---|
| GET | `/health` | `{ok, version, public_url}` (ohne Anmeldung) |
| GET | `/devices` | alle Anlagen mit Zustand, Verbrauch heute, Verfügbarkeit der Funktionen |
| POST | `/devices/{id}/control` | Änderung, z. B. `{"power":true,"mode":"cool","target_temperature":23,"fan_speed":102}` |
| POST | `/devices/{id}/refresh` | sofort lesen |
| GET | `/devices/{id}/diagnostics` | Diagnosewerte (Gruppen 1–7) |
| GET | `/devices/{id}/history?hours=24` | Messpunkte |
| GET | `/devices/{id}/history.csv?days=30&comma=1` | CSV |
| GET | `/devices/{id}/energy?period=day\|week\|month\|year&offset=0` | Verbrauch/Kosten je Stunde/Tag/Monat |
| GET/PUT | `/devices/{id}/timers` | Server-Timer `{on_at, on_scene_id, off_at}` (RFC 3339) |
| GET/POST/PUT/DELETE | `/scenes`, `/schedules`, `/rules` | Szenen, Wochenprogramm, Regeln |
| POST | `/scenes/{id}/apply?device={id}` | Szene anwenden |
| POST | `/setup/discover` | Suche im LAN `{target?, subnet?}` |
| POST | `/setup/cloud/login` | Midea-Konto anmelden, Anlagen des Kontos |
| POST | `/devices` | Anlage hinzufügen `{device_id, ip, port, from_cloud \| token+key \| cloud_only, name, room}` |
| GET | `/backup` · POST `/setup/import` | Sicherung (Klimacontrol-kompatibel) |
| GET/PUT | `/settings` | Strompreis, Intervalle, MQTT, Webhooks, Benachrichtigungen |
| GET | `/events` | SSE: `snapshot`, `device`, `devices`, `notice`, `log`, `scenes`, `automation`, `timers`, `settings` |

### Steuerfelder (`Control`)

`power`, `mode` (auto, cool, dry, heat, fan), `target_temperature` (16–30), `fan_speed` (1–100, 102 = Auto),
`swing`, `eco`, `turbo`, `sleep`, `purifier` (Ionisierer), `freeze_protection` (8 °C, nur Heizen),
`on_timer_minutes` / `off_timer_minutes` (Gerätetimer, 0 = löschen), `outdoor_silent`, `self_clean`, `ieco`,
`sound`, `rate_percent` (50, 75, 100), `display_on`.

Nicht übernommene Änderungen (die Anlage meldet nach drei Versuchen einen anderen Stand) liefern HTTP 409 mit
`{"error", "state"}`.

## MQTT

Präfix `portacontrol` (einstellbar):

- `portacontrol/<id>/state` – JSON mit Zustand (retained)
- `portacontrol/<id>/availability` – `online`/`offline`
- `portacontrol/<id>/set/<feld>` – `power`, `mode` (inkl. `off`, `fan_only`), `target_temperature`, `fan_mode`,
  `swing_mode`, `preset_mode` (eco, boost, sleep, none), `outdoor_silent`, `ieco`, `purifier`, `display_on`,
  `freeze_protection`, `sound`, `self_clean`, `rate_percent`, `scene` (Szenen-ID)
- `portacontrol/<id>/set` – JSON wie `Control`
- Home-Assistant-Discovery unter `homeassistant/…`
