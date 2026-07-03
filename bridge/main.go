package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bimmlite/bridge/internal/transport"
	"github.com/gorilla/websocket"
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
	WSURL           string
	SessionToken    string
	SessionID       string
	DeviceID        string
	TLSSkipVerify   bool
	HeartbeatSecond  time.Duration
	ReconnectSecond  time.Duration
}

func envDuration(key string, fallback int) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return time.Duration(fallback) * time.Second
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(parsed) * time.Second
}

func loadConfig() Config {
	return Config{
		WSURL:          getEnv("BRIDGE_WS_URL", "ws://localhost:8000/ws/bridge"),
		SessionToken:   getEnv("BRIDGE_SESSION_TOKEN", "change-me"),
		SessionID:      getEnv("BRIDGE_SESSION_ID", "bridge-local"),
		DeviceID:       getEnv("BRIDGE_DEVICE_ID", "desktop-loopback"),
		TLSSkipVerify:  strings.EqualFold(getEnv("BRIDGE_TLS_INSECURE", "true"), "true"),
		HeartbeatSecond: envDuration("BRIDGE_HEARTBEAT_SECONDS", 15),
		ReconnectSecond: envDuration("BRIDGE_RECONNECT_SECONDS", 5),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg := loadConfig()
	log.Printf("bridge starting ws_url=%s session_id=%s", cfg.WSURL, cfg.SessionID)

	client := &BridgeClient{
		cfg:       cfg,
		transport: &transport.LoopbackTransport{},
	}

	if err := client.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("bridge stopped: %v", err)
	}
}

type BridgeClient struct {
	cfg       Config
	transport transport.AdapterTransport
}

func (b *BridgeClient) Run(ctx context.Context) error {
	backoff := b.cfg.ReconnectSecond
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

		log.Printf("bridge reconnecting after error: %v", err)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil
		}
	}
}

func (b *BridgeClient) runOnce(ctx context.Context) error {
	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 10 * time.Second,
	}
	if strings.HasPrefix(b.cfg.WSURL, "wss://") && b.cfg.TLSSkipVerify {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // dev only
	}

	conn, _, err := dialer.DialContext(ctx, b.cfg.WSURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := b.transport.Connect(ctx); err != nil {
		return err
	}
	defer b.transport.Close()

	if err := b.sendAuth(ctx, conn); err != nil {
		return err
	}
	if err := b.sendLog(ctx, conn, "INFO", "bridge", "bridge.connected", "bridge connected", "", "", ""); err != nil {
		return err
	}

	heartbeatTicker := time.NewTicker(b.cfg.HeartbeatSecond)
	defer heartbeatTicker.Stop()

	readErr := make(chan error, 1)
	go func() {
		readErr <- b.readLoop(ctx, conn)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			return err
		case <-heartbeatTicker.C:
			if err := b.sendHeartbeat(ctx, conn); err != nil {
				return err
			}
		}
	}
}

func (b *BridgeClient) sendAuth(ctx context.Context, conn *websocket.Conn) error {
	envelope := Envelope{
		Version:   "1.0",
		Ts:        time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:   "bridge-auth",
		SessionID: b.cfg.SessionID,
		MsgType:   "auth",
		Payload: map[string]any{
			"token":     b.cfg.SessionToken,
			"session_id": b.cfg.SessionID,
			"device_id":  b.cfg.DeviceID,
		},
	}
	return conn.WriteJSON(envelope)
}

func (b *BridgeClient) sendHeartbeat(ctx context.Context, conn *websocket.Conn) error {
	envelope := Envelope{
		Version:   "1.0",
		Ts:        time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:   "bridge-heartbeat",
		SessionID: b.cfg.SessionID,
		MsgType:   "heartbeat",
		Payload: map[string]any{
			"status": "alive",
		},
	}
	return conn.WriteJSON(envelope)
}

func (b *BridgeClient) sendLog(ctx context.Context, conn *websocket.Conn, level, module, event, message, result, errText, payloadHex string) error {
	envelope := Envelope{
		Version:   "1.0",
		Ts:        time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:   "bridge-log",
		SessionID: b.cfg.SessionID,
		MsgType:   "log",
		Payload: map[string]any{
			"level":       level,
			"module":      module,
			"event":       event,
			"message":     message,
			"result":      result,
			"error":       errText,
			"payload_hex": payloadHex,
		},
	}
	return conn.WriteJSON(envelope)
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
			b.handleCommand(ctx, conn, envelope)
		case "heartbeat":
			log.Printf("bridge heartbeat ack trace_id=%s", envelope.TraceID)
		}
	}
}

func (b *BridgeClient) handleCommand(ctx context.Context, conn *websocket.Conn, envelope Envelope) {
	command, _ := envelope.Payload["command"].(string)
	args, _ := envelope.Payload["args"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}

	if err := b.sendLog(ctx, conn, "INFO", "bridge", "bridge.command.received", "command received", "", "", ""); err != nil {
		log.Printf("bridge log send failed: %v", err)
	}

	data, err := b.transport.Execute(ctx, command, args)
	resultPayload := map[string]any{
		"ok": err == nil,
		"data": data,
	}
	if err != nil {
		resultPayload["error"] = err.Error()
	}

	level := "INFO"
	errorText := ""
	if err != nil {
		level = "ERROR"
		errorText = err.Error()
	}

	if err := b.sendLog(ctx, conn, level, "bridge", "bridge.command.completed", "command completed", "", errorText, ""); err != nil {
		log.Printf("bridge log send failed: %v", err)
	}

	response := Envelope{
		Version:   "1.0",
		Ts:        time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:   envelope.TraceID,
		SessionID: envelope.SessionID,
		MsgType:   "result",
		Payload:   resultPayload,
	}
	if err := conn.WriteJSON(response); err != nil {
		log.Printf("bridge write result failed: %v", err)
	}
}
