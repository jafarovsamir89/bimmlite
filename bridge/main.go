package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"bimmlite/bridge/internal/transport"
	"github.com/gorilla/websocket"
)

type Mode string

const (
	ModeLauncher Mode = "launcher"
	ModeBridge   Mode = "bridge"
)

type Envelope struct {
	Version   string         `json:"version"`
	Ts        string         `json:"ts"`
	TraceID   string         `json:"trace_id"`
	SessionID string         `json:"session_id"`
	MsgType   string         `json:"msg_type"`
	Payload   map[string]any `json:"payload"`
}

type Config struct {
	WSURL                 string
	SessionToken          string
	SessionID             string
	DeviceID              string
	TLSSkipVerify         bool
	HeartbeatInterval     time.Duration
	ReconnectInterval     time.Duration
	TesterPresentInterval time.Duration
}

func main() {
	if detectMode() == ModeBridge {
		runBridgeCLI()
		return
	}
	runLauncherCLI()
}

func runBridgeCLI() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg := loadConfig()
	client := newBridgeClient(cfg, nil)

	log.Printf("bridge starting ws_url=%s session_id=%s", cfg.WSURL, cfg.SessionID)
	if err := client.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("bridge stopped: %v", err)
	}
}

func loadConfig() Config {
	return Config{
		WSURL:                 getEnv("BRIDGE_WS_URL", "ws://localhost:8000/ws/bridge"),
		SessionToken:          getEnv("BRIDGE_SESSION_TOKEN", "change-me"),
		SessionID:             getEnv("BRIDGE_SESSION_ID", "bridge-local"),
		DeviceID:              getEnv("BRIDGE_DEVICE_ID", "desktop-readonly"),
		TLSSkipVerify:         strings.EqualFold(getEnv("BRIDGE_TLS_INSECURE", "true"), "true"),
		HeartbeatInterval:     envDuration("BRIDGE_HEARTBEAT_SECONDS", 15),
		ReconnectInterval:     envDuration("BRIDGE_RECONNECT_SECONDS", 5),
		TesterPresentInterval: envDuration("BRIDGE_TESTER_PRESENT_SECONDS", 15),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback int) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return time.Duration(fallback) * time.Second
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(parsed) * time.Second
}

type BridgeClient struct {
	cfg              Config
	transport        transport.AdapterTransport
	writeMu          sync.Mutex
	transportMu      sync.Mutex
	keepaliveStarted bool
	statusSink       func(BridgeStatus)
}

type BridgeStatus struct {
	Connected     bool
	LastHeartbeat time.Time
	Message       string
}

func newBridgeClient(cfg Config, statusSink func(BridgeStatus)) *BridgeClient {
	return &BridgeClient{
		cfg:        cfg,
		transport:  transport.NewAutoTransportFromEnv(),
		statusSink: statusSink,
	}
}

func (b *BridgeClient) notifyStatus(connected bool, message string) {
	if b.statusSink == nil {
		return
	}
	heartbeat := time.Time{}
	if connected {
		heartbeat = time.Now().UTC()
	}
	b.statusSink(BridgeStatus{
		Connected:     connected,
		LastHeartbeat: heartbeat,
		Message:       message,
	})
}

func (b *BridgeClient) Run(ctx context.Context) error {
	backoff := b.cfg.ReconnectInterval
	if backoff <= 0 {
		backoff = 5 * time.Second
	}

	for {
		if ctx.Err() != nil {
			return nil
		}
		err := b.runOnce(ctx)
		if err == nil || ctx.Err() != nil {
			return err
		}
		b.notifyStatus(false, err.Error())
		log.Printf("bridge reconnecting after error: %v", err)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil
		}
	}
}

