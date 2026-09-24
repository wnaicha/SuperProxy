package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type NodeTestResult struct {
	OK        bool   `json:"ok"`
	Stage     string `json:"stage,omitempty"`
	TCPMs     int64  `json:"tcp_ms,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	ExitIP    string `json:"exit_ip,omitempty"`
	Error     string `json:"error,omitempty"`
	TestedAt  string `json:"tested_at,omitempty"`
}

type Node struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	Server           string          `json:"server"`
	Port             int             `json:"port"`
	UUID             string          `json:"uuid,omitempty"`
	Username         string          `json:"username,omitempty"`
	Password         string          `json:"password,omitempty"`
	Method           string          `json:"method,omitempty"`
	TLS              bool            `json:"tls,omitempty"`
	ServerName       string          `json:"server_name,omitempty"`
	Security         string          `json:"security,omitempty"`
	Flow             string          `json:"flow,omitempty"`
	Fingerprint      string          `json:"fingerprint,omitempty"`
	RealityPublicKey string          `json:"reality_public_key,omitempty"`
	RealityShortID   string          `json:"reality_short_id,omitempty"`
	Transport        string          `json:"transport,omitempty"`
	LastTest         *NodeTestResult `json:"last_test,omitempty"`
}
type Binding struct {
	NodeID string   `json:"node_id"`
	IPs    []string `json:"ips"`
}
type Config struct {
	Listen                  string    `json:"listen"`
	Token                   string    `json:"token"`
	LANInterface            string    `json:"lan_interface"`
	WANInterface            string    `json:"wan_interface"`
	TunName                 string    `json:"tun_name"`
	Nodes                   []Node    `json:"nodes"`
	Bindings                []Binding `json:"bindings"`
	AutoTestIntervalMinutes int       `json:"auto_test_interval_minutes,omitempty"`
}
type App struct {
	mu   sync.Mutex
	cfg  Config
	path string
}

func main() {
	path := env("SUPERPROXY_CONFIG", "/etc/superproxy/config.json")
	a := &App{path: path}
	if err := a.load(); err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", a.auth(a.status))
	mux.HandleFunc("/api/config", a.auth(a.config))
	mux.HandleFunc("/api/apply", a.auth(a.apply))
	mux.HandleFunc("/api/service/", a.auth(a.service))
	mux.HandleFunc("/api/logs", a.auth(a.logs))
	mux.HandleFunc("/api/core", a.auth(a.core))
	mux.HandleFunc("/api/core/install", a.auth(a.coreInstall))
	mux.HandleFunc("/api/node/test/", a.auth(a.nodeTest))
	mux.HandleFunc("/api/node/test-all", a.auth(a.nodeTestAll))
	mux.HandleFunc("/api/node/import", a.auth(a.nodeImport))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		http.FileServer(http.Dir(env("SUPERPROXY_WEB", "/usr/share/superproxy/web"))).ServeHTTP(w, r)
	}))
	go a.autoTestLoop()
	log.Printf("SuperProxy listening on %s", a.cfg.Listen)
	log.Fatal(http.ListenAndServe(a.cfg.Listen, mux))
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func randomToken() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
func (a *App) load() error {
	b, e := os.ReadFile(a.path)
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &a.cfg); e != nil {
		return e
	}
	if a.cfg.Listen == "" || a.cfg.Listen == "0.0.0.0:9090" {
		a.cfg.Listen = "0.0.0.0:9088"
	}
	if a.cfg.AutoTestIntervalMinutes <= 0 {
		a.cfg.AutoTestIntervalMinutes = 30
	}
	if a.cfg.Token == "" || a.cfg.Token == "CHANGE-ME-NOW" {
		a.cfg.Token = randomToken()
		if a.cfg.Token == "" {
			return fmt.Errorf("cannot generate token")
		}
		return a.save()
	}
	return nil
}
func (a *App) save() error {
	b, _ := json.MarshalIndent(a.cfg, "", "  ")
	return os.WriteFile(a.path, b, 0600)
}
func (a *App) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Token != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(a.cfg.Token)) != 1 {
				http.Error(w, "unauthorized", 401)
				return
			}
		}
		next(w, r)
	}
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func commandOutput(name string, args ...string) string {
	o, _ := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(o))
}
func running() bool {
	return exec.Command("pgrep", "-f", "sing-box run -c /etc/sing-box/config.json").Run() == nil
}
func (a *App) status(w http.ResponseWriter, r *http.Request) {
	path, e := exec.LookPath("sing-box")
	installed := e == nil
	ver := ""
	if installed {
		ver = commandOutput(path, "version")
	}
	jsonOut(w, map[string]any{"core_installed": installed, "core_running": running(), "core_version": ver, "nodes": len(a.cfg.Nodes), "bindings": len(a.cfg.Bindings), "arch": commandOutput("uname", "-m"), "listen": a.cfg.Listen, "tun": a.cfg.TunName})
}
func (a *App) config(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Method == "GET" {
		jsonOut(w, a.cfg)
		return
	}
	if r.Method != "PUT" {
		http.Error(w, "method", 405)
		return
	}
	var c Config
	if json.NewDecoder(r.Body).Decode(&c) != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if c.Listen == "" {
		c.Listen = a.cfg.Listen
	}
	if c.Token == "" {
		c.Token = a.cfg.Token
	}
	old := a.cfg
	a.cfg = c
	if e := a.validate(); e != nil {
		a.cfg = old
		http.Error(w, e.Error(), 400)
		return
	}
	if e := a.save(); e != nil {
		a.cfg = old
		http.Error(w, e.Error(), 500)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}
func (a *App) validate() error {
	seen := map[string]string{}
	nodes := map[string]bool{}
	for _, n := range a.cfg.Nodes {
		if n.ID == "" || n.Server == "" || n.Port < 1 || n.Port > 65535 {
			return fmt.Errorf("invalid node")
		}
		nodes[n.ID] = true
	}
	for _, b := range a.cfg.Bindings {
		if !nodes[b.NodeID] {
			return fmt.Errorf("binding references missing node %s", b.NodeID)
		}
		for _, ip := range b.IPs {
			p := net.ParseIP(ip)
			if p == nil || p.To4() == nil {
				return fmt.Errorf("invalid IPv4 %s", ip)
			}
			if old, ok := seen[ip]; ok && old != b.NodeID {
				return fmt.Errorf("IP %s bound twice", ip)
			}
			seen[ip] = b.NodeID
		}
	}
	return nil
}
func nodeOutbound(n Node) map[string]any {
	m := map[string]any{"type": n.Type, "tag": "node-" + n.ID, "server": n.Server, "server_port": n.Port}
	if n.UUID != "" {
		m["uuid"] = n.UUID
	}
	if n.Username != "" {
		m["username"] = n.Username
	}
	if n.Password != "" {
		m["password"] = n.Password
	}
	if n.Method != "" {
		m["method"] = n.Method
	}
	if n.Flow != "" {
		m["flow"] = n.Flow
	}
	if n.Type == "vless" && n.Transport != "" && n.Transport != "tcp" {
		m["transport"] = map[string]any{"type": n.Transport}
	}
	if n.TLS || n.Security == "reality" || n.Security == "tls" {
		tls := map[string]any{"enabled": true}
		if n.ServerName != "" {
			tls["server_name"] = n.ServerName
		}
		if n.Fingerprint != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": n.Fingerprint}
		}
		if n.Security == "reality" {
			tls["reality"] = map[string]any{"enabled": true, "public_key": n.RealityPublicKey, "short_id": n.RealityShortID}
		}
		m["tls"] = tls
	}
	return m
}
func (a *App) generate() error {
	os.MkdirAll("/etc/sing-box", 0755)
	outs := []map[string]any{}
	for _, n := range a.cfg.Nodes {
		outs = append(outs, nodeOutbound(n))
	}
	rules := []map[string]any{}
	for _, b := range a.cfg.Bindings {
		cidrs := []string{}
		for _, ip := range b.IPs {
			cidrs = append(cidrs, ip+"/32")
		}
		rules = append(rules, map[string]any{"source_ip_cidr": cidrs, "action": "route", "outbound": "node-" + b.NodeID})
	}
	rules = append(rules, map[string]any{"action": "reject", "method": "drop"})
	cfg := map[string]any{"log": map[string]any{"level": "info", "timestamp": true}, "inbounds": []any{map[string]any{"type": "tun", "tag": "tun-in", "interface_name": a.cfg.TunName, "address": []string{"172.19.0.1/30"}, "auto_route": true, "auto_redirect": true, "strict_route": true, "stack": "system"}}, "outbounds": outs, "route": map[string]any{"rules": rules}}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := "/etc/sing-box/config.json.tmp"
	if e := os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, "/etc/sing-box/config.json")
}
func (a *App) apply(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if e := a.validate(); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if e := a.generate(); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	if out, e := exec.Command("sing-box", "check", "-c", "/etc/sing-box/config.json").CombinedOutput(); e != nil {
		http.Error(w, "sing-box check failed: "+string(out), 500)
		return
	}
	if out, e := exec.Command("/usr/libexec/superproxy-firewall", "apply").CombinedOutput(); e != nil {
		http.Error(w, "firewall failed: "+string(out), 500)
		return
	}
	out, e := exec.Command("/etc/init.d/superproxy-singbox", "restart").CombinedOutput()
	time.Sleep(700 * time.Millisecond)
	if e != nil || !running() {
		http.Error(w, "sing-box start failed: "+string(out)+"\n"+commandOutput("logread", "-e", "superproxy-singbox"), 500)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "running": true})
}
func (a *App) service(w http.ResponseWriter, r *http.Request) {
	act := filepath.Base(r.URL.Path)
	if act != "start" && act != "stop" && act != "restart" {
		http.Error(w, "bad action", 400)
		return
	}
	out, e := exec.Command("/etc/init.d/superproxy-singbox", act).CombinedOutput()
	time.Sleep(350 * time.Millisecond)
	jsonOut(w, map[string]any{"ok": e == nil, "running": running(), "output": string(out)})
}
func (a *App) core(w http.ResponseWriter, r *http.Request) {
	path, e := exec.LookPath("sing-box")
	if e != nil {
		jsonOut(w, map[string]any{"installed": false})
		return
	}
	jsonOut(w, map[string]any{"installed": true, "path": path, "version": commandOutput(path, "version"), "running": running()})
}
func (a *App) coreInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	out, e := exec.Command("/usr/libexec/superproxy-core", "install").CombinedOutput()
	jsonOut(w, map[string]any{"ok": e == nil, "output": string(out)})
}
func (a *App) logs(w http.ResponseWriter, r *http.Request) {
	out, _ := exec.Command("logread", "-e", "sing-box").CombinedOutput()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(out)
}
func testNodeCore(n Node) NodeTestResult {
	res := NodeTestResult{TestedAt: time.Now().Format(time.RFC3339)}
	start := time.Now()
	conn, e := net.DialTimeout("tcp", net.JoinHostPort(n.Server, strconv.Itoa(n.Port)), 5*time.Second)
	if e != nil {
		res.Stage = "tcp"
		res.Error = e.Error()
		return res
	}
	conn.Close()
	res.TCPMs = time.Since(start).Milliseconds()
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		res.Stage = "listen"
		res.Error = e.Error()
		return res
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	testcfg := map[string]any{"log": map[string]any{"level": "error"}, "inbounds": []any{map[string]any{"type": "mixed", "tag": "test-in", "listen": "127.0.0.1", "listen_port": port}}, "outbounds": []any{nodeOutbound(n)}, "route": map[string]any{"default_domain_resolver": "local"}, "dns": map[string]any{"servers": []any{map[string]any{"type": "local", "tag": "local"}}}}
	b, _ := json.MarshalIndent(testcfg, "", "  ")
	tmp := fmt.Sprintf("/tmp/superproxy-node-test-%d.json", time.Now().UnixNano())
	os.WriteFile(tmp, b, 0600)
	defer os.Remove(tmp)
	if out, ce := exec.Command("sing-box", "check", "-c", tmp).CombinedOutput(); ce != nil {
		res.Stage = "config"
		res.Error = strings.TrimSpace(string(out))
		return res
	}
	var corelog bytes.Buffer
	cmd := exec.Command("sing-box", "run", "-c", tmp)
	cmd.Stdout, cmd.Stderr = &corelog, &corelog
	if e = cmd.Start(); e != nil {
		res.Stage = "start"
		res.Error = e.Error()
		return res
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	time.Sleep(500 * time.Millisecond)
	proxy := fmt.Sprintf("socks5h://127.0.0.1:%d", port)
	t := time.Now()
	out, e := exec.Command("curl", "-4", "-fsS", "--max-time", "10", "--proxy", proxy, "https://api.ipify.org").CombinedOutput()
	res.LatencyMs = time.Since(t).Milliseconds()
	if e != nil {
		res.Stage = "proxy"
		res.Error = strings.TrimSpace(string(out))
		if strings.TrimSpace(corelog.String()) != "" {
			res.Error += " | sing-box: " + strings.TrimSpace(corelog.String())
		}
		return res
	}
	res.OK = true
	res.Stage = "ok"
	res.ExitIP = strings.TrimSpace(string(out))
	return res
}
func (a *App) storeTest(id string, res NodeTestResult) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.cfg.Nodes {
		if a.cfg.Nodes[i].ID == id {
			a.cfg.Nodes[i].LastTest = &res
			_ = a.save()
			return
		}
	}
}
func (a *App) nodeTest(w http.ResponseWriter, r *http.Request) {
	id := filepath.Base(r.URL.Path)
	a.mu.Lock()
	var n *Node
	for i := range a.cfg.Nodes {
		if a.cfg.Nodes[i].ID == id {
			c := a.cfg.Nodes[i]
			n = &c
			break
		}
	}
	a.mu.Unlock()
	if n == nil {
		http.Error(w, "node not found", 404)
		return
	}
	res := testNodeCore(*n)
	a.storeTest(id, res)
	jsonOut(w, res)
}
func (a *App) nodeTestAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	go a.runAllTests()
	jsonOut(w, map[string]any{"ok": true, "started": true})
}
func (a *App) runAllTests() {
	a.mu.Lock()
	nodes := append([]Node(nil), a.cfg.Nodes...)
	a.mu.Unlock()
	for _, n := range nodes {
		res := testNodeCore(n)
		a.storeTest(n.ID, res)
		time.Sleep(250 * time.Millisecond)
	}
}
func (a *App) autoTestLoop() {
	for {
		a.mu.Lock()
		mins := a.cfg.AutoTestIntervalMinutes
		a.mu.Unlock()
		if mins < 5 {
			mins = 5
		}
		time.Sleep(time.Duration(mins) * time.Minute)
		a.runAllTests()
	}
}

func decodeB64(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if b, e := enc.DecodeString(s); e == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("invalid base64")
}
func nodeID() string { return fmt.Sprintf("n%x", time.Now().UnixNano()) }
func parseNodeLink(raw string) (Node, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Node{}, fmt.Errorf("empty link")
	}
	if strings.HasPrefix(raw, "ss://") {
		body := strings.TrimPrefix(raw, "ss://")
		name := "Shadowsocks"
		if i := strings.Index(body, "#"); i >= 0 {
			if x, e := url.QueryUnescape(body[i+1:]); e == nil && x != "" {
				name = x
			}
			body = body[:i]
		}
		var method, pass, hostport string
		if at := strings.LastIndex(body, "@"); at >= 0 {
			cred := body[:at]
			hostport = body[at+1:]
			if d, e := decodeB64(cred); e == nil {
				cred = d
			}
			parts := strings.SplitN(cred, ":", 2)
			if len(parts) != 2 {
				return Node{}, fmt.Errorf("invalid ss credentials")
			}
			method, pass = parts[0], parts[1]
		} else {
			d, e := decodeB64(body)
			if e != nil {
				return Node{}, e
			}
			at := strings.LastIndex(d, "@")
			if at < 0 {
				return Node{}, fmt.Errorf("invalid ss link")
			}
			cred, hp := d[:at], d[at+1:]
			parts := strings.SplitN(cred, ":", 2)
			if len(parts) != 2 {
				return Node{}, fmt.Errorf("invalid ss credentials")
			}
			method, pass, hostport = parts[0], parts[1], hp
		}
		host, ps, e := net.SplitHostPort(hostport)
		if e != nil {
			return Node{}, e
		}
		port, e := strconv.Atoi(ps)
		if e != nil {
			return Node{}, e
		}
		return Node{ID: nodeID(), Name: name, Type: "shadowsocks", Server: host, Port: port, Method: method, Password: pass}, nil
	}
	u, e := url.Parse(raw)
	if e != nil {
		return Node{}, e
	}
	name, _ := url.QueryUnescape(u.Fragment)
	if name == "" {
		name = u.Hostname()
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		if u.Scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	}
	n := Node{ID: nodeID(), Name: name, Server: u.Hostname(), Port: port}
	q := u.Query()
	switch strings.ToLower(u.Scheme) {
	case "socks", "socks5", "socks5h":
		n.Type = "socks"
		if u.User != nil {
			n.Username = u.User.Username()
			n.Password, _ = u.User.Password()
		}
	case "http", "https":
		n.Type = "http"
		n.TLS = u.Scheme == "https"
		if u.User != nil {
			n.Username = u.User.Username()
			n.Password, _ = u.User.Password()
		}
		n.ServerName = q.Get("sni")
	case "trojan":
		n.Type = "trojan"
		if u.User != nil {
			n.Password = u.User.Username()
		}
		n.TLS = true
		n.ServerName = q.Get("sni")
		if n.ServerName == "" {
			n.ServerName = q.Get("peer")
		}
	case "vless":
		n.Type = "vless"
		if u.User != nil {
			n.UUID = u.User.Username()
		}
		n.Security = strings.ToLower(q.Get("security"))
		n.TLS = n.Security == "tls" || n.Security == "reality"
		n.ServerName = q.Get("sni")
		n.Flow = q.Get("flow")
		n.Fingerprint = q.Get("fp")
		n.RealityPublicKey = q.Get("pbk")
		n.RealityShortID = q.Get("sid")
		n.Transport = strings.ToLower(q.Get("type"))
		if n.Transport == "" {
			n.Transport = "tcp"
		}
		if n.Security == "reality" {
			if n.ServerName == "" || n.RealityPublicKey == "" {
				return Node{}, fmt.Errorf("VLESS REALITY missing sni or pbk")
			}
		}
	default:
		return Node{}, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	if n.Server == "" || n.Port < 1 {
		return Node{}, fmt.Errorf("missing server/port")
	}
	return n, nil
}
func (a *App) nodeImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	var req struct {
		Links string `json:"links"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad json", 400)
		return
	}
	lines := strings.Fields(req.Links)
	added := []Node{}
	errs := []string{}
	for _, line := range lines {
		n, e := parseNodeLink(line)
		if e != nil {
			errs = append(errs, e.Error())
			continue
		}
		added = append(added, n)
	}
	jsonOut(w, map[string]any{"ok": len(added) > 0, "nodes": added, "errors": errs})
}
