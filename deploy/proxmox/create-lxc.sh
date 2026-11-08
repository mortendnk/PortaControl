#!/usr/bin/env bash
# PortaControl: create (or update) a Debian 12 LXC container on a Proxmox VE host.
#
# Run as root on the Proxmox host:
#   bash -c "$(curl -fsSL https://raw.githubusercontent.com/mortendnk/PortaControl/main/deploy/proxmox/create-lxc.sh)"
# or with a local release bundle:
#   PORTACONTROL_BUNDLE=/root/portacontrol-linux-amd64.tar.gz bash create-lxc.sh
#
# Without a GitHub release (or with PORTACONTROL_SOURCE=1) the binaries are
# built from the repository inside the container (Go and Node are fetched
# temporarily and removed again).
#
# Every value can be overridden with an environment variable. Running the
# script again updates the binaries in the existing container; data, config
# and password are kept.
set -euo pipefail

CTID="${CTID:-}"
CT_HOSTNAME="${CT_HOSTNAME:-portacontrol}"
BRIDGE="${BRIDGE:-vmbr0}"
STORAGE="${STORAGE:-local-lvm}"
TEMPLATE_STORAGE="${TEMPLATE_STORAGE:-local}"
MEMORY="${MEMORY:-512}"
CORES="${CORES:-1}"
DISK_GB="${DISK_GB:-4}"
NET="${NET:-ip=dhcp}"              # e.g. NET="ip=192.168.178.20/24,gw=192.168.178.1"
PORT="${PORT:-8080}"
PORTACONTROL_VERSION="${PORTACONTROL_VERSION:-latest}"
PORTACONTROL_REPO="${PORTACONTROL_REPO:-mortendnk/PortaControl}"
PORTACONTROL_BRANCH="${PORTACONTROL_BRANCH:-main}"
PORTACONTROL_BUNDLE="${PORTACONTROL_BUNDLE:-}"   # local tar.gz instead of a download
PORTACONTROL_SOURCE="${PORTACONTROL_SOURCE:-}"   # 1 = build from source in the container
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-90}"
GO_VERSION="${GO_VERSION:-}"       # source build: empty = latest
NODE_VERSION="${NODE_VERSION:-}"   # source build: empty = latest 22.x

log()  { printf '\033[1;34m[portacontrol]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[portacontrol] %s\033[0m\n' "$*" >&2; exit 1; }

[[ $(id -u) -eq 0 ]] || fail "run as root on the Proxmox host"
command -v pct >/dev/null || fail "pct not found: this must run on a Proxmox VE host"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# --- 1. Obtain and verify the release bundle -------------------------------
have_bundle=""
if [[ -n "$PORTACONTROL_BUNDLE" ]]; then
  [[ -f "$PORTACONTROL_BUNDLE" ]] || fail "bundle $PORTACONTROL_BUNDLE not found"
  cp "$PORTACONTROL_BUNDLE" "$WORK/bundle.tar.gz"
  if [[ -f "$PORTACONTROL_BUNDLE.sha256" ]]; then
    (cd "$WORK" && awk '{print $1"  bundle.tar.gz"}' "$PORTACONTROL_BUNDLE.sha256" | sha256sum -c -) || fail "checksum mismatch"
    log "local bundle checksum ok"
  fi
  have_bundle=1