func (b *BridgeClient) runOnce(ctx context.Context) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	b.transportMu.Lock()
	b.keepaliveStarted = false
	b.transportMu.Unlock()

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 10 * time.Second,
	}
	if strings.HasPrefix(strings.ToLower(b.cfg.WSURL), "wss://") && b.cfg.TLSSkipVerify {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // dev only; prod must disable
	}

	conn, _, err := dialer.DialContext(ctx, b.cfg.WSURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := b.sendEnvelope(conn, Envelope{
		Version:   "1.0",
		Ts:        time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:   newTraceID(),
		SessionID: b.cfg.SessionID,
		MsgType:   "auth",
		Payload: map[string]any{
			"token":      b.cfg.SessionToken,
			"session_id": b.cfg.SessionID,
			"device_id":  b.cfg.DeviceID,
		},
	}); err != nil {
		return err
	}
	b.notifyStatus(true, "bridge authenticated")
	defer b.transport.Close()

	heartbeatTicker := time.NewTicker(b.cfg.HeartbeatInterval)
	defer heartbeatTicker.Stop()

	readErr := make(chan error, 1)
	go func() {
		readErr <- b.readLoop(sessionCtx, conn)
	}()

	for {
		select {
		case <-sessionCtx.Done():
			return nil
		case err := <-readErr:
			return err
		case <-heartbeatTicker.C:
			if err := b.sendEnvelope(conn, Envelope{
				Version:   "1.0",
				Ts:        time.Now().UTC().Format(time.RFC3339Nano),
				TraceID:   newTraceID(),
				SessionID: b.cfg.SessionID,
				MsgType:   "heartbeat",
				Payload: map[string]any{
					"status": "alive",
				},
			}); err != nil {
				return err
			}
		}
	}
}

func (b *BridgeClient) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var envelope Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			log.Printf("bridge received invalid message: %v", err)
			continue
		}

		switch envelope.MsgType {
		case "command":
			if err := b.handleCommand(ctx, conn, envelope); err != nil {
				log.Printf("bridge command failed: %v", err)
			}
		case "heartbeat":
			log.Printf("bridge heartbeat ack trace_id=%s", envelope.TraceID)
			b.notifyStatus(true, "heartbeat acknowledged")
		}
	}
}

func (b *BridgeClient) handleCommand(ctx context.Context, conn *websocket.Conn, envelope Envelope) error {
	command, _ := envelope.Payload["command"].(string)
	args, _ := envelope.Payload["args"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}

	if err := b.sendLog(conn, envelope.TraceID, envelope.SessionID, "INFO", "bridge", "bridge.command.received", "command received", "", "", ""); err != nil {
		return err
	}

	var result map[string]any
	var opErr error

	switch command {
	case "ping":
		result = map[string]any{"result": "pong"}
	case "echo":
		result = map[string]any{"echo": args}
	default:
		result, opErr = b.executeTransport(ctx, conn, envelope.TraceID, envelope.SessionID, command, args)
	}

	payload := map[string]any{
		"ok":   opErr == nil,
		"data": result,
	}
	if opErr != nil {
		payload["error"] = opErr.Error()
	}

	if err := b.sendLog(conn, envelope.TraceID, envelope.SessionID, func() string {
		if opErr != nil {
			return "ERROR"
		}
		return "INFO"
	}(), "bridge", "bridge.command.completed", "command completed", "", func() string {
		if opErr != nil {
			return opErr.Error()
		}
		return ""
	}(), ""); err != nil {
		return err
	}

	return b.sendEnvelope(conn, Envelope{
		Version:   "1.0",
		Ts:        time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:   envelope.TraceID,
		SessionID: envelope.SessionID,
		MsgType:   "result",
		Payload:   payload,
	})
}

