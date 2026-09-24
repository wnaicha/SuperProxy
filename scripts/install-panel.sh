#!/bin/sh
set -eu
OWNER="${SUPERPROXY_OWNER:-wnaicha}"
REPO="${SUPERPROXY_REPO:-SuperProxy}"
BRANCH="${SUPERPROXY_BRANCH:-main}"
RAW="https://raw.githubusercontent.com/${OWNER}/${REPO}/refs/heads/${BRANCH}"

[ "$(id -u)" = "0" ] || { echo "ERROR: run as root"; exit 1; }

case "$(uname -m)" in
  aarch64|arm64) BIN="superproxyd-linux-arm64" ;;
  x86_64|amd64) BIN="superproxyd-linux-amd64" ;;
  *) echo "ERROR: unsupported architecture: $(uname -m)"; exit 1 ;;
esac

fetch() {
  echo "Downloading $1"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  else
    wget -qO "$2" "$1"
  fi
}

mkdir -p /etc/superproxy /usr/share/superproxy/web /usr/libexec /usr/bin
TMP="/tmp/superproxy-install.$$"
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT INT TERM

fetch "$RAW/etc/config.example.json" "$TMP/config.json"
fetch "$RAW/web/index.html" "$TMP/index.html"
fetch "$RAW/openwrt/superproxy.init" "$TMP/init"
fetch "$RAW/openwrt/superproxy-firewall" "$TMP/firewall"
fetch "$RAW/openwrt/superproxy-core" "$TMP/core"
fetch "$RAW/dist/$BIN" "$TMP/superproxyd"

for f in config.json index.html init firewall core superproxyd; do
  [ -s "$TMP/$f" ] || { echo "ERROR: failed to download $f"; exit 1; }
done

[ -f /etc/superproxy/config.json ] || cp "$TMP/config.json" /etc/superproxy/config.json
cp "$TMP/index.html" /usr/share/superproxy/web/index.html
cp "$TMP/init" /etc/init.d/superproxy
cp "$TMP/firewall" /usr/libexec/superproxy-firewall
cp "$TMP/core" /usr/libexec/superproxy-core
cp "$TMP/superproxyd" /usr/bin/superproxyd
chmod +x /etc/init.d/superproxy /usr/libexec/superproxy-* /usr/bin/superproxyd

if ! command -v nft >/dev/null 2>&1; then
  opkg update
  opkg install nftables-json
fi

# Dedicated SuperProxy phone LAN: IPv6 disabled.
uci -q set network.lan.ip6assign='0' || true
uci -q delete network.lan.ip6hint || true
uci -q set dhcp.lan.dhcpv6='disabled' || true
uci -q set dhcp.lan.ra='disabled' || true
uci -q set dhcp.lan.ndp='disabled' || true
uci commit network || true
uci commit dhcp || true

mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-superproxy-ipv6.conf <<'EOF'
net.ipv6.conf.all.disable_ipv6=1
net.ipv6.conf.default.disable_ipv6=1
EOF
sysctl -p /etc/sysctl.d/99-superproxy-ipv6.conf >/dev/null 2>&1 || true

/etc/init.d/superproxy enable
/etc/init.d/superproxy restart

echo "========================================"
echo " SuperProxy panel installed"
echo " Architecture: $(uname -m)"
echo " Dashboard: http://ROUTER_IP:9090"
if command -v sing-box >/dev/null 2>&1; then
  echo " sing-box: installed"
else
  echo " sing-box: NOT installed (install it from Dashboard or install-core.sh)"
fi
echo "========================================"
