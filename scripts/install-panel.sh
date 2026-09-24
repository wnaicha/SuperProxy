#!/bin/sh
set -eu
REPO_URL=${REPO_URL:-https://github.com/YOURNAME/superproxy/releases/latest/download}
ARCH=$(uname -m)
case "$ARCH" in aarch64|arm64) BIN=superproxyd-linux-arm64;; x86_64|amd64) BIN=superproxyd-linux-amd64;; *) echo "Unsupported arch: $ARCH"; exit 1;; esac
[ "$(id -u)" = 0 ] || { echo 'Run as root'; exit 1; }
command -v nft >/dev/null 2>&1 || { opkg update; opkg install nftables-json jsonfilter ca-bundle curl; }
mkdir -p /etc/superproxy /usr/share/superproxy/web /usr/libexec
[ -f /etc/superproxy/config.json ] || cp ./etc/config.example.json /etc/superproxy/config.json
cp ./web/index.html /usr/share/superproxy/web/index.html
cp ./openwrt/superproxy.init /etc/init.d/superproxy
cp ./openwrt/superproxy-firewall /usr/libexec/superproxy-firewall
cp ./openwrt/superproxy-core /usr/libexec/superproxy-core
chmod +x /etc/init.d/superproxy /usr/libexec/superproxy-firewall /usr/libexec/superproxy-core
if [ -f ./dist/$BIN ]; then cp ./dist/$BIN /usr/bin/superproxyd; else curl -fL "$REPO_URL/$BIN" -o /usr/bin/superproxyd; fi
chmod +x /usr/bin/superproxyd
# Dedicated phone gateway: IPv6 disabled.
uci -q set network.lan.ip6assign='0' || true
uci -q delete network.lan.ip6hint || true
uci -q set dhcp.lan.dhcpv6='disabled' || true
uci -q set dhcp.lan.ra='disabled' || true
uci -q set dhcp.lan.ndp='disabled' || true
uci commit network; uci commit dhcp
mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-superproxy-ipv6.conf <<SYS
net.ipv6.conf.all.disable_ipv6=1
net.ipv6.conf.default.disable_ipv6=1
SYS
sysctl -p /etc/sysctl.d/99-superproxy-ipv6.conf || true
/etc/init.d/superproxy enable
/etc/init.d/superproxy restart
echo 'SuperProxy panel installed: http://ROUTER_IP:9090'
echo 'If sing-box is absent, open Dashboard and click 安装内核.'
