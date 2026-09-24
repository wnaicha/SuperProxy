#!/bin/sh
set -eu
[ "$(id -u)" = 0 ] || { echo '请使用 root 运行'; exit 1; }
echo '[SuperProxy] 安装/升级 sing-box 官方核心...'
if command -v curl >/dev/null 2>&1; then
  curl -fsSL https://sing-box.app/install.sh | sh
elif command -v wget >/dev/null 2>&1; then
  wget -qO- https://sing-box.app/install.sh | sh
else
  opkg update && opkg install curl ca-bundle
  curl -fsSL https://sing-box.app/install.sh | sh
fi
command -v sing-box >/dev/null
sing-box version
[ -x /etc/init.d/sing-box ] && /etc/init.d/sing-box enable || true
echo '[SuperProxy] sing-box 核心安装完成。'
