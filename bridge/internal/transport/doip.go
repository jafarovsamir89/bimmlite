package transport

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	doipVersion          = 0x02
	doipPayloadDiagMsg   = 0x8001
	doipPayloadRouting    = 0x0005
	doipPayloadVehicleID  = 0x0001
	doipPayloadAliveCheck = 0x0007
)

type DoIPTransport struct {
	host      string
	port      int
	source    uint16
	target    uint16
	sink      FrameSink
	conn      net.Conn
	mu        sync.Mutex
	lastSeen  time.Time
}

func NewDoIPTransport(host string) *DoIPTransport {
	return &DoIPTransport{
		host:   host,
		port:   13400,
		source: 0x0E00,
		target: 0x0001,
	}
}

func (t *DoIPTransport) Name() string { return "doip" }

func (t *DoIPTransport) SetFrameSink(sink FrameSink) { t.sink = sink }

func (t *DoIPTransport) emit(frame FrameRecord) {
	if t.sink != nil {
		t.sink(frame)
	}
}

func (t *DoIPTransport) Connect(ctx context.Context) error {
	if t.conn != nil {
		return nil
	}
	if strings.TrimSpace(t.host) == "" {
		return errors.New("doip host not configured")
	}

	dialer := net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", t.host, t.port))
	if err != nil {
		return err
	}
	t.conn = conn
	return t.routingActivation(ctx)
}

func (t *DoIPTransport) routingActivation(ctx context.Context) error {
	payload := make([]byte, 7)
	binary.BigEndian.PutUint16(payload[0:2], t.source)
	payload[2] = 0x00
	payload[3] = 0x00
	payload[4] = 0x00
	payload[5] = 0x00
	payload[6] = 0x00
	if err := t.writeMessage(ctx, doipPayloadRouting, payload, "routing.activation"); err != nil {
		return err
	}
	_, _, err := t.readMessage(ctx)
	return err
}

func (t *DoIPTransport) Discover(ctx context.Context) (DiscoveryResult, error) {
	if err := t.Connect(ctx); err != nil {
		return DiscoveryResult{}, err
	}

	vin, battery, err := t.readVehicleIdentity(ctx)
	if err != nil {
		return DiscoveryResult{Protocol: t.Name(), ECUs: []ECUInfo{}}, err
	}

	return DiscoveryResult{
		Protocol:       t.Name(),
		VIN:            vin,
		BatteryVoltage:  battery,
		ECUs:           []ECUInfo{},
		TargetAddress:  formatU16(t.target),
		SessionControl: "0x03",
	}, nil
}

func (t *DoIPTransport) readVehicleIdentity(ctx context.Context) (string, float64, error) {
	uds := []byte{0x22, 0xF1, 0x90}
	resp, err := t.request(ctx, t.target, uds, "connect.discover")
	if err != nil {
		return "", 0, err
	}
	if len(resp) < 3 || resp[0] != 0x62 || resp[1] != 0xF1 || resp[2] != 0x90 {
		return "", 0, fmt.Errorf("unexpected VIN response: %X", resp)
	}
	return strings.TrimSpace(string(resp[3:])), 0, nil
}

func (t *DoIPTransport) ScanECUs(ctx context.Context) ([]ECUInfo, error) {
	if err := t.Connect(ctx); err != nil {
		return nil, err
	}
	candidates := defaultECUCandidates()
	result := make([]ECUInfo, 0, len(candidates))
	for _, addr := range candidates {
		uds := []byte{0x10, 0x03}
		resp, err := t.request(ctx, addr, uds, "ecu.scan")
		if err != nil {
			continue
		}
		if len(resp) >= 2 && resp[0] == 0x50 && resp[1] == 0x03 {
			result = append(result, ECUInfo{
				Address:  formatU16(addr),
				Name:     "",
				Protocol: t.Name(),
				Present:  true,
			})
		}
	}
	return result, nil
}

func (t *DoIPTransport) ReadDTC(ctx context.Context, ecu ECUInfo) ([]DTCInfo, error) {
	addr, err := parseMaybeHexU16(ecu.Address)
	if err != nil {
		return nil, err
	}
	resp, err := t.request(ctx, addr, []byte{0x19, 0x02}, "dtc.read")
	if err != nil {
		return nil, err
	}
	if len(resp) < 2 || resp[0] != 0x59 || resp[1] != 0x02 {
		return nil, fmt.Errorf("unexpected DTC response: %X", resp)
	}
	return parseDTCResponse(ecu, resp[2:]), nil
}

