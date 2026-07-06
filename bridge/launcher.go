package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"github.com/zalando/go-keyring"
)

const (
	launcherKeyringService = "BimmLite"
	launcherKeyringUser    = "bridge-session-token"
	defaultAppURL          = "https://34.44.19.28"
	defaultWSURL           = "wss://34.44.19.28/ws/bridge"
)

type Mode string

const (
	ModeLauncher Mode = "launcher"
	ModeBridge   Mode = "bridge"
	ModeDiagnose Mode = "diagnose"
)

type launcherApp struct {
	cfg     Config
	appURL  string
	traceID string
	log     *launcherLogger

	statusMu sync.Mutex
	status   BridgeStatus
	statusCh chan BridgeStatus

	bridgeMu    sync.Mutex
	bridgeStart bool
	bridgeCtx   context.Context
	bridgeStop  context.CancelFunc

	bootstrapMu   sync.Mutex
	bootstrapStop func()
}

type launcherLogger struct {
	mu   sync.Mutex
	file *os.File
}

type launcherLogEntry struct {
	Ts         string `json:"ts"`
	Level      string `json:"level"`
	Module     string `json:"module"`
	Event      string `json:"event"`
	TraceID    string `json:"trace_id"`
	SessionID  string `json:"session_id"`
	UserID     string `json:"user_id"`
	VIN        string `json:"vin"`
	ECU        string `json:"ecu"`
	DurationMS int64  `json:"duration_ms"`
	PayloadHex string `json:"payload_hex"`
	Result     string `json:"result"`
	Error      string `json:"error"`
	Message    string `json:"message"`
}

func detectMode() Mode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BIMM_MODE"))) {
	case "bridge":
		return ModeBridge
	case "diagnose":
		return ModeDiagnose
	}
	for _, arg := range os.Args[1:] {
		switch strings.ToLower(strings.TrimSpace(arg)) {
		case "--bridge", "-bridge", "--mode=bridge":
			return ModeBridge
		case "--diagnose", "-diagnose", "--mode=diagnose":
			return ModeDiagnose
		}
	}
	return ModeLauncher
}

func runLauncherCLI() {
	cfg := loadConfig(false)
	appURL := strings.TrimSpace(os.Getenv("APP_URL"))
	if appURL == "" {
		appURL = defaultAppURL
	}

	logger, err := newLauncherLogger()
	if err != nil {
		log.Printf("launcher log setup failed: %v", err)
	}
	if logger != nil {
		defer logger.Close()
	}

	traceID := newTraceID()
	app := &launcherApp{
		cfg:      cfg,
		appURL:   appURL,
		traceID:  traceID,
		log:      logger,
		statusCh: make(chan BridgeStatus, 16),
	}
	app.logEvent("INFO", "launcher", "launcher.start", traceID, cfg.SessionID, "launcher starting", nil)

	token, source, err := app.resolveToken()
	if err != nil {
		app.logEvent("ERROR", "security", "launcher.token.resolve_failed", traceID, cfg.SessionID, err.Error(), map[string]any{"source": source})
	} else if token != "" {
		app.logEvent("INFO", "security", "launcher.token.loaded", traceID, cfg.SessionID, "session token loaded", map[string]any{"source": source})
	}

	ctx, cancel := context.WithCancel(context.Background())
	app.bridgeCtx = ctx
	app.bridgeStop = cancel

	if token != "" {
		app.startBridge(ctx, traceID, token)
		_ = openURL(app.appURL)
	}

	if strings.TrimSpace(token) == "" {
		if bootstrapURL, stop, err := app.startBootstrapServer(); err != nil {
			app.logEvent("ERROR", "launcher", "launcher.bootstrap.failed", traceID, cfg.SessionID, err.Error(), nil)
		} else {
			app.bootstrapMu.Lock()
			app.bootstrapStop = stop
			app.bootstrapMu.Unlock()
			app.logEvent("INFO", "launcher", "launcher.bootstrap.open", traceID, cfg.SessionID, "opening token bootstrap page", map[string]any{"url": bootstrapURL})
			_ = openURL(bootstrapURL)
		}
	}

	systray.Run(func() {
		app.onReady()
	}, func() {
		app.onExit()
	})
}

