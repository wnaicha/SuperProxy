#!/bin/sh
set -eu

VERSION="0.5.2"
OWNER="${SUPERPROXY_OWNER:-wnaicha}"
REPO="${SUPERPROXY_REPO:-SuperProxy}"
BRANCH="${SUPERPROXY_BRANCH:-main}"
# Optional mirror. Supported forms:
#   SUPERPROXY_MIRROR=https://mirror.example.com/https://github.com
#   SUPERPROXY_MIRROR=https://ghproxy.example.com/
MIRROR="${SUPERPROXY_MIRROR:-}"
RAW="https://raw.githubusercontent.com/${OWNER}/${REPO}/refs/heads/${BRANCH}"
RELEASE="https://github.com/${OWNER}/${REPO}/releases/download/v${VERSION}"

[ "$(id -u)" = 0 ] || { echo 'ERROR: run as root'; exit 1; }
case "$(uname -m)" in
  aarch64|arm64) BIN=superproxyd-linux-arm64 ;;
  x86_64|amd64) BIN=superproxyd-linux-amd64 ;;
  *) echo "ERROR: unsupported architecture: $(uname -m)"; exit 1 ;;
esac

if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
  echo 'ERROR: curl or wget is required'; exit 1
fi

human_size() {
  bytes="$1"
  awk -v b="$bytes" 'BEGIN { if (b>=1048576) printf "%.2f MB",b/1048576; else if (b>=1024) printf "%.1f KB",b/1024; else printf "%d B",b }'
}

# Visible transfer meter: curl's normal meter includes %, bytes, speed and ETA.
download() {
  url="$1" out="$2" label="${3:-file}"
  echo ""
  echo "  -> $label"
  echo "     $url"
  rm -f "$out"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --connect-timeout 12 --retry 2 --retry-delay 1 \
      --speed-time 20 --speed-limit 1024 \
      -o "$out" "$url"
  else
    wget --timeout=15 --tries=3 -O "$out" "$url"
  fi
  [ -s "$out" ] || return 1
  size="$(wc -c < "$out" | tr -d ' ')"
  echo "     OK: $(human_size "$size")"
}

mirror_url() {
  base="$1"; target="$2"
  case "$base" in
    */) printf '%s%s' "$base" "$target" ;;
    *)  printf '%s/%s' "$base" "$target" ;;
  esac
}

fetch_small() {
  rel="$1" out="$2"
  download "$RAW/$rel" "$out" "$rel"
}

fetch_binary() {
  out="$1"
  release_url="$RELEASE/$BIN"
  if [ -n "$MIRROR" ]; then
    mu="$(mirror_url "$MIRROR" "$release_url")"
    echo "[binary] Trying configured mirror first..."
    if download "$mu" "$out" "$BIN (mirror)"; then return 0; fi
    echo "WARN: mirror failed, falling back to official sources."
  fi
  echo "[binary] Trying GitHub Release..."
  if download "$release_url" "$out" "$BIN (GitHub Release)"; then return 0; fi
  echo "WARN: Release asset unavailable; falling back to GitHub Raw."
  download "$RAW/dist/$BIN" "$out" "$BIN (GitHub Raw)"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then openssl dgst -sha256 "$1" | awk '{print $NF}'
  else return 1
  fi
}

echo "========================================"
echo " SuperProxy v$VERSION installer"
echo " Repo: ${OWNER}/${REPO} (${BRANCH})"
echo " Arch: $(uname -m) -> $BIN"
[ -n "$MIRROR" ] && echo " Mirror: $MIRROR"
echo "========================================"

for c in nft jsonfilter; do
  command -v "$c" >/dev/null 2>&1 || {
    echo "[1/7] Installing dependencies..."
    opkg update
    opkg install nftables-json jsonfilter ca-bundle curl
    break
  }
done

mkdir -p /etc/superproxy /usr/share/superproxy/web /usr/libexec /usr/bin /etc/sing-box
TMP="/tmp/superproxy-install.$$"; mkdir -p "$TMP"; trap 'rm -rf "$TMP"' EXIT INT TERM

echo "[2/7] Downloading small configuration/UI/service files..."
fetch_small etc/config.example.json "$TMP/config.json"
fetch_small web/index.html "$TMP/index.html"
fetch_small openwrt/superproxy.init "$TMP/superproxy.init"
fetch_small openwrt/superproxy-singbox.init "$TMP/superproxy-singbox.init"
fetch_small openwrt/superproxy-firewall "$TMP/firewall"
fetch_small openwrt/superproxy-core "$TMP/core"