func (t *DoIPTransport) ReadParameters(ctx context.Context, ecu ECUInfo, dids []string) ([]ParameterInfo, error) {
	addr, err := parseMaybeHexU16(ecu.Address)
	if err != nil {
		return nil, err
	}
	params := make([]ParameterInfo, 0, len(dids))
	for _, did := range dids {
		didBytes, err := decodeHex(did)
		if err != nil || len(didBytes) != 2 {
			continue
		}
		uds := []byte{0x22, didBytes[0], didBytes[1]}
		resp, err := t.request(ctx, addr, uds, "params.read")
		if err != nil {
			continue
		}
		if len(resp) < 3 || resp[0] != 0x62 || resp[1] != didBytes[0] || resp[2] != didBytes[1] {
			continue
		}
		value := resp[3:]
		params = append(params, ParameterInfo{
			ECUAddress: ecu.Address,
			ECUName:    ecu.Name,
			DID:        strings.ToUpper(did),
			ValueHex:   encodeHex(value),
			ValueText:  decodeText(value),
		})
	}
	return params, nil
}

func (t *DoIPTransport) TesterPresent(ctx context.Context, ecu ECUInfo) error {
	addr, err := parseMaybeHexU16(ecu.Address)
	if err != nil {
		addr = t.target
	}
	_, err = t.request(ctx, addr, []byte{0x3E, 0x80}, "tester.present")
	return err
}

func (t *DoIPTransport) Close() error {
	if t.conn != nil {
		err := t.conn.Close()
		t.conn = nil
		return err
	}
	return nil
}

func (t *DoIPTransport) request(ctx context.Context, target uint16, uds []byte, message string) ([]byte, error) {
	start := time.Now()
	if err := t.writeDiagnostic(ctx, target, uds, message); err != nil {
		return nil, err
	}
	for {
		_, payload, err := t.readMessage(ctx)
		if err != nil {
			return nil, err
		}
		if len(payload) < 4 {
			continue
		}
		respTarget := binary.BigEndian.Uint16(payload[0:2])
		respSource := binary.BigEndian.Uint16(payload[2:4])
		_ = respTarget
		_ = respSource
		data := payload[4:]
		if len(data) >= 3 && data[0] == 0x7F && data[2] == 0x78 {
			continue
		}
		_ = start
		return data, nil
	}
}

func (t *DoIPTransport) writeDiagnostic(ctx context.Context, target uint16, uds []byte, message string) error {
	payload := make([]byte, 4+len(uds))
	binary.BigEndian.PutUint16(payload[0:2], t.source)
	binary.BigEndian.PutUint16(payload[2:4], target)
	copy(payload[4:], uds)
	return t.writeMessage(ctx, doipPayloadDiagMsg, payload, message)
}

func (t *DoIPTransport) writeMessage(ctx context.Context, payloadType uint16, payload []byte, message string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return errors.New("doip connection not established")
	}

	header := make([]byte, 8)
	header[0] = doipVersion
	header[1] = ^doipVersion
	binary.BigEndian.PutUint16(header[2:4], payloadType)
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	if _, err := t.conn.Write(header); err != nil {
		return err
	}
	if _, err := t.conn.Write(payload); err != nil {
		return err
	}
	t.emit(FrameRecord{
		Protocol:  t.Name(),
		Direction: "tx",
		FrameHex:  encodeHex(append(header, payload...)),
		Source:    formatU16(t.source),
		Target:    formatU16(t.target),
		Message:   message,
	})
	return nil
}

func (t *DoIPTransport) readMessage(ctx context.Context) (uint16, []byte, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(t.conn, header); err != nil {
		return 0, nil, err
	}
	length := binary.BigEndian.Uint32(header[4:8])
	payload := make([]byte, length)
	if _, err := io.ReadFull(t.conn, payload); err != nil {
		return 0, nil, err
	}
	t.emit(FrameRecord{
		Protocol:  t.Name(),
		Direction: "rx",
		FrameHex:  encodeHex(append(header, payload...)),
		Source:    formatU16(t.source),
		Target:    formatU16(t.target),
	})
	return binary.BigEndian.Uint16(header[2:4]), payload, nil
}

func defaultECUCandidates() []uint16 {
	return []uint16{0x7E0, 0x7E1, 0x7E2, 0x7E3, 0x7E4, 0x7E5, 0x7E6, 0x7E7, 0x7E8, 0x7E9, 0x7EA, 0x7EB, 0x7EC, 0x7ED, 0x7EE, 0x7EF}
}

func parseDTCResponse(ecu ECUInfo, raw []byte) []DTCInfo {
	if len(raw) == 0 {
		return nil
	}
	var result []DTCInfo
	for i := 0; i+3 < len(raw); i += 4 {
		code := fmt.Sprintf("%02X%02X%02X", raw[i], raw[i+1], raw[i+2])
		status := fmt.Sprintf("%02X", raw[i+3])
		result = append(result, DTCInfo{
			ECUAddress:  ecu.Address,
			ECUName:     ecu.Name,
			Code:        code,
			Status:      status,
			Description: "",
			Raw:         encodeHex(raw[i : i+4]),
		})
	}
	return result
}

func decodeText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	b := make([]byte, 0, len(raw))
	for _, c := range raw {
		if c >= 0x20 && c <= 0x7E {
			b = append(b, c)
		}
	}
	if len(b) == 0 {
		return ""
	}
	return string(b)
}