func (b *BridgeClient) executeTransport(ctx context.Context, conn *websocket.Conn, traceID, sessionID, command string, args map[string]any) (map[string]any, error) {
	b.transportMu.Lock()
	defer b.transportMu.Unlock()

	b.transport.SetFrameSink(func(frame transport.FrameRecord) {
		_ = b.sendFrame(conn, sessionID, traceID, frame)
	})

	switch command {
	case "connect.discover":
		if err := b.sendLog(conn, traceID, sessionID, "INFO", "bridge", "connect.discover.start", "starting vehicle discovery", "", "", ""); err != nil {
			return nil, err
		}
		discovery, err := b.transport.Discover(ctx)
		if err != nil {
			return nil, err
		}
		if !b.keepaliveStarted {
			b.keepaliveStarted = true
			go b.keepAliveLoop(ctx, conn, traceID, sessionID)
		}
		if discovery.VIN != "" {
			_ = b.sendLogWithFields(conn, traceID, sessionID, "INFO", "bridge", "connect.discover.found", fmt.Sprintf("vehicle discovered at %s via %s", discovery.IP, discovery.Protocol), discovery.Protocol, "", "", map[string]any{
				"vin": discovery.VIN,
			})
		}
		return map[string]any{
			"protocol":         discovery.Protocol,
			"ip":               discovery.IP,
			"vin":              discovery.VIN,
			"battery_voltage":  discovery.BatteryVoltage,
			"target_address":   discovery.TargetAddress,
			"ecus":             discovery.ECUs,
			"discovery_source": discovery.DiscoverySource,
		}, nil
	case "ecu.scan":
		ecus, err := b.transport.ScanECUs(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ecus": ecus}, nil
	case "dtc.read":
		ecu := transport.ECUInfo{
			Address:  stringFrom(args["ecu_address"]),
			Name:     stringFrom(args["ecu_name"]),
			Protocol: b.transport.Name(),
			Present:  true,
		}
		dtcs, err := b.transport.ReadDTC(ctx, ecu)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ecu": ecu, "dtcs": dtcs}, nil
	case "params.read":
		ecu := transport.ECUInfo{
			Address:  stringFrom(args["ecu_address"]),
			Name:     stringFrom(args["ecu_name"]),
			Protocol: b.transport.Name(),
			Present:  true,
		}
		dids := stringSlice(args["dids"])
		params, err := b.transport.ReadParameters(ctx, ecu, dids)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ecu": ecu, "parameters": params}, nil
	case "tester.present":
		ecu := transport.ECUInfo{
			Address:  stringFrom(args["ecu_address"]),
			Name:     stringFrom(args["ecu_name"]),
			Protocol: b.transport.Name(),
			Present:  true,
		}
		if err := b.transport.TesterPresent(ctx, ecu); err != nil {
			return nil, err
		}
		return map[string]any{"result": "tester-present"}, nil
	default:
		return nil, &unsupportedCommandError{command: command}
	}
}

func stringFrom(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		return strings.TrimSpace(strings.Trim(fmt.Sprintf("%v", v), "[]"))
	}
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			result = append(result, stringFrom(item))
		}
		return result
	case nil:
		return nil
	default:
		return []string{stringFrom(v)}
	}
}

func (b *BridgeClient) sendEnvelope(conn *websocket.Conn, envelope Envelope) error {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	return conn.WriteJSON(envelope)
}

func (b *BridgeClient) sendLog(conn *websocket.Conn, traceID, sessionID, level, module, event, message, result, errorText, payloadHex string) error {
	return b.sendLogWithFields(conn, traceID, sessionID, level, module, event, message, result, errorText, payloadHex, nil)
}

func (b *BridgeClient) sendLogWithFields(conn *websocket.Conn, traceID, sessionID, level, module, event, message, result, errorText, payloadHex string, extra map[string]any) error {
	if conn == nil {
		return nil
	}
	payload := map[string]any{
		"level":       level,
		"module":      module,
		"event":       event,
		"message":     message,
		"result":      result,
		"error":       errorText,
		"payload_hex": payloadHex,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return b.sendEnvelope(conn, Envelope{
		Version:   "1.0",
		Ts:        time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:   traceID,
		SessionID: sessionID,
		MsgType:   "log",
		Payload:   payload,
	})
}

func (b *BridgeClient) sendFrame(conn *websocket.Conn, sessionID, traceID string, frame transport.FrameRecord) error {
	return b.sendEnvelope(conn, Envelope{
		Version:   "1.0",
		Ts:        time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:   traceID,
		SessionID: sessionID,
		MsgType:   "frame",
		Payload: map[string]any{
			"protocol":   frame.Protocol,
			"direction":  frame.Direction,
			"frame_hex":  frame.FrameHex,
			"source":     frame.Source,
			"target":     frame.Target,
			"service_id": frame.ServiceID,
			"nrc":        frame.NRC,
			"rtt_ms":     frame.RTTMS,
			"message":    frame.Message,
			"metadata":   frame.Metadata,
		},
	})
}

func (b *BridgeClient) keepAliveLoop(ctx context.Context, conn *websocket.Conn, traceID, sessionID string) {
	ticker := time.NewTicker(b.cfg.TesterPresentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.transportMu.Lock()
			err := b.transport.TesterPresent(ctx, transport.ECUInfo{})
			b.transportMu.Unlock()
			if err != nil {
				_ = b.sendLog(conn, traceID, sessionID, "WARN", "bridge", "bridge.keepalive.failed", "tester present failed", "", err.Error(), "")
				continue
			}
			_ = b.sendLog(conn, traceID, sessionID, "DEBUG", "bridge", "bridge.keepalive.sent", "tester present sent", "", "", "")
		}
	}
}

type unsupportedCommandError struct {
	command string
}

func (e *unsupportedCommandError) Error() string {
	return "unsupported command: " + e.command
}

func newTraceID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("bridge-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
