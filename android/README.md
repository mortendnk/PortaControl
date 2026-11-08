# PortaControl für Android

Kotlin + Jetpack Compose, minSdk 26. Spricht nur mit dem PortaControl-Server (nicht direkt mit der Anlage –
die nimmt nur eine Verbindung an).

- **Koppeln**: QR-Code aus Weboberfläche › Einstellungen › Zugriff scannen, Server per mDNS
  (`_portacontrol._tcp`) finden oder Adresse + API-Key eingeben. Links `portacontrol://connect?url=…&key=…`
  funktionieren auch.
- **Live**: Server-Sent Events, solange die App sichtbar ist.
- **Widget** und **Schnelleinstellungs-Kachel** steuern die Lieblingsanlage (Einstellungen in der App).
- Push-Benachrichtigungen bei geschlossener App über ntfy (in der Weboberfläche einrichten).

```bash
./gradlew testDebugUnitTest assembleDebug     # app/build/outputs/apk/debug/
./gradlew assembleRelease                     # mit android/keystore.properties signiert, sonst Debug-Schlüssel
```
