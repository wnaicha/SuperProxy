# SuperProxy v0.4.1

# SuperProxy v0.3.1

OpenWrt/iStoreOS fixed-IP proxy controller for sing-box.

## v0.3.1
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


## v0.3.1
- 修复 Dashboard 登录按钮事件与登录反馈。
- 禁用 Dashboard 静态页面缓存，升级后立即加载新 UI。
- 新增分享链接批量导入：SOCKS5、HTTP/HTTPS、VLESS、Trojan、Shadowsocks。
- 节点导入后可直接使用节点测试功能验证出口 IPv4 与耗时。

## v0.4.0
- Complete VLESS REALITY/Vision share-link parsing for sing-box: security, flow, SNI, uTLS fingerprint, REALITY public key and short ID.
- Node tests now run `sing-box check` on the exact temporary outbound before starting it.
- Test failures return a stage (`tcp`, `config`, `start`, `proxy`) and include sing-box diagnostics.
- Dashboard exposes REALITY fields for manual VLESS nodes and identifies REALITY/Vision nodes in the list.


## v0.4.1
- Fixed/normalized remote `wget | sh` panel installer.
- Installer prints progress and download errors instead of failing silently.
- Keeps existing `/etc/superproxy/config.json` and migrates dashboard port 9090 to 9088.
- Retains v0.4 VLESS REALITY/Vision import and sing-box-backed node testing.

## v0.5.0
- 自动节点测速，默认每 30 分钟后台逐个测试；可选 5/15/30/60/180/360 分钟。
- 每个节点持久保存最后测速时间、完整代理耗时、TCP 耗时、出口 IPv4 与失败阶段；刷新/重新打开面板仍显示。
- 节点列表直接“绑定”弹窗，无需滚动到页面底部；同一 IP 保存时自动从其他节点解绑。
- 节点实时搜索：节点名、服务器 IP、绑定 IP、协议、安全类型和 Flow。
- 节点列表与绑定总览 UI 优化，增加“全部测速”。
