# Architektur

```
Android-App ──┐
Weboberfläche ├── HTTP /api/v1 + SSE ──> portacontrold ──TCP 6444 (V3, AES)──> PortaSplit
Home Assistant┘   MQTT <────────────────┘      │  └─UDP 6445/20086 (Suche)
                                               └─HTTPS (optional)──> Midea-Cloud (Token, Rückfall)
```

`server/` (Go, ein statisches Binary, Weboberfläche eingebettet):

| Paket | Aufgabe |
|---|---|
| `internal/midea` | Protokoll: AES/MD5/SHA256, V2-Paket (0x5A5A), V3-Transport (0x8370, Handshake, Sitzungsschlüssel), Befehlsrahmen (0x40 Zustand, 0x41 Abfragen, 0xB0/B1 Eigenschaften, Gruppen 1–7), Antworten, Discovery, Cloud (MSmartHome und NetHome Plus: Login, Token, Gerätelisten, transparentes Senden), `Device` mit Wiederholung, Prüfung „übernommen?“, LAN→Cloud-Rückfall und Trennen nach Leerlauf |
| `internal/fakeac` | Simulierte PortaSplit (TCP + Discovery) für Tests, Demo und Entwicklung |
| `internal/app` | Geräteverwaltung, Polling (schneller, solange jemand zuschaut), Verlauf, Verbrauch, Timer, Wochenprogramm, Regeln, Benachrichtigungen, Webhooks, Einrichtung, Sicherung |
| `internal/store` | SQLite (rein Go): Dokumente (Anlagen, Szenen, Programme, Regeln, Timer), Einstellungen, Messpunkte, Protokoll, Sitzungen |
| `internal/api` | REST, SSE, Rollen (Admin/Haushalt), Login |
| `internal/mqtt` | MQTT-Bridge mit Home-Assistant-Discovery |
| `web/` | React + Vite + Tailwind |

Entscheidungen:

- **Eine Verbindung**: Die Anlage akzeptiert nur eine TCP-Verbindung. Jeder Auftrag läuft seriell; nach 8 s
  Leerlauf wird getrennt, damit Midea-App und andere Clients nicht ausgesperrt werden.
- **Bestätigte Befehle**: Nach jedem Stellbefehl wird der Zustand gelesen und nur die geänderten Felder werden
  verglichen; bis zu drei Versuche (wie Klimacontrol). Ging ein Befehl hinaus und brach dann die Verbindung ab,
  wird er nicht still über die Cloud wiederholt.
- **Funktionen je Modus**: Eco nur beim Kühlen, Turbo/Außen leise beim Kühlen/Heizen, Frostschutz nur beim
  Heizen – nicht passende Wünsche werden vor dem Senden entfernt (wie die Midea-App sie ausgraut).
- **Verbrauch**: aus dem binären Zähler der Anlage (Gruppe 4, 0,01 kWh), sonst Leistung × Zeit.
- **Zeitzone**: Wochenprogramm und Auswertungen nach `time_zone` aus der Konfiguration.