func (a *launcherApp) onReady() {
	iconDisconnected := buildStatusIcon(false)
	iconConnected := buildStatusIcon(true)

	systray.SetIcon(iconDisconnected)
	systray.SetTooltip("BimmLite: disconnected")

	mOpen := systray.AddMenuItem("Open App", "Open the BimmLite application")
	mLogs := systray.AddMenuItem("Logs", "Open local launcher logs")
	systray.AddSeparator()
	mExit := systray.AddMenuItem("Exit", "Stop bridge and quit")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				a.logEvent("INFO", "ui", "launcher.open_app", a.traceID, a.cfg.SessionID, "open app requested", nil)
				_ = openURL(a.appURL)
			case <-mLogs.ClickedCh:
				a.logEvent("INFO", "ui", "launcher.open_logs", a.traceID, a.cfg.SessionID, "open logs requested", nil)
				_ = openPath(a.logDir())
			case <-mExit.ClickedCh:
				a.logEvent("INFO", "ui", "launcher.exit", a.traceID, a.cfg.SessionID, "exit requested", nil)
				systray.Quit()
				return
			case status := <-a.statusCh:
				a.applyStatus(status, iconConnected, iconDisconnected)
			}
		}
	}()
}

func (a *launcherApp) onExit() {
	a.bootstrapMu.Lock()
	if a.bootstrapStop != nil {
		a.bootstrapStop()
		a.bootstrapStop = nil
	}
	a.bootstrapMu.Unlock()

	a.bridgeMu.Lock()
	if a.bridgeStop != nil {
		a.bridgeStop()
		a.bridgeStop = nil
	}
	a.bridgeMu.Unlock()
}

func (a *launcherApp) applyStatus(status BridgeStatus, connectedIcon, disconnectedIcon []byte) {
	a.statusMu.Lock()
	a.status = status
	a.statusMu.Unlock()

	if status.Connected {
		systray.SetIcon(connectedIcon)
		systray.SetTooltip("BimmLite: connected")
		return
	}
	systray.SetIcon(disconnectedIcon)
	systray.SetTooltip("BimmLite: disconnected")
}

func (a *launcherApp) resolveToken() (string, string, error) {
	if envToken := strings.TrimSpace(os.Getenv("BRIDGE_SESSION_TOKEN")); envToken != "" && envToken != "change-me" {
		_ = a.storeToken(envToken)
		return envToken, "env", nil
	}

	token, err := keyring.Get(launcherKeyringService, launcherKeyringUser)
	if err == nil {
		return strings.TrimSpace(token), "keyring", nil
	}
	if errors.Is(err, keyring.ErrNotFound) || errors.Is(err, keyring.ErrUnsupportedPlatform) {
		return "", "missing", nil
	}
	return "", "keyring", err
}

func (a *launcherApp) storeToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("empty token")
	}
	return keyring.Set(launcherKeyringService, launcherKeyringUser, token)
}

func (a *launcherApp) startBridge(ctx context.Context, traceID, token string) {
	a.bridgeMu.Lock()
	defer a.bridgeMu.Unlock()
	if a.bridgeStart {
		return
	}
	a.bridgeStart = true

	cfg := a.cfg
	cfg.SessionToken = strings.TrimSpace(token)
	if cfg.SessionToken == "" {
		return
	}
	ensureWindowsFirewallRules()

	client := newBridgeClient(cfg, func(status BridgeStatus) {
		a.pushStatus(status)
		if status.Connected {
			a.logEvent("INFO", "bridge", "bridge.connected", traceID, cfg.SessionID, status.Message, nil)
			return
		}
		a.logEvent("WARN", "bridge", "bridge.disconnected", traceID, cfg.SessionID, status.Message, nil)
	})

	go func() {
		a.logEvent("INFO", "bridge", "bridge.launch", traceID, cfg.SessionID, "bridge goroutine starting", nil)
		if err := client.Run(ctx); err != nil && ctx.Err() == nil {
			a.logEvent("ERROR", "bridge", "bridge.exit", traceID, cfg.SessionID, err.Error(), nil)
		}
	}()
}

