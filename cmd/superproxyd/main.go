package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Node struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Server     string `json:"server"`
	Port       int    `json:"port"`
	UUID       string `json:"uuid,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	Method     string `json:"method,omitempty"`
	TLS        bool   `json:"tls,omitempty"`
	ServerName string `json:"server_name,omitempty"`
}
type Binding struct {
	NodeID string   `json:"node_id"`
	IPs    []string `json:"ips"`
}
type Config struct {
	Listen       string    `json:"listen"`
	Token        string    `json:"token"`
	LANInterface string    `json:"lan_interface"`
	WANInterface string    `json:"wan_interface"`
	TunName      string    `json:"tun_name"`
	Nodes        []Node    `json:"nodes"`
	Bindings     []Binding `json:"bindings"`
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
	mux.Handle("/", http.FileServer(http.Dir(env("SUPERPROXY_WEB", "/usr/share/superproxy/web"))))
	log.Printf("SuperProxy listening on %s", a.cfg.Listen)
	log.Fatal(http.ListenAndServe(a.cfg.Listen, mux))
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func (a *App) load() error {
	b, e := os.ReadFile(a.path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, &a.cfg)
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
func (a *App) status(w http.ResponseWriter, r *http.Request) {
	_, lookErr := exec.LookPath("sing-box")
	service := "not installed"
	version := ""
	if lookErr == nil {
		out, _ := exec.Command("/etc/init.d/sing-box", "status").CombinedOutput()
		service = strings.TrimSpace(string(out))
		v, _ := exec.Command("sing-box", "version").CombinedOutput()
		version = strings.TrimSpace(string(v))
	}
	jsonOut(w, map[string]any{"core_installed": lookErr == nil, "core_version": version, "service": service, "nodes": len(a.cfg.Nodes), "bindings": len(a.cfg.Bindings)})
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
	a.cfg = c
	if err := a.validate(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := a.save(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}
func (a *App) validate() error {
	seen := map[string]string{}
	nodes := map[string]bool{}
	for _, n := range a.cfg.Nodes {
		if n.ID == "" || n.Server == "" || n.Port < 1 {
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
func (a *App) apply(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.validate(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := a.generate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if out, err := exec.Command("sing-box", "check", "-c", "/etc/sing-box/config.json").CombinedOutput(); err != nil {
		http.Error(w, "sing-box check failed: "+string(out), 500)
		return
	}
	if out, err := exec.Command("/usr/libexec/superproxy-firewall", "apply").CombinedOutput(); err != nil {
		http.Error(w, "firewall failed: "+string(out), 500)
		return
	}
	out, err := exec.Command("/etc/init.d/sing-box", "restart").CombinedOutput()
	if err != nil {
		http.Error(w, string(out), 500)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}
func (a *App) generate() error {
	os.MkdirAll("/etc/sing-box", 0755)
	out := []map[string]any{{"type": "direct", "tag": "direct"}}
	for _, n := range a.cfg.Nodes {
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
		if n.TLS {
			m["tls"] = map[string]any{"enabled": true, "server_name": n.ServerName}
		}
		out = append(out, m)
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
	cfg := map[string]any{"log": map[string]any{"level": "info", "timestamp": true}, "inbounds": []any{map[string]any{"type": "tun", "tag": "tun-in", "interface_name": a.cfg.TunName, "address": []string{"172.19.0.1/30"}, "auto_route": true, "auto_redirect": true, "strict_route": true, "stack": "system"}}, "outbounds": out, "route": map[string]any{"rules": rules}}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := "/etc/sing-box/config.json.tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, "/etc/sing-box/config.json")
}
func (a *App) service(w http.ResponseWriter, r *http.Request) {
	act := filepath.Base(r.URL.Path)
	if act != "start" && act != "stop" && act != "restart" {
		http.Error(w, "bad action", 400)
		return
	}
	out, err := exec.Command("/etc/init.d/sing-box", act).CombinedOutput()
	jsonOut(w, map[string]any{"ok": err == nil, "output": string(out)})
}
func (a *App) core(w http.ResponseWriter, r *http.Request) {
	path, err := exec.LookPath("sing-box")
	if err != nil {
		jsonOut(w, map[string]any{"installed": false})
		return
	}
	out, _ := exec.Command(path, "version").CombinedOutput()
	jsonOut(w, map[string]any{"installed": true, "path": path, "version": strings.TrimSpace(string(out))})
}
func (a *App) coreInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	out, err := exec.Command("/usr/libexec/superproxy-core", "install").CombinedOutput()
	jsonOut(w, map[string]any{"ok": err == nil, "output": string(out)})
}
func (a *App) logs(w http.ResponseWriter, r *http.Request) {
	out, _ := exec.Command("logread", "-e", "sing-box").CombinedOutput()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(out)
}