elif [[ -z "$PORTACONTROL_SOURCE" ]]; then
  if [[ "$PORTACONTROL_VERSION" == latest ]]; then
    base="https://github.com/$PORTACONTROL_REPO/releases/latest/download"
  else
    base="https://github.com/$PORTACONTROL_REPO/releases/download/$PORTACONTROL_VERSION"
  fi
  arch="$(dpkg --print-architecture 2>/dev/null || echo amd64)"
  asset="portacontrol-linux-$arch.tar.gz"
  log "downloading $asset from $base"
  if curl -fsSL "$base/$asset" -o "$WORK/bundle.tar.gz" && curl -fsSL "$base/$asset.sha256" -o "$WORK/bundle.sha256"; then
    (cd "$WORK" && awk '{print $1"  bundle.tar.gz"}' bundle.sha256 | sha256sum -c -) || fail "checksum mismatch"
    if command -v cosign >/dev/null && curl -fsSL "$base/$asset.sigstore.json" -o "$WORK/bundle.sigstore.json" 2>/dev/null; then
      cosign verify-blob --bundle "$WORK/bundle.sigstore.json" \
        --certificate-identity-regexp "^https://github.com/$PORTACONTROL_REPO/" \
        --certificate-oidc-issuer https://token.actions.githubusercontent.com \
        "$WORK/bundle.tar.gz" || fail "signature check failed"
      log "signature ok"
    fi
    have_bundle=1
  else
    log "no release found – building from source ($PORTACONTROL_REPO@$PORTACONTROL_BRANCH) inside the container"
  fi
fi

if [[ -n "$have_bundle" ]]; then
  mkdir -p "$WORK/bundle"
  tar -xzf "$WORK/bundle.tar.gz" -C "$WORK/bundle"
  for f in portacontrold portacontrol-cli; do
    [[ -f "$WORK/bundle/$f" ]] || fail "bundle lacks $f"
    chmod 0755 "$WORK/bundle/$f"
  done
fi

# --- 2. Find or create the container --------------------------------------
existing=""
if [[ -n "$CTID" ]] && pct status "$CTID" >/dev/null 2>&1; then
  existing="$CTID"
else
  for id in $(pct list | awk 'NR>1 {print $1}'); do
    cfg="$(pct config "$id" 2>/dev/null || true)"
    if grep -qx "hostname: $CT_HOSTNAME" <<<"$cfg"; then existing="$id"; break; fi
  done
fi

if [[ -n "$existing" ]]; then
  CTID="$existing"
  log "updating existing container $CTID ($CT_HOSTNAME), data and config are kept"
  [[ "$(pct status "$CTID")" == "status: running" ]] || pct start "$CTID"
else
  CTID="${CTID:-$(pvesh get /cluster/nextid)}"
  log "creating container $CTID ($CT_HOSTNAME) on $BRIDGE"

  pveam update >/dev/null
  tmpl="$(pveam available --section system | awk '/debian-12-standard/ {print $2}' | sort -V | tail -1)"
  [[ -n "$tmpl" ]] || fail "no Debian 12 template available"
  if ! pveam list "$TEMPLATE_STORAGE" | grep -q "$tmpl"; then
    log "downloading template $tmpl"
    pveam download "$TEMPLATE_STORAGE" "$tmpl" >/dev/null
  fi

  # Bridged networking (not NAT): the UDP discovery broadcast and the TCP
  # connection to the air conditioner need the same layer 2 network.
  pct create "$CTID" "$TEMPLATE_STORAGE:vztmpl/$tmpl" \
    --hostname "$CT_HOSTNAME" --memory "$MEMORY" --cores "$CORES" \
    --rootfs "$STORAGE:$DISK_GB" --net0 "name=eth0,bridge=$BRIDGE,$NET" \
    --unprivileged 1 --features nesting=1 --onboot 1 --ostype debian \
    --description "PortaControl – Midea PortaSplit control (https://github.com/$PORTACONTROL_REPO)" >/dev/null
  pct start "$CTID"
fi

# --- 3. Wait for the network inside the container -------------------------
log "waiting for network"
ip=""
for _ in $(seq 1 60); do
  ip="$(pct exec "$CTID" -- sh -c "hostname -I 2>/dev/null | awk '{print \$1}'" || true)"
  [[ -n "$ip" ]] && break
  sleep 2
done
[[ -n "$ip" ]] || fail "container got no IP address"
log "container IP: $ip"

