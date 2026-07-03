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
	source uint16
	target uint16
	sink   FrameSink
	conn   net.Conn
	mu     sync.Mutex
}

func NewHSFZTransport(host string) *HSFZTransport {
	return &HSFZTransport{
		host:   host,
		port:   6801,
		source: 0xF4,
		target: 0x01,
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
	resp, err := t.request(ctx, t.target, []byte{0x22, 0xF1, 0x90}, "connect.discover")
	if err != nil {
		return DiscoveryResult{Protocol: t.Name(), ECUs: []ECUInfo{}}, err
	}
	vin := ""
	if len(resp) >= 3 && resp[0] == 0x62 && resp[1] == 0xF1 && resp[2] == 0x90 {
		vin = strings.TrimSpace(string(resp[3:]))
	}
	return DiscoveryResult{
		Protocol:      t.Name(),
		VIN:           vin,
		BatteryVoltage: 0,
		ECUs:          []ECUInfo{},
		TargetAddress: formatU16(t.target),
		SessionControl: "0x03",
	}, nil
}

func (t *HSFZTransport) ScanECUs(ctx context.Context) ([]ECUInfo, error) {
	if err := t.Connect(ctx); err != nil {
		return nil, err
	}
	result := make([]ECUInfo, 0)
	for _, addr := range defaultECUCandidates() {
		resp, err := t.request(ctx, addr, []byte{0x10, 0x03}, "ecu.scan")
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

func (t *HSFZTransport) ReadDTC(ctx context.Context, ecu ECUInfo) ([]DTCInfo, error) {
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

func (t *HSFZTransport) TesterPresent(ctx context.Context, ecu ECUInfo) error {
	addr, err := parseMaybeHexU16(ecu.Address)
	if err != nil {
		addr = t.target
	}
	_, err = t.request(ctx, addr, []byte{0x3E, 0x80}, "tester.present")
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

func (t *HSFZTransport) request(ctx context.Context, target uint16, uds []byte, message string) ([]byte, error) {
	start := time.Now()
	if err := t.writeFrame(target, uds, message); err != nil {
		return nil, err
	}
	_ = start
	for {
		_, payload, err := t.readFrame()
		if err != nil {
			return nil, err
		}
		if len(payload) >= 3 && payload[0] == 0x7F && payload[2] == 0x78 {
			continue
		}
		return payload, nil
	}
}

func (t *HSFZTransport) writeFrame(target uint16, uds []byte, message string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return errors.New("hsfz connection not established")
	}
	body := make([]byte, 4+len(uds))
	binary.BigEndian.PutUint16(body[0:2], t.source)
	binary.BigEndian.PutUint16(body[2:4], target)
	copy(body[4:], uds)
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
		Source:    formatU16(t.source),
		Target:    formatU16(target),
		Message:   message,
	})
	return nil
}

func (t *HSFZTransport) readFrame() (uint16, []byte, error) {
	header := make([]byte, 6)
	if _, err := io.ReadFull(t.conn, header); err != nil {
		return 0, nil, err
	}
	length := binary.BigEndian.Uint32(header[0:4])
	body := make([]byte, length)
	if _, err := io.ReadFull(t.conn, body); err != nil {
		return 0, nil, err
	}
	t.emit(FrameRecord{
		Protocol:  t.Name(),
		Direction: "rx",
		FrameHex:  encodeHex(append(header, body...)),
		Source:    formatU16(t.source),
		Target:    formatU16(t.target),
	})
	if len(body) < 4 {
		return 0, nil, errors.New("hsfz body too short")
	}
	return binary.BigEndian.Uint16(body[2:4]), body[4:], nil
}
