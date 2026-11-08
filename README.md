# PortaControl

Selbst gehostete Steuerung für **Midea-PortaSplit**-Klimaanlagen: lokal im Heimnetz über das Midea-Protokoll V3
(„8370“, TCP 6444, AES), mit Weboberfläche (als App installierbar), Android-App, Wochenprogramm,
Klima-Regeln, Verbrauchsauswertung und Home-Assistant-Anbindung. Läuft als kleiner Dienst in einem
Proxmox-LXC-Container – rund um die Uhr, auch wenn kein Rechner oder Handy an ist.

Das Protokoll ist aus [Klimacontrol](https://github.com/mlafx/Klimacontrol) nach Go portiert und gegen dessen
(aus msmart-ng erzeugte) Testvektoren byte-genau geprüft.

## Funktionen

| Bereich | Umfang |
|---|---|
| Bedienung (wie Midea-App) | Ein/Aus, Kühlen, Heizen, Entfeuchten, Lüften, Auto; Soll 16–30 °C; Lüfter stufenlos 1–100 % + Leise/Niedrig/Mittel/Hoch/Max/Auto; Schwenken, Eco, Turbo, Schlafmodus, Außengerät leise, iECO, Ionisierer, Frostschutz 8 °C, Selbstreinigung, Anzeige, Signalton, Leistungsbegrenzung; Gerätetimer Ein/Aus; Filter- und Fehlermeldung; Verbrauch |
| Wie Klimacontrol | LAN-Steuerung mit Rückfall auf die Cloud („Automatisch“, „Nur WLAN“, „Nur Cloud“), Token/Schlüssel per Midea-Konto (MSmartHome / NetHome Plus) oder manuell, Szenen, Ein-/Ausschalt-Timer, Verlauf (Temperaturen, Leistung, Verbrauch, Kosten je Tag/Woche/Monat/Jahr), CSV-Export, Diagnose (Verdichter, Kältekreis, Lüfter, Fühler, EEV, Kondensatpumpe …), Adresse neu suchen, Sicherung (Klimacontrol-kompatibel), Deutsch/Englisch |
| Neu | Server läuft 24/7: Timer und **Wochenprogramm** laufen ohne offene App · **Klima-Regeln** („wenn innen > 27 °C zwischen 10 und 22 Uhr → Kühlen 24 °C“, mit Hysterese) · **mehrere Anlagen** · **Home Assistant** per MQTT-Discovery (Klima-Entity, Sensoren, Schalter, Select, Button) · **Webhooks** · **Push** via ntfy (Filter, Fehler, offline, Temperatur-Alarm) · Kosten, CO₂, Laufzeit · Kindersicherung (min./max. Soll, Gästesperre) · Haushaltszugang ohne Login im Heimnetz, Einstellungen nur mit Passwort · REST-API + Live-Events (SSE) · CLI · Demo-Modus mit simulierter PortaSplit |
| Android | Kopplung per QR-Code oder mDNS-Suche, Live-Steuerung mit Thermostat-Regler, Szenen, Wochenprogramm/Regeln an/aus, Verbrauchs- und Temperaturverlauf, **Homescreen-Widget** (Temperatur, −/+, Ein/Aus) und **Schnelleinstellungs-Kachel** |

## Schnellstart: Proxmox LXC

Auf dem Proxmox-Host als root:

```bash
bash -c "$(curl -fsSL https://raw.githubusercontent.com/mortendnk/PortaControl/main/deploy/proxmox/create-lxc.sh)"
```

Das Skript legt einen unprivilegierten Debian-12-Container an `vmbr0` an, installiert PortaControl und zeigt
URL, Web-Passwort und API-Key. Gibt es noch kein GitHub-Release, baut es PortaControl direkt im Container aus
dem Quellcode. Danach:

1. URL öffnen, anmelden, **Einstellungen › Anlagen › Hinzufügen**: Die PortaSplit wird im Netz gefunden, Token
   und Schlüssel werden einmalig mit dem MSmartHome-/NetHome-Plus-Konto geholt und an der Anlage geprüft. Ohne
   Konto: Token/Schlüssel manuell eingeben oder eine Klimacontrol-Sicherung einlesen.
2. **Android**: `portacontrol-android-<version>.apk` aus dem Release installieren und den QR-Code unter
   **Einstellungen › Zugriff** scannen.
3. Im Router die Adressen von Container und Klimaanlage fest vergeben.

Erneut ausführen = Update (Daten bleiben erhalten); im Container geht auch `portacontrol-cli update`.

Optionen per Umgebungsvariable, z. B. `CTID=120 NET="ip=192.168.178.20/24,gw=192.168.178.1" PORT=8080 bash create-lxc.sh`.
Alle Variablen stehen oben im [Skript](deploy/proxmox/create-lxc.sh).

## Andere Wege

- **Docker**: `docker compose -f deploy/docker/compose.yaml up -d --build` (Host-Netzwerk ist nötig für die Suche
  im LAN). Passwort: `docker compose -f deploy/docker/compose.yaml exec portacontrol portacontrol-cli -config /data/config.yaml password --generate`.
- **Aus dem Quellcode**: `make build` (Go ≥ 1.26, Node ≥ 20), dann `bin/portacontrold -config config.yaml`.
- **Ausprobieren ohne Anlage**: `make demo` startet den Server mit simulierter PortaSplit auf http://127.0.0.1:8080.

## Betriebshinweise

- **Eine Verbindung gleichzeitig**: Die PortaSplit nimmt nur eine lokale Verbindung an. PortaControl trennt nach
  jeder Abfrage (Standard 8 s), damit die Midea-App weiter funktioniert. „Anlage belegt“ heißt meist: Midea-App
  oder ein zweites Programm (z. B. Klimacontrol) ist gerade verbunden.
- **Netz**: UDP 6445/20086 (Suche) und TCP 6444 (Steuerung) müssen zwischen Container und Anlage erreichbar sein –
  der Container hängt deshalb gebrückt an `vmbr0`, nicht hinter NAT.
- **Zugriff**: Im Heimnetz darf jeder bedienen (abschaltbar), Einstellungen nur mit Passwort oder API-Key. Für
  Zugriff von außen einen VPN (WireGuard/Tailscale) oder einen Reverse-Proxy mit TLS davorsetzen.
- **Datenschutz**: Token, Schlüssel und Verlauf liegen nur in `/var/lib/portacontrol`. Verbindungen zu Midea gibt
  es nur mit eingerichtetem Midea-Konto (Token-Abruf, Cloud-Rückfall).

## Entwicklung

```bash
make test                       # Go-Tests (Protokollvektoren, Simulator, API)
make web-dev                    # Weboberfläche mit Hot Reload gegen portacontrold auf :8080
make demo                       # Server mit simulierter PortaSplit
make fakeac                     # simulierte PortaSplit im LAN (für die Midea-Einrichtung ohne Hardware)
make bundle                     # dist/-Bundle für create-lxc.sh (PORTACONTROL_BUNDLE=…)
cd android && ./gradlew testDebugUnitTest assembleRelease
```

Ein Release entsteht durch ein Tag `v*` (GitHub Actions: Linux-Bundles amd64/arm64 mit Sigstore-Signatur,
APK, Container-Image). Für eine stabile APK-Signatur die Secrets `ANDROID_KEYSTORE_BASE64` und
`ANDROID_KEYSTORE_PASSWORD` (Alias `portacontrol`) setzen; ohne sie wird mit dem Debug-Schlüssel signiert.

Architektur und API: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) · [docs/API.md](docs/API.md)

## Lizenz und Hinweise

MIT. Die Protokollimplementierung folgt [Klimacontrol](https://github.com/mlafx/Klimacontrol) (MIT),
[msmart-ng](https://github.com/mill1000/midea-msmart), [midea-beautiful-air](https://github.com/nbogojevic/midea-beautiful-air)
und [midea_ac_lan](https://github.com/georgezhao2010/midea_ac_lan); die Testvektoren stammen aus Klimacontrol.

PortaControl ist ein privates Projekt und kein Produkt von Midea. Midea, MSmartHome, NetHome Plus und PortaSplit
sind Marken ihrer jeweiligen Inhaber. Nutzung auf eigene Verantwortung.
