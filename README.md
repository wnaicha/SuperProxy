# SuperProxy · 超级代理

面向 OpenWrt 代理网关的独立管理面板。设计目标：固定手机 IPv4 -> 指定 sing-box 节点；一个节点可绑定多个手机 IP；未匹配流量默认拒绝；IPv6 关闭。

## 支持架构

- ARM64 / aarch64（包括 MT7986）
- AMD64 / x86_64

Web 前端完全共用；安装器只根据 `uname -m` 选择对应的 `superproxyd` 后端二进制。

## 推荐安装方式（两步）

克隆/上传仓库后，在仓库根目录执行：

```sh
# 1. 单独安装 sing-box 官方核心
sh scripts/install-core.sh

# 2. 安装 SuperProxy 面板
sh scripts/install-panel.sh
```

也可以只执行第 2 步。面板启动后如果检测不到 sing-box，会显示 **“安装 sing-box 内核”** 按钮，点击后调用 sing-box 官方安装脚本安装核心。

默认面板：`http://路由器IP:9090`

首次使用请修改 `/etc/superproxy/config.json` 中的 `token`，然后执行：

```sh
/etc/init.d/superproxy restart
```

## 节点填写

Dashboard 提供表单，目前支持基础字段：

- SOCKS5：服务器、端口、用户名、密码
- HTTP：服务器、端口、用户名、密码、可选 TLS/SNI
- VLESS：服务器、端口、UUID、可选 TLS/SNI
- Trojan：服务器、端口、密码、可选 TLS/SNI
- Shadowsocks：服务器、端口、method、password

> Reality、WS、gRPC、Hysteria2 等扩展字段尚未放进可视化表单；后续版本再加入。高级 JSON 入口保留用于调试。

## 多 IP 绑定

一个节点可以绑定多个固定 IPv4：

```text
192.168.20.101
192.168.20.102
192.168.20.103
```

也支持范围：

```text
192.168.20.101-192.168.20.110
```

同一个手机 IP 不允许同时绑定两个节点。

## 服务

```sh
/etc/init.d/superproxy start
/etc/init.d/superproxy stop
/etc/init.d/superproxy restart
/etc/init.d/superproxy enable
```

sing-box 核心由其自己的 `/etc/init.d/sing-box` 管理。

## 重要说明

SuperProxy 会生成 sing-box TUN 配置并启用 `auto_route`、`auto_redirect`、`strict_route`。当前生成器使用 sing-box 新式 route action（`route` / `reject`），不依赖已移除的 `block` special outbound。

防泄漏采用两层思路：sing-box 未匹配流量 reject；nftables 对已绑定手机 IPv4 增加直出 WAN 的 guard。由于不同 OpenWrt 固件的 WAN 设备名可能不同，请确认 `/etc/superproxy/config.json` 的 `wan_interface` 与实际 nft `oifname` 一致。

在投入 100 台设备前，至少实机验证：节点断线时手机不能直出 WAN、DNS 行为、UDP/QUIC、WAN 设备名、重启恢复以及 25/50/100 台逐级并发。