echo "[3/7] Checking backend binary..."
EXPECTED=""
if download "$RAW/dist/SHA256SUMS" "$TMP/SHA256SUMS" "SHA256SUMS"; then
  EXPECTED="$(awk -v f="$BIN" '$2==f || $2=="*"f {print $1; exit}' "$TMP/SHA256SUMS")"
fi

NEED_BIN=1
if [ -n "$EXPECTED" ] && [ -s /usr/bin/superproxyd ]; then
  CURRENT="$(sha256_file /usr/bin/superproxyd 2>/dev/null || true)"
  if [ "$CURRENT" = "$EXPECTED" ]; then
    NEED_BIN=0
    echo "  Backend unchanged (SHA256 match); skipping ~5 MB download."
  fi
fi

if [ "$NEED_BIN" = 1 ]; then
  echo "[4/7] Downloading backend (progress includes speed and ETA)..."
  fetch_binary "$TMP/superproxyd"
  if [ -n "$EXPECTED" ]; then
    GOT="$(sha256_file "$TMP/superproxyd" 2>/dev/null || true)"
    [ "$GOT" = "$EXPECTED" ] || { echo "ERROR: SHA256 mismatch for $BIN"; exit 1; }
    echo "  SHA256 verified: $GOT"
  else
    echo "WARN: checksum list unavailable; binary size was checked but SHA256 could not be verified."
  fi
else
  echo "[4/7] Backend download skipped."
fi

for f in config.json index.html superproxy.init superproxy-singbox.init firewall core; do
  [ -s "$TMP/$f" ] || { echo "ERROR: download failed: $f"; exit 1; }
done

NEW_TOKEN=""
if [ ! -f /etc/superproxy/config.json ]; then cp "$TMP/config.json" /etc/superproxy/config.json; fi
sed -i 's/0\.0\.0\.0:9090/0.0.0.0:9088/g' /etc/superproxy/config.json
if grep -q '"token"[[:space:]]*:[[:space:]]*"CHANGE-ME-NOW"' /etc/superproxy/config.json; then
  NEW_TOKEN="$(dd if=/dev/urandom bs=24 count=1 2>/dev/null | base64 | tr -dc 'A-Za-z0-9' | cut -c1-32)"
  [ -n "$NEW_TOKEN" ] || NEW_TOKEN="sp$(date +%s)$(awk 'BEGIN{srand();print int(rand()*1000000)}')"
  sed -i "s/\"token\"[[:space:]]*:[[:space:]]*\"CHANGE-ME-NOW\"/\"token\": \"$NEW_TOKEN\"/" /etc/superproxy/config.json
fi

echo "[5/7] Installing files (existing config/nodes/bindings preserved)..."
cp "$TMP/index.html" /usr/share/superproxy/web/index.html
cp "$TMP/superproxy.init" /etc/init.d/superproxy
cp "$TMP/superproxy-singbox.init" /etc/init.d/superproxy-singbox
cp "$TMP/firewall" /usr/libexec/superproxy-firewall
cp "$TMP/core" /usr/libexec/superproxy-core
if [ "$NEED_BIN" = 1 ]; then cp "$TMP/superproxyd" /usr/bin/superproxyd; fi
chmod +x /etc/init.d/superproxy /etc/init.d/superproxy-singbox /usr/libexec/superproxy-* /usr/bin/superproxyd

uci -q set network.lan.ip6assign='0'||true
uci -q delete network.lan.ip6hint||true
uci -q set dhcp.lan.dhcpv6='disabled'||true
uci -q set dhcp.lan.ra='disabled'||true
uci -q set dhcp.lan.ndp='disabled'||true
uci commit network||true; uci commit dhcp||true
mkdir -p /etc/sysctl.d
printf '%s\n' 'net.ipv6.conf.all.disable_ipv6=1' 'net.ipv6.conf.default.disable_ipv6=1' >/etc/sysctl.d/99-superproxy-ipv6.conf
sysctl -p /etc/sysctl.d/99-superproxy-ipv6.conf >/dev/null 2>&1||true

echo "[6/7] Restarting SuperProxy..."
/etc/init.d/superproxy enable
/etc/init.d/superproxy restart
sleep 1
if ! /etc/init.d/superproxy status >/dev/null 2>&1; then
  echo 'ERROR: SuperProxy failed to start'; logread -e superproxyd | tail -20; exit 1
fi

echo "[7/7] Done."
echo '========================================'
echo " SuperProxy v$VERSION installed"
echo " Architecture: $(uname -m)"
echo ' Dashboard: http://ROUTER_IP:9088'
[ -n "$NEW_TOKEN" ] && echo " Management Token: $NEW_TOKEN"
command -v sing-box >/dev/null 2>&1 && echo " sing-box: $(sing-box version | head -1)" || echo ' sing-box: NOT installed'
echo '========================================'