# --- 4. Packages, user, files ----------------------------------------------
if [[ -z "$existing" ]]; then
  log "installing base packages"
  pct exec "$CTID" -- bash -c "export DEBIAN_FRONTEND=noninteractive LC_ALL=C.UTF-8 LANG=C.UTF-8; apt-get update -qq && apt-get install -y -qq --no-install-recommends avahi-daemon ca-certificates curl tzdata >/dev/null"
  pct exec "$CTID" -- bash -c "id portacontrol >/dev/null 2>&1 || useradd --system --home /var/lib/portacontrol --shell /usr/sbin/nologin portacontrol"
fi

pct exec "$CTID" -- systemctl stop portacontrold 2>/dev/null || true

if [[ -n "$have_bundle" ]]; then
  pct push "$CTID" "$WORK/bundle/portacontrold" /usr/local/bin/portacontrold --perms 0755
  pct push "$CTID" "$WORK/bundle/portacontrol-cli" /usr/local/bin/portacontrol-cli --perms 0755
else
  log "building inside the container (needs ~1 GB RAM temporarily, takes a few minutes)"
  cat > "$WORK/build.sh" <<BUILD
#!/usr/bin/env bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive LC_ALL=C.UTF-8 LANG=C.UTF-8
apt-get install -y -qq --no-install-recommends git xz-utils make >/dev/null
arch=\$(dpkg --print-architecture)
case "\$arch" in amd64) narch=x64 ;; arm64) narch=arm64 ;; *) echo "unsupported arch \$arch"; exit 1 ;; esac
B=/tmp/pc-build; rm -rf \$B; mkdir -p \$B/node
gov="${GO_VERSION}"; [[ -n "\$gov" ]] || gov=\$(curl -fsSL "https://go.dev/VERSION?m=text" | head -1 | sed 's/^go//')
nodev="${NODE_VERSION}"; [[ -n "\$nodev" ]] || nodev=\$(curl -fsSL https://nodejs.org/dist/latest-v22.x/SHASUMS256.txt | sed -n 's/.*node-v\([0-9.]*\)-linux-x64.tar.xz/\1/p' | head -1)
echo "go \$gov, node \$nodev"
curl -fsSL "https://go.dev/dl/go\$gov.linux-\$arch.tar.gz" | tar -xz -C \$B
curl -fsSL "https://nodejs.org/dist/v\$nodev/node-v\$nodev-linux-\$narch.tar.xz" | tar -xJ -C \$B/node --strip-components=1
export PATH=\$B/go/bin:\$B/node/bin:\$PATH GOPATH=\$B/gopath GOCACHE=\$B/gocache GOTOOLCHAIN=auto
git clone --depth 1 -b "${PORTACONTROL_BRANCH}" "https://github.com/${PORTACONTROL_REPO}.git" \$B/src
cd \$B/src/server/web && npm ci --no-audit --no-fund >/dev/null && npm run build >/dev/null
cd \$B/src/server
v=\$(git -C \$B/src describe --tags --always 2>/dev/null || echo source)
c=\$(git -C \$B/src rev-parse --short HEAD)
PKG=github.com/mortendnk/portacontrol/server/internal/version
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X \$PKG.Version=\$v -X \$PKG.Commit=\$c -X \$PKG.Date=\$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o /usr/local/bin/ ./cmd/portacontrold ./cmd/portacontrol-cli
rm -rf \$B
BUILD
  pct push "$CTID" "$WORK/build.sh" /root/portacontrol-build.sh --perms 0755
  cur_mem="$(pct config "$CTID" | awk '/^memory:/ {print $2}')"
  if [[ "${cur_mem:-0}" -lt 1536 ]]; then pct set "$CTID" --memory 1536 >/dev/null; fi
  pct exec "$CTID" -- /root/portacontrol-build.sh || fail "build failed"
  pct exec "$CTID" -- rm -f /root/portacontrol-build.sh
  if [[ "${cur_mem:-0}" -lt 1536 ]]; then pct set "$CTID" --memory "$cur_mem" >/dev/null; fi
fi

cat > "$WORK/portacontrold.service" <<'UNIT'
[Unit]
Description=PortaControl (Midea PortaSplit control)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=portacontrol
Group=portacontrol
ExecStart=/usr/local/bin/portacontrold -config /etc/portacontrol/config.yaml
Restart=on-failure
RestartSec=3
UMask=0027
CapabilityBoundingSet=
AmbientCapabilities=
NoNewPrivileges=yes
# Namespace hardening (ProtectSystem, PrivateTmp, ...) is left out on purpose:
# it fails with 226/NAMESPACE in unprivileged LXC containers. The
# unprivileged container is the sandbox.

[Install]
WantedBy=multi-user.target
UNIT
pct push "$CTID" "$WORK/portacontrold.service" /etc/systemd/system/portacontrold.service

host_tz="$(timedatectl show -p Timezone --value 2>/dev/null || cat /etc/timezone 2>/dev/null || echo Europe/Berlin)"
if ! pct exec "$CTID" -- test -f /etc/portacontrol/config.yaml; then
  cat > "$WORK/config.yaml" <<CFG
# PortaControl configuration. Devices, scenes, prices, MQTT etc. are set in
# the web UI and stored in data_dir.
listen: ":$PORT"
# Address the Android app is given (QR code); empty = the container's LAN address.
public_url: ""
data_dir: /var/lib/portacontrol
# Weekly programs and timers use this time zone.
time_zone: "$host_tz"
log:
  level: info
  json: false
CFG
  pct exec "$CTID" -- mkdir -p /etc/portacontrol
  pct push "$CTID" "$WORK/config.yaml" /etc/portacontrol/config.yaml --perms 0644
fi

cat > "$WORK/portacontrol.service.xml" <<AVAHI
<?xml version="1.0" standalone='no'?>
<!DOCTYPE service-group SYSTEM "avahi-service.dtd">
<service-group>
  <name replace-wildcards="yes">PortaControl on %h</name>
  <service><type>_portacontrol._tcp</type><port>$PORT</port></service>
  <service><type>_http._tcp</type><port>$PORT</port></service>
</service-group>
AVAHI
pct push "$CTID" "$WORK/portacontrol.service.xml" /etc/avahi/services/portacontrol.service

pct exec "$CTID" -- bash -c "mkdir -p /var/lib/portacontrol && chown -R portacontrol:portacontrol /var/lib/portacontrol && systemctl daemon-reload && systemctl enable --now portacontrold >/dev/null 2>&1 && systemctl restart avahi-daemon"

# --- 5. Wait for health -----------------------------------------------------
log "waiting for http://$ip:$PORT/health"
ok=""
for _ in $(seq 1 "$HEALTH_TIMEOUT"); do
  if pct exec "$CTID" -- curl -fsS "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then ok=1; break; fi
  sleep 1
done
[[ -n "$ok" ]] || { pct exec "$CTID" -- journalctl -u portacontrold -n 30 --no-pager || true; fail "portacontrold did not become healthy"; }

version="$(pct exec "$CTID" -- /usr/local/bin/portacontrold -version)"
cli="pct exec $CTID -- runuser -u portacontrol -- /usr/local/bin/portacontrol-cli"
apikey="$($cli apikey)"
password="$($cli password --generate --if-unset)"
cat <<DONE

  PortaControl is running.
    Container : $CTID ($CT_HOSTNAME)
    Version   : $version
    URL       : http://$ip:$PORT
    mDNS      : $CT_HOSTNAME.local, service _portacontrol._tcp
    Web login : $password
    API key   : $apikey

  Next steps:
    1. Open http://$ip:$PORT, log in, Einstellungen > Anlagen > Hinzufügen.
       The PortaSplit is found on the network; token and key are fetched
       once with your MSmartHome / NetHome Plus account.
    2. Android: install portacontrol-android-<version>.apk from the release and
       scan the QR code under Einstellungen > Zugriff.
    3. Reserve $ip for this container in your router, and ideally the
       air conditioner's address too.

  CLI on this host:  $cli --help
  Update later:      run this script again, or inside the container: portacontrol-cli update

DONE
