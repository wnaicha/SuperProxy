# SuperProxy v0.3.0

OpenWrt/iStoreOS fixed-IP proxy controller for sing-box.

## v0.3.0
- Dashboard default port moved to 9088 to avoid common Mihomo 9090 conflict.
- Dedicated `superproxy-singbox` procd service; Apply now validates config, applies firewall, starts sing-box, and verifies it is running.
- Dashboard shows installed/running state, architecture, node count and bound-IP count.
- Per-node **Test** performs TCP reachability plus a real proxied IPv4 request and reports exit IP and elapsed time.
- Installer supports amd64 and arm64, preserves existing config/nodes/bindings, migrates 9090 -> 9088, replaces the default token, and checks daemon startup.
- Refreshed responsive Dashboard UI.

## Install / upgrade

```sh
wget -qO- https://raw.githubusercontent.com/wnaicha/SuperProxy/refs/heads/main/scripts/install-panel.sh | sh
```

The installer preserves `/etc/superproxy/config.json`. If the token is still `CHANGE-ME-NOW`, it creates a random token and prints it once.

## Files
- `cmd/superproxyd/main.go` backend
- `web/index.html` Dashboard
- `openwrt/superproxy.init` Dashboard service
- `openwrt/superproxy-singbox.init` managed sing-box service
- `openwrt/superproxy-firewall` fail-closed guard
- `scripts/install-panel.sh` remote installer/upgrader
- `dist/` static amd64/arm64 backend binaries
