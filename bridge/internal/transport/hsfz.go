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

type HSFZTransport struct {
	host   string
	port   int
	source uint8
	target uint8
	sink   FrameSink
	conn   net.Conn
	mu     sync.Mutex
}

func NewHSFZTransport(host string) *HSFZTransport {
	return &HSFZTransport{
		host:   host,
		port:   6801,
		source: 0xF4,
		target: 0x10,
	}
}

func (t *HSFZTransport) Name() string { return "hsfz" }

func (t *HSFZTransport) SetFrameSink(sink FrameSink) { t.sink = sink }

func (t *HSFZTransport) emit(frame FrameRecord) {
	if t.sink != nil {
		t.sink(frame)
	}
}

func (t *HSFZTransport) Connect(ctx context.Context) error {
	if t.conn != nil {
		return nil
	}
	if strings.TrimSpace(t.host) == "" {
		return errors.New("hsfz host not configured")
	}
	dialer := net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", t.host, t.port))
	if err != nil {
		return err
	}
	t.conn = conn
	return nil
}

func (t *HSFZTransport) Discover(ctx context.Context) (DiscoveryResult, error) {
	if err := t.Connect(ctx); err != nil {
		return DiscoveryResult{}, err
	}

	for _, target := range []uint8{0x10, 0x40, 0xDF} {
		vin, battery, err := t.readVehicleIdentity(ctx, target)
		if err != nil || vin == "" {
			continue
		}
		t.target = target
		return DiscoveryResult{
			Protocol:        t.Name(),
			IP:              t.host,
			VIN:             vin,
			BatteryVoltage:  battery,
			ECUs:            []ECUInfo{},
			TargetAddress:   formatU8(target),
			SessionControl:  "0x03",
			DiscoverySource: "uds_vin",
		}, nil
	}

	return DiscoveryResult{Protocol: t.Name(), ECUs: []ECUInfo{}}, errors.New("vin discovery failed")
}

func (t *HSFZTransport) readVehicleIdentity(ctx context.Context, target uint8) (string, float64, error) {
	resp, err := t.request(ctx, target, []byte{0x22, 0xF1, 0x90}, "connect.discover.vin")
	if err != nil {
		return "", 0, err
	}
	if len(resp) < 3 || resp[0] != 0x62 || resp[1] != 0xF1 || resp[2] != 0x90 {
		return "", 0, fmt.Errorf("unexpected VIN response: %X", resp)
	}

	vin := strings.TrimSpace(string(resp[3:]))
	battery, _ := t.readBatteryVoltage(ctx, target)
	return vin, battery, nil
}

func (t *HSFZTransport) readBatteryVoltage(ctx context.Context, target uint8) (float64, error) {
	resp, err := t.request(ctx, target, []byte{0x22, 0xDA, 0xD8}, "vehicle.battery")
	if err != nil {
		return 0, err
	}
	if len(resp) < 5 || resp[0] != 0x62 || resp[1] != 0xDA || resp[2] != 0xD8 {
		return 0, fmt.Errorf("unexpected battery response: %X", resp)
	}

	raw := uint16(resp[len(resp)-2])<<8 | uint16(resp[len(resp)-1])
	if raw >= 80 && raw <= 170 {
		return float64(raw) / 10, nil
	}
	return 0, nil
}

func (t *HSFZTransport) ScanECUs(ctx context.Context) ([]ECUInfo, error) {
	if err := t.Connect(ctx); err != nil {
		return nil, err
	}

	if ecus, err := t.broadcastScan(ctx); err == nil && len(ecus) > 0 {
		return ecus, nil
	}

	candidates := bmwEcuCandidates()
	result := make([]ECUInfo, 0, len(candidates))
	for _, addr := range candidates {
		resp, err := t.request(ctx, addr, []byte{0x10, 0x03}, "ecu.scan")
		if err != nil {
			continue
		}
		if len(resp) >= 2 && resp[0] == 0x50 && resp[1] == 0x03 {
			result = append(result, ECUInfo{
				Address:  formatU8(addr),
				Name:     bmwEcuName(addr),
				Protocol: t.Name(),
				Present:  true,
			})
		}
	}
	return result, nil
}