func (a *launcherApp) startBootstrapServer() (string, func(), error) {
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}

	page := template.Must(template.New("bootstrap").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>BimmLite Launcher</title>
  <style>
    body { font-family: system-ui, sans-serif; margin: 0; padding: 32px; background: #0b1220; color: #eef2ff; }
    .card { max-width: 720px; margin: 0 auto; padding: 24px; border-radius: 18px; background: rgba(255,255,255,.05); box-shadow: 0 12px 36px rgba(0,0,0,.35); }
    input, button { width: 100%; padding: 12px 14px; border-radius: 12px; border: 1px solid rgba(255,255,255,.12); box-sizing: border-box; }
    input { margin: 12px 0 16px; background: rgba(255,255,255,.08); color: #fff; }
    button { background: #2dd4bf; color: #04111c; font-weight: 700; cursor: pointer; }
    code { word-break: break-all; }
  </style>
</head>
<body>
  <div class="card">
    <h1>BimmLite launcher</h1>
    <p>Token not found. Enter a session token to save it in the system keyring and start the bridge.</p>
    <p>After saving, the browser will open the application: <code>{{.AppURL}}</code></p>
    <form method="post" action="/save">
      <label for="token">Session token</label>
      <input id="token" name="token" type="password" autocomplete="off" autofocus required>
      <button type="submit">Save token</button>
    </form>
  </div>
</body>
</html>`))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = page.Execute(w, map[string]any{"AppURL": a.appURL})
	})
	mux.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		token := strings.TrimSpace(r.FormValue("token"))
		if token == "" {
			http.Error(w, "token required", http.StatusBadRequest)
			return
		}
		if err := keyring.Set(launcherKeyringService, launcherKeyringUser, token); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.logEvent("INFO", "security", "launcher.token.saved", a.traceID, a.cfg.SessionID, "session token stored", map[string]any{"source": "bootstrap"})
		a.startBridge(a.bridgeCtx, a.traceID, token)
		_ = openURL(a.appURL)
		_, _ = w.Write([]byte("<html><body><h2>Token saved</h2><p>You can return to the application.</p></body></html>"))
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}

	go func() {
		_ = srv.Serve(listener)
	}()

	stop := func() {
		_ = srv.Close()
	}

	return "http://" + listener.Addr().String(), stop, nil
}

func (a *launcherApp) pushStatus(status BridgeStatus) {
	a.statusMu.Lock()
	a.status = status
	a.statusMu.Unlock()

	select {
	case a.statusCh <- status:
	default:
	}
}

func ensureWindowsFirewallRules() {
	if runtime.GOOS != "windows" {
		return
	}

	exePath, err := os.Executable()
	if err != nil || strings.TrimSpace(exePath) == "" {
		log.Printf("firewall rule check skipped: %v", err)
		return
	}

	rules := []struct {
		name string
		dir  string
	}{
		{name: "BimmLite UDP in", dir: "in"},
		{name: "BimmLite UDP out", dir: "out"},
	}

	for _, rule := range rules {
		if firewallRuleExists(rule.name) {
			continue
		}
		args := []string{
			"advfirewall", "firewall", "add", "rule",
			"name=" + rule.name,
			"dir=" + rule.dir,
			"action=allow",
			"program=" + exePath,
			"protocol=UDP",
			"localport=6811,6801,13400",
			"profile=private",
		}
		cmd := exec.Command("netsh", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("firewall rule setup failed rule=%s output=%s err=%v", rule.name, strings.TrimSpace(string(output)), err)
		} else {
			log.Printf("firewall rule ensured rule=%s", rule.name)
		}
	}
}

func firewallRuleExists(name string) bool {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name="+name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	text := strings.ToLower(string(output))
	return strings.Contains(text, "rule name") || strings.Contains(text, "show rule")
}

func (a *launcherApp) logDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base, _ = os.UserHomeDir()
	}
	return filepath.Join(base, "BimmLite", "logs")
}

func (a *launcherApp) logEvent(level, module, event, traceID, sessionID, message string, extra map[string]any) {
	if a.log == nil {
		return
	}
	_ = a.log.Event(level, module, event, traceID, sessionID, message, extra)
}

func newLauncherLogger() (*launcherLogger, error) {
	dir := filepath.Join(configBaseDir(), "BimmLite", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, "launcher.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &launcherLogger{file: file}, nil
}

func configBaseDir() string {
	if base, err := os.UserConfigDir(); err == nil && base != "" {
		return base
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

func (l *launcherLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

func (l *launcherLogger) Event(level, module, event, traceID, sessionID, message string, extra map[string]any) error {
	if l == nil || l.file == nil {
		return nil
	}
	entry := launcherLogEntry{
		Ts:        time.Now().UTC().Format(time.RFC3339Nano),
		Level:     strings.ToUpper(level),
		Module:    module,
		Event:     event,
		TraceID:   traceID,
		SessionID: sessionID,
		Message:   message,
	}
	if extra != nil {
		if v, ok := extra["user_id"].(string); ok {
			entry.UserID = v
		}
		if v, ok := extra["vin"].(string); ok {
			entry.VIN = v
		}
		if v, ok := extra["ecu"].(string); ok {
			entry.ECU = v
		}
		if v, ok := extra["payload_hex"].(string); ok {
			entry.PayloadHex = v
		}
		if v, ok := extra["result"].(string); ok {
			entry.Result = v
		}
		if v, ok := extra["error"].(string); ok {
			entry.Error = v
		}
		if v, ok := extra["duration_ms"].(int64); ok {
			entry.DurationMS = v
		}
		if v, ok := extra["duration_ms"].(int); ok {
			entry.DurationMS = int64(v)
		}
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = l.file.Write(append(raw, '\n'))
	return err
}

func buildStatusIcon(connected bool) []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{A: 0}}, image.Point{}, draw.Src)

	center := float64(size) / 2
	radius := float64(size) * 0.34
	outer := radius * radius
	border := radius * 0.18
	borderLine := (radius - border) * (radius - border)

	fill := color.RGBA{R: 148, G: 163, B: 184, A: 255}
	stroke := color.RGBA{R: 71, G: 85, B: 105, A: 255}
	if connected {
		fill = color.RGBA{R: 34, G: 197, B: 94, A: 255}
		stroke = color.RGBA{R: 10, G: 92, B: 38, A: 255}
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) + 0.5 - center
			dy := float64(y) + 0.5 - center
			dist := dx*dx + dy*dy
			if dist <= outer {
				if dist >= borderLine {
					img.SetRGBA(x, y, stroke)
					continue
				}
				img.SetRGBA(x, y, fill)
			}
		}
	}

	if runtime.GOOS != "windows" {
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		return buf.Bytes()
	}

	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)
	return buildWindowsICO(pngBuf.Bytes(), size, size)
}

func buildWindowsICO(pngData []byte, width, height int) []byte {
	if len(pngData) == 0 {
		return nil
	}
	if width <= 0 || height <= 0 {
		width = 32
		height = 32
	}

	var out bytes.Buffer
	out.Write([]byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00})
	out.WriteByte(byte(width))
	out.WriteByte(byte(height))
	out.WriteByte(0x00)
	out.WriteByte(0x00)
	out.Write([]byte{0x01, 0x00})
	out.Write([]byte{0x20, 0x00})
	sizeBytes := uint32(len(pngData))
	out.Write([]byte{byte(sizeBytes), byte(sizeBytes >> 8), byte(sizeBytes >> 16), byte(sizeBytes >> 24)})
	offset := uint32(6 + 16)
	out.Write([]byte{byte(offset), byte(offset >> 8), byte(offset >> 16), byte(offset >> 24)})
	out.Write(pngData)
	return out.Bytes()
}

func openURL(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return errors.New("empty url")
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", "start", "", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

func openPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("empty path")
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}
