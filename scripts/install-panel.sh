#!/bin/sh
set -eu
VERSION="0.4.1"
OWNER="${SUPERPROXY_OWNER:-wnaicha}"; REPO="${SUPERPROXY_REPO:-SuperProxy}"; BRANCH="${SUPERPROXY_BRANCH:-main}"
RAW="https://raw.githubusercontent.com/${OWNER}/${REPO}/refs/heads/${BRANCH}"
[ "$(id -u)" = 0 ] || { echo 'ERROR: run as root'; exit 1; }
case "$(uname -m)" in aarch64|arm64) BIN=superproxyd-linux-arm64;; x86_64|amd64) BIN=superproxyd-linux-amd64;; *) echo "ERROR: unsupported architecture: $(uname -m)"; exit 1;; esac
fetch(){ echo "Downloading: $1"; if command -v curl >/dev/null 2>&1; then curl -fL --connect-timeout 15 --retry 2 "$1" -o "$2"; else wget -O "$2" "$1"; fi; }
echo "== SuperProxy v$VERSION installer =="
echo "Repository: ${OWNER}/${REPO} (${BRANCH})"
echo "Architecture: $(uname -m)"
if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then echo 'ERROR: curl or wget is required'; exit 1; fi
for c in nft jsonfilter; do command -v "$c" >/dev/null 2>&1 || { echo "Installing dependencies..."; opkg update; opkg install nftables-json jsonfilter ca-bundle curl; break; }; done
mkdir -p /etc/superproxy /usr/share/superproxy/web /usr/libexec /usr/bin /etc/sing-box
TMP="/tmp/superproxy-install.$$"; mkdir -p "$TMP"; trap 'rm -rf "$TMP"' EXIT INT TERM
fetch "$RAW/etc/config.example.json" "$TMP/config.json"; fetch "$RAW/web/index.html" "$TMP/index.html"; fetch "$RAW/openwrt/superproxy.init" "$TMP/superproxy.init"; fetch "$RAW/openwrt/superproxy-singbox.init" "$TMP/superproxy-singbox.init"; fetch "$RAW/openwrt/superproxy-firewall" "$TMP/firewall"; fetch "$RAW/openwrt/superproxy-core" "$TMP/core"; fetch "$RAW/dist/$BIN" "$TMP/superproxyd"
for f in config.json index.html superproxy.init superproxy-singbox.init firewall core superproxyd;do [ -s "$TMP/$f" ]||{ echo "ERROR: download failed: $f";exit 1;};done
NEW_TOKEN=""
if [ ! -f /etc/superproxy/config.json ];then cp "$TMP/config.json" /etc/superproxy/config.json;fi
# v0.3 migration: avoid Mihomo 9090 and replace the insecure placeholder token.
sed -i 's/0\.0\.0\.0:9090/0.0.0.0:9088/g' /etc/superproxy/config.json
if grep -q '"token"[[:space:]]*:[[:space:]]*"CHANGE-ME-NOW"' /etc/superproxy/config.json;then NEW_TOKEN="$(dd if=/dev/urandom bs=24 count=1 2>/dev/null | base64 | tr -dc 'A-Za-z0-9' | cut -c1-32)"; [ -n "$NEW_TOKEN" ]||NEW_TOKEN="sp$(date +%s)$(awk 'BEGIN{srand();print int(rand()*1000000)}')"; sed -i "s/\"token\"[[:space:]]*:[[:space:]]*\"CHANGE-ME-NOW\"/\"token\": \"$NEW_TOKEN\"/" /etc/superproxy/config.json;fi
cp "$TMP/index.html" /usr/share/superproxy/web/index.html; cp "$TMP/superproxy.init" /etc/init.d/superproxy; cp "$TMP/superproxy-singbox.init" /etc/init.d/superproxy-singbox; cp "$TMP/firewall" /usr/libexec/superproxy-firewall; cp "$TMP/core" /usr/libexec/superproxy-core; cp "$TMP/superproxyd" /usr/bin/superproxyd
chmod +x /etc/init.d/superproxy /etc/init.d/superproxy-singbox /usr/libexec/superproxy-* /usr/bin/superproxyd
uci -q set network.lan.ip6assign='0'||true; uci -q delete network.lan.ip6hint||true; uci -q set dhcp.lan.dhcpv6='disabled'||true; uci -q set dhcp.lan.ra='disabled'||true; uci -q set dhcp.lan.ndp='disabled'||true; uci commit network||true; uci commit dhcp||true
mkdir -p /etc/sysctl.d; printf '%s\n' 'net.ipv6.conf.all.disable_ipv6=1' 'net.ipv6.conf.default.disable_ipv6=1' >/etc/sysctl.d/99-superproxy-ipv6.conf; sysctl -p /etc/sysctl.d/99-superproxy-ipv6.conf >/dev/null 2>&1||true
/etc/init.d/superproxy enable; /etc/init.d/superproxy restart; sleep 1
if ! /etc/init.d/superproxy status >/dev/null 2>&1;then echo 'ERROR: SuperProxy failed to start'; logread -e superproxyd | tail -20; exit 1;fi
echo '========================================'; echo " SuperProxy v$VERSION installed"; echo " Architecture: $(uname -m)"; echo ' Dashboard: http://ROUTER_IP:9088'; [ -n "$NEW_TOKEN" ]&&echo " Management Token: $NEW_TOKEN"; command -v sing-box >/dev/null 2>&1&&echo " sing-box: $(sing-box version | head -1)"||echo ' sing-box: NOT installed'; echo '========================================'