func (t *HSFZTransport) broadcastScan(ctx context.Context) ([]ECUInfo, error) {
	if err := t.writeFrame(0xDF, []byte{0x3E, 0x00, 0x01}, "ecu.scan.broadcast"); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(700 * time.Millisecond)
	seen := map[uint8]ECUInfo{}
	for time.Now().Before(deadline) {
		_ = t.conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		source, _, data, err := t.readFrame()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			break
		}
		if source == 0x00 || source == t.source || source == 0xDF {
			continue
		}
		if len(data) == 0 {
			continue
		}
		if _, exists := seen[source]; exists {
			continue
		}
		seen[source] = ECUInfo{
			Address:  formatU8(source),
			Name:     bmwEcuName(source),
			Protocol: t.Name(),
			Present:  true,
		}
	}

	ecus := make([]ECUInfo, 0, len(seen))
	for _, ecu := range seen {
		ecus = append(ecus, ecu)
	}
	return ecus, nil
}

func (t *HSFZTransport) ReadDTC(ctx context.Context, ecu ECUInfo) ([]DTCInfo, error) {
	addr, err := parseMaybeHexU16(ecu.Address)
	if err != nil {
		return nil, err
	}
	resp, err := t.request(ctx, uint8(addr), []byte{0x19, 0x02, 0x2F}, "dtc.read")
	if err != nil {
		return nil, err
	}
	if len(resp) < 3 || resp[0] != 0x59 || resp[1] != 0x02 {
		return nil, fmt.Errorf("unexpected DTC response: %X", resp)
	}
	return parseDTCResponse(ecu, resp[3:]), nil
}

func (t *HSFZTransport) ReadParameters(ctx context.Context, ecu ECUInfo, dids []string) ([]ParameterInfo, error) {
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
		resp, err := t.request(ctx, uint8(addr), []byte{0x22, didBytes[0], didBytes[1]}, "params.read")
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

func (t *HSFZTransport) ClearDTC(ctx context.Context, ecu ECUInfo) (map[string]any, error) {
	addr, err := parseMaybeHexU16(ecu.Address)
	if err != nil {
		return nil, err
	}
	resp, err := t.request(ctx, uint8(addr), []byte{0x14, 0xFF, 0xFF, 0xFF}, "dtc.clear")
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

func (t *HSFZTransport) TesterPresent(ctx context.Context, ecu ECUInfo) error {
	addr, err := parseMaybeHexU16(ecu.Address)
	if err != nil {
		addr = uint16(t.target)
	}
	_, err = t.request(ctx, uint8(addr), []byte{0x3E, 0x80}, "tester.present")
	return err
}

func (t *HSFZTransport) Close() error {
	if t.conn != nil {
		err := t.conn.Close()
		t.conn = nil
		return err
	}
	return nil
}

func (t *HSFZTransport) request(ctx context.Context, target uint8, uds []byte, message string) ([]byte, error) {
	if err := t.writeFrame(target, uds, message); err != nil {
		return nil, err
	}
	for {
		_ = t.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		source, _, payload, err := t.readFrame()
		if err != nil {
			return nil, err
		}
		if len(payload) >= 3 && payload[0] == 0x7F {
			switch payload[2] {
			case 0x78:
				continue
			case 0x36, 0x37:
				time.Sleep(11 * time.Second)
				continue
			default:
				return nil, fmt.Errorf("nrc 0x%02X from %02X: %X", payload[2], source, payload)
			}
		}
		return payload, nil
	}
}

func (t *HSFZTransport) writeFrame(target uint8, uds []byte, message string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return errors.New("hsfz connection not established")
	}

	body := append([]byte{t.source, target}, uds...)
	header := make([]byte, 6)
	binary.BigEndian.PutUint32(header[0:4], uint32(len(body)))
	binary.BigEndian.PutUint16(header[4:6], 0x0001)
	if _, err := t.conn.Write(header); err != nil {
		return err
	}
	if _, err := t.conn.Write(body); err != nil {
		return err
	}
	t.emit(FrameRecord{
		Protocol:  t.Name(),
		Direction: "tx",
		FrameHex:  encodeHex(append(header, body...)),
		Source:    formatU8(t.source),
		Target:    formatU8(target),
		Message:   message,
	})
	return nil
}

func (t *HSFZTransport) readFrame() (uint8, uint8, []byte, error) {
	header := make([]byte, 6)
	if _, err := io.ReadFull(t.conn, header); err != nil {
		return 0, 0, nil, err
	}
	length := binary.BigEndian.Uint32(header[0:4])
	body := make([]byte, length)
	if _, err := io.ReadFull(t.conn, body); err != nil {
		return 0, 0, nil, err
	}
	if len(body) < 2 {
		return 0, 0, nil, errors.New("hsfz body too short")
	}

	source := body[0]
	target := body[1]
	payload := body[2:]
	t.emit(FrameRecord{
		Protocol:  t.Name(),
		Direction: "rx",
		FrameHex:  encodeHex(append(header, body...)),
		Source:    formatU8(source),
		Target:    formatU8(target),
	})
	return source, target, payload, nil
}
