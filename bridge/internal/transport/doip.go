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
	doipPayloadRouting   = 0x0005
	doipPayloadVehicleID = 0x0001
	doipPayloadVinResp   = 0x0004
)

type DoIPTransport struct {
	host   string
	port   int
	source uint16
	target uint16
	sink   FrameSink
	conn   net.Conn
	mu     sync.Mutex
}

func NewDoIPTransport(host string) *DoIPTransport {
	return &DoIPTransport{
		host:   host,
		port:   13400,
		source: 0x0E00,
		target: 0x0010,
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
	// Activation type and reserved bytes stay zero for a basic read-only session.
	if err := t.writeMessage(ctx, doipPayloadRouting, payload, "routing.activation", t.source, 0); err != nil {
		return err
	}

	msgType, resp, err := t.readMessage(ctx)
	if err != nil {
		return err
	}
	if msgType != 0x0006 {
		return fmt.Errorf("unexpected routing activation response type 0x%04X", msgType)
	}
	if len(resp) == 0 || resp[0] != 0x10 {
		return fmt.Errorf("routing activation failed: %X", resp)
	}
	return nil
}

func (t *DoIPTransport) Discover(ctx context.Context) (DiscoveryResult, error) {
	if err := t.Connect(ctx); err != nil {
		return DiscoveryResult{}, err
	}

	vin, battery, err := t.identifyVehicle(ctx)
	if err != nil {
		return DiscoveryResult{Protocol: t.Name(), ECUs: []ECUInfo{}}, err
	}
	return DiscoveryResult{
		Protocol:        t.Name(),
		IP:              t.host,
		VIN:             vin,
		BatteryVoltage:  battery,
		ECUs:            []ECUInfo{},
		TargetAddress:   formatU16(t.target),
		SessionControl:  "0x03",
		DiscoverySource: "doip_vehicle_identification",
	}, nil
}

func (t *DoIPTransport) identifyVehicle(ctx context.Context) (string, float64, error) {
	msgType, resp, err := t.requestVehicleIdentification(ctx)
	if err != nil {
		return "", 0, err
	}
	if msgType != doipPayloadVinResp || len(resp) < 19 {
		return "", 0, fmt.Errorf("unexpected vehicle id response type 0x%04X payload=%X", msgType, resp)
	}

	vin := strings.TrimSpace(string(resp[:17]))
	logicalAddress := binary.BigEndian.Uint16(resp[17:19])
	if logicalAddress != 0 {
		t.target = logicalAddress
	}

	battery, _ := t.readBatteryVoltage(ctx, t.target)
	return vin, battery, nil
}

func (t *DoIPTransport) requestVehicleIdentification(ctx context.Context) (uint16, []byte, error) {
	if err := t.writeMessage(ctx, doipPayloadVehicleID, nil, "connect.discover.identify", 0, 0); err != nil {
		return 0, nil, err
	}
	msgType, payload, err := t.readMessage(ctx)
	return msgType, payload, err
}

func (t *DoIPTransport) readBatteryVoltage(ctx context.Context, target uint16) (float64, error) {
	resp, err := t.request(ctx, target, []byte{0x22, 0xDA, 0xD8}, "vehicle.battery")
	if err == nil && len(resp) >= 5 && resp[0] == 0x62 && resp[1] == 0xDA && resp[2] == 0xD8 {
		raw := uint16(resp[len(resp)-2])<<8 | uint16(resp[len(resp)-1])
		if raw >= 80 && raw <= 170 {
			return float64(raw) / 10, nil
		}
	}

	resp, err = t.request(ctx, target, []byte{0x22, 0xF1, 0x01}, "vehicle.battery.fallback")
	if err != nil || len(resp) < 5 || resp[0] != 0x62 || resp[1] != 0xF1 || resp[2] != 0x01 {
		return 0, nil
	}
	return parseBCDVoltage(resp[3:]), nil
}

func (t *DoIPTransport) ScanECUs(ctx context.Context) ([]ECUInfo, error) {
	if err := t.Connect(ctx); err != nil {
		return nil, err
	}

	result := make([]ECUInfo, 0, len(bmwEcuCandidates()))
	for _, addr := range bmwEcuCandidates() {
		resp, err := t.request(ctx, uint16(addr), []byte{0x10, 0x03}, "ecu.scan")
		if err != nil {
			continue
		}
		if len(resp) >= 2 && resp[0] == 0x50 && resp[1] == 0x03 {
			result = append(result, ECUInfo{
				Address:  formatU16(uint16(addr)),
				Name:     bmwEcuName(addr),
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
	resp, err := t.request(ctx, addr, []byte{0x19, 0x02, 0x2F}, "dtc.read")
	if err != nil {
		return nil, err
	}
	if len(resp) < 3 || resp[0] != 0x59 || resp[1] != 0x02 {
		return nil, fmt.Errorf("unexpected DTC response: %X", resp)
	}
	return parseDTCResponse(ecu, resp[3:]), nil
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
		resp, err := t.request(ctx, addr, []byte{0x22, didBytes[0], didBytes[1]}, "params.read")
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

func (t *DoIPTransport) ClearDTC(ctx context.Context, ecu ECUInfo) (map[string]any, error) {
	addr, err := parseMaybeHexU16(ecu.Address)
	if err != nil {
		return nil, err
	}
	resp, err := t.request(ctx, addr, []byte{0x14, 0xFF, 0xFF, 0xFF}, "dtc.clear")
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 || resp[0] != 0x54 {
		return nil, fmt.Errorf("unexpected clear DTC response: %X", resp)
	}
	return map[string]any{
		"result": "cleared",
		"raw":    encodeHex(resp),
	}, nil
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
	msgType, payload, err := t.sendDiagnostic(ctx, target, uds, message)
	if err != nil {
		return nil, err
	}
	if msgType != doipPayloadDiagMsg {
		return nil, fmt.Errorf("unexpected message type 0x%04X", msgType)
	}
	if len(payload) < 4 {
		return nil, errors.New("diagnostic payload too short")
	}
	data := payload[4:]
	for len(data) >= 3 && data[0] == 0x7F {
		switch data[2] {
		case 0x78:
			msgType, payload, err = t.readMessage(ctx)
			if err != nil {
				return nil, err
			}
			if msgType != doipPayloadDiagMsg || len(payload) < 4 {
				return nil, fmt.Errorf("unexpected follow-up payload: type=0x%04X data=%X", msgType, payload)
			}
			data = payload[4:]
		case 0x36, 0x37:
			time.Sleep(11 * time.Second)
			msgType, payload, err = t.readMessage(ctx)
			if err != nil {
				return nil, err
			}
			if msgType != doipPayloadDiagMsg || len(payload) < 4 {
				return nil, fmt.Errorf("unexpected follow-up payload: type=0x%04X data=%X", msgType, payload)
			}
			data = payload[4:]
		default:
			return nil, fmt.Errorf("nrc 0x%02X: %X", data[2], data)
		}
	}
	return data, nil
}

func (t *DoIPTransport) sendDiagnostic(ctx context.Context, target uint16, uds []byte, message string) (uint16, []byte, error) {
	payload := make([]byte, 4+len(uds))
	binary.BigEndian.PutUint16(payload[0:2], t.source)
	binary.BigEndian.PutUint16(payload[2:4], target)
	copy(payload[4:], uds)
	if err := t.writeMessage(ctx, doipPayloadDiagMsg, payload, message, t.source, target); err != nil {
		return 0, nil, err
	}
	return doipPayloadDiagMsg, payload, nil
}

func (t *DoIPTransport) writeMessage(ctx context.Context, payloadType uint16, payload []byte, message string, source uint16, target uint16) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return errors.New("doip connection not established")
	}

	header := make([]byte, 8)
	header[0] = doipVersion
	header[1] = byte(^byte(doipVersion))
	binary.BigEndian.PutUint16(header[2:4], payloadType)
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	if _, err := t.conn.Write(header); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := t.conn.Write(payload); err != nil {
			return err
		}
	}
	t.emit(FrameRecord{
		Protocol:  t.Name(),
		Direction: "tx",
		FrameHex:  encodeHex(append(header, payload...)),
		Source:    formatU16(source),
		Target:    formatU16(target),
		Message:   message,
	})
	return nil
}

func (t *DoIPTransport) readMessage(ctx context.Context) (uint16, []byte, error) {
	header := make([]byte, 8)
	if err := setConnDeadline(t.conn, time.Now().Add(3*time.Second)); err != nil {
		return 0, nil, err
	}
	if _, err := io.ReadFull(t.conn, header); err != nil {
		return 0, nil, err
	}
	length := binary.BigEndian.Uint32(header[4:8])
	payload := make([]byte, length)
	if _, err := io.ReadFull(t.conn, payload); err != nil {
		return 0, nil, err
	}

	var source, target uint16
	if len(payload) >= 4 {
		source = binary.BigEndian.Uint16(payload[0:2])
		target = binary.BigEndian.Uint16(payload[2:4])
	}
	t.emit(FrameRecord{
		Protocol:  t.Name(),
		Direction: "rx",
		FrameHex:  encodeHex(append(header, payload...)),
		Source:    formatU16(source),
		Target:    formatU16(target),
	})
	return binary.BigEndian.Uint16(header[2:4]), payload, nil
}

func setConnDeadline(conn net.Conn, deadline time.Time) error {
	if conn == nil {
		return errors.New("connection not established")
	}
	return conn.SetReadDeadline(deadline)
}

func parseBCDVoltage(raw []byte) float64 {
	if len(raw) == 0 {
		return 0
	}
	value := 0.0
	multiplier := 1.0
	for i := len(raw) - 1; i >= 0; i-- {
		hi := int((raw[i] >> 4) & 0x0F)
		lo := int(raw[i] & 0x0F)
		value += float64(lo) * multiplier
		multiplier *= 10
		value += float64(hi) * multiplier
		multiplier *= 10
	}
	if value > 0 {
		return value / 10
	}
	return 0
}
