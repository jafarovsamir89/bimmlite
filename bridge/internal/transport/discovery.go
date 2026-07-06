package transport

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	zgwSearchPacket  = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x11}
	doipSearchPacket = []byte{0x02, 0xFD, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}
)

type vehicleCandidate struct {
	IP              string
	VIN             string
	Protocol        string
	DiscoverySource string
}

type DiscoveryEvent struct {
	Ts      string
	Level   string
	Event   string
	Message string
	Fields  map[string]any
}

type DiscoveryObserver func(DiscoveryEvent)

var (
	discoveryObserverMu sync.RWMutex
	discoveryObserver   DiscoveryObserver
)

func SetDiscoveryObserver(observer DiscoveryObserver) func() {
	discoveryObserverMu.Lock()
	previous := discoveryObserver
	discoveryObserver = observer
	discoveryObserverMu.Unlock()

	return func() {
		discoveryObserverMu.Lock()
		discoveryObserver = previous
		discoveryObserverMu.Unlock()
	}
}

func emitDiscovery(level, event, message string, fields map[string]any) {
	discoveryObserverMu.RLock()
	observer := discoveryObserver
	discoveryObserverMu.RUnlock()
	if observer == nil {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	observer(DiscoveryEvent{
		Ts:      time.Now().UTC().Format(time.RFC3339Nano),
		Level:   level,
		Event:   event,
		Message: message,
		Fields:  fields,
	})
}

type localInterfaceEntry struct {
	Name      string
	IP        string
	Broadcast string
	IsAny     bool
}

type discoveryProbe struct {
	Addr     *net.UDPAddr
	Payload  []byte
	Protocol string
	Label    string
}

func discoverVehicle(ctx context.Context) (vehicleCandidate, error) {
	interfaces, err := localIPv4Addrs()
	if err != nil {
		return vehicleCandidate{}, err
	}

	var candidates []vehicleCandidate
	seen := map[string]struct{}{}
	for _, iface := range interfaces {
		emitDiscovery("DEBUG", "discover.iface", "probing interface", map[string]any{
			"iface":     iface.Name,
			"ip":        iface.IP,
			"broadcast": iface.Broadcast,
			"is_any":    iface.IsAny,
		})
		found, err := discoverOnInterface(ctx, iface)
		if err != nil {
			emitDiscovery("WARN", "discover.iface.error", "interface probe failed", map[string]any{
				"iface":     iface.Name,
				"ip":        iface.IP,
				"broadcast": iface.Broadcast,
				"error":     err.Error(),
			})
			continue
		}
		for _, candidate := range found {
			key := candidate.VIN + "|" + candidate.IP
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) == 0 {
		emitDiscovery("INFO", "discover.empty", "no vehicle discovery responses received", nil)
		return vehicleCandidate{}, errors.New("no BMW vehicle discovered on ENET/DoIP")
	}

	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if best.Protocol == "" && candidate.Protocol != "" {
			best = candidate
			continue
		}
	}

	if best.Protocol == "" {
		if protocol, err := detectProtocol(ctx, best.IP); err == nil {
			best.Protocol = protocol
		}
	}
	if best.Protocol == "" {
		best.Protocol = "hsfz"
	}
	if isDiscoveryObserverActive() {
		probeDiagnosticPorts(ctx, best)
	}
	emitDiscovery("INFO", "discover.found", "vehicle discovered", map[string]any{
		"ip":               best.IP,
		"vin":              best.VIN,
		"protocol":         best.Protocol,
		"discovery_source": best.DiscoverySource,
	})
	return best, nil
}

func isDiscoveryObserverActive() bool {
	discoveryObserverMu.RLock()
	defer discoveryObserverMu.RUnlock()
	return discoveryObserver != nil
}

func localIPv4Addrs() ([]localInterfaceEntry, error) {
	netIfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	result := []localInterfaceEntry{{
		Name:      "any",
		IP:        "0.0.0.0",
		Broadcast: "255.255.255.255",
		IsAny:     true,
	}}
	for _, iface := range netIfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			value := ip.String()
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, localInterfaceEntry{
				Name:      iface.Name,
				IP:        value,
				Broadcast: broadcastFor(ipNet),
			})
		}
	}
	return result, nil
}

func discoverOnInterface(ctx context.Context, iface localInterfaceEntry) ([]vehicleCandidate, error) {
	listenConfig := net.ListenConfig{
		Control: func(_, _ string, rawConn syscall.RawConn) error {
			var controlErr error
			if err := rawConn.Control(func(fd uintptr) {
				controlErr = enableBroadcast(fd)
			}); err != nil {
				return err
			}
			return controlErr
		},
	}
	packetConn, err := listenConfig.ListenPacket(ctx, "udp4", net.JoinHostPort(iface.IP, "0"))
	if err != nil {
		return nil, err
	}
	defer packetConn.Close()
	conn := packetConn.(*net.UDPConn)

	if err := conn.SetWriteBuffer(4096); err != nil {
		return nil, err
	}
	if err := conn.SetReadBuffer(8192); err != nil {
		return nil, err
	}

	targets := discoveryTargetsForProbe(iface)
	deadline := time.Now().Add(3 * time.Second)
	sentCount := 0
	rxCount := 0
	for _, target := range targets {
		_ = conn.SetWriteDeadline(time.Now().Add(300 * time.Millisecond))
		_, err := conn.WriteToUDP(target.Payload, target.Addr)
		sentCount++
		emitDiscovery("TRACE", "discover.tx", "udp packet sent", map[string]any{
			"iface":     iface.Name,
			"ip":        iface.IP,
			"broadcast": iface.Broadcast,
			"target":    target.Addr.IP.String(),
			"port":      target.Addr.Port,
			"len":       len(target.Payload),
			"hex":       strings.ToUpper(hex.EncodeToString(target.Payload)),
			"protocol":  target.Protocol,
			"label":     target.Label,
			"is_any":    iface.IsAny,
			"error": func() string {
				if err != nil {
					return err.Error()
				}
				return ""
			}(),
		})
	}

	var result []vehicleCandidate
	seen := map[string]struct{}{}
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		buf := make([]byte, 2048)
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			break
		}
		packet := buf[:n]
		rxCount++
		emitDiscovery("TRACE", "discover.rx", "udp packet received", map[string]any{
			"iface":     iface.Name,
			"ip":        iface.IP,
			"broadcast": iface.Broadcast,
			"from":      addr.IP.String(),
			"port":      addr.Port,
			"len":       n,
			"hex":       strings.ToUpper(hex.EncodeToString(packet)),
			"is_any":    iface.IsAny,
		})
		if candidate, ok := parseBMWDiscoveryResponse(packet, addr.IP.String()); ok {
			candidate.Protocol = "hsfz"
			key := candidate.VIN + "|" + candidate.IP
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				result = append(result, candidate)
			}
			continue
		}
		if candidate, ok := parseDoIPDiscoveryResponse(packet, addr.IP.String()); ok {
			key := candidate.VIN + "|" + candidate.IP
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				result = append(result, candidate)
			}
			continue
		}
		if isDiagnosticPattern(packet) {
			emitDiscovery("TRACE", "discover.rx.unparsed", "discovery packet not parsed", map[string]any{
				"iface":     iface.Name,
				"ip":        iface.IP,
				"broadcast": iface.Broadcast,
				"from":      addr.IP.String(),
				"port":      addr.Port,
				"len":       n,
				"hex":       strings.ToUpper(hex.EncodeToString(packet)),
				"is_any":    iface.IsAny,
			})
		}
	}
	emitDiscovery("DEBUG", "discover.iface.summary", "interface probe completed", map[string]any{
		"iface":      iface.Name,
		"ip":         iface.IP,
		"broadcast":  iface.Broadcast,
		"sent":       sentCount,
		"received":   rxCount,
		"has_result": len(result) > 0,
		"is_any":     iface.IsAny,
	})
	if rxCount == 0 {
		emitDiscovery("INFO", "discover.empty", "no UDP responses received for interface", map[string]any{
			"iface":     iface.Name,
			"ip":        iface.IP,
			"broadcast": iface.Broadcast,
			"sent":      sentCount,
			"received":  rxCount,
			"is_any":    iface.IsAny,
		})
	}
	return result, nil
}

func discoveryTargetsForProbe(iface localInterfaceEntry) []discoveryProbe {
	targets := []discoveryProbe{
		{
			Addr:     &net.UDPAddr{IP: net.IPv4bcast, Port: 6811},
			Payload:  zgwSearchPacket,
			Protocol: "hsfz",
			Label:    "zgwSearchPacket",
		},
		{
			Addr:     &net.UDPAddr{IP: net.IPv4bcast, Port: 13400},
			Payload:  doipSearchPacket,
			Protocol: "doip",
			Label:    "doipSearchPacket",
		},
	}

	if strings.HasPrefix(iface.IP, "169.254.") {
		linkLocal := net.ParseIP("169.254.255.255")
		targets = append([]discoveryProbe{
			{
				Addr:     &net.UDPAddr{IP: linkLocal, Port: 6811},
				Payload:  zgwSearchPacket,
				Protocol: "hsfz",
				Label:    "zgwSearchPacket.linklocal",
			},
			{
				Addr:     &net.UDPAddr{IP: linkLocal, Port: 6801},
				Payload:  zgwSearchPacket,
				Protocol: "hsfz",
				Label:    "zgwSearchPacket.boot",
			},
		}, targets...)
	}

	if broadcast := net.ParseIP(iface.Broadcast); broadcast != nil && iface.Broadcast != "" && iface.Broadcast != "255.255.255.255" {
		targets = append(targets, []discoveryProbe{
			{
				Addr:     &net.UDPAddr{IP: broadcast, Port: 6811},
				Payload:  zgwSearchPacket,
				Protocol: "hsfz",
				Label:    "zgwSearchPacket.broadcast",
			},
			{
				Addr:     &net.UDPAddr{IP: broadcast, Port: 6801},
				Payload:  zgwSearchPacket,
				Protocol: "hsfz",
				Label:    "zgwSearchPacket.broadcast.boot",
			},
			{
				Addr:     &net.UDPAddr{IP: broadcast, Port: 13400},
				Payload:  doipSearchPacket,
				Protocol: "doip",
				Label:    "doipSearchPacket.broadcast",
			},
		}...)
	}

	return targets
}

func discoveryTargetsForIP(bindIP string) []discoveryProbe {
	return discoveryTargetsForProbe(localInterfaceEntry{
		Name:      "compat",
		IP:        bindIP,
		Broadcast: "255.255.255.255",
		IsAny:     bindIP == "0.0.0.0",
	})
}

func parseBMWDiscoveryResponse(packet []byte, ip string) (vehicleCandidate, bool) {
	if len(packet) < 6 {
		return vehicleCandidate{}, false
	}
	if !bytesEqual(packet[4:6], []byte{0x00, 0x11}) && !(len(packet) >= 12 && bytesEqual(packet[8:12], []byte{0x00, 0x00, 0x00, 0x11})) {
		return vehicleCandidate{}, false
	}
	text := string(packet)
	vin := extractBMWVIN(text)
	if vin == "" && len(packet) >= 29 {
		vin = sanitizeVIN(string(packet[len(packet)-17:]))
	}
	if vin == "" {
		return vehicleCandidate{}, false
	}
	return vehicleCandidate{
		IP:              ip,
		VIN:             vin,
		Protocol:        "hsfz",
		DiscoverySource: "enet_bmwvin",
	}, true
}

func broadcastFor(ipNet *net.IPNet) string {
	if ipNet == nil || ipNet.IP == nil || ipNet.Mask == nil {
		return "255.255.255.255"
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		return "255.255.255.255"
	}
	mask := ipNet.Mask
	if len(mask) != net.IPv4len {
		return "255.255.255.255"
	}
	broadcast := make(net.IP, net.IPv4len)
	for i := 0; i < net.IPv4len; i++ {
		broadcast[i] = ip[i] | ^mask[i]
	}
	return broadcast.String()
}

func isDiagnosticPattern(packet []byte) bool {
	if len(packet) >= 12 && bytesEqual(packet[8:12], []byte{0x00, 0x00, 0x00, 0x11}) {
		return true
	}
	return len(packet) >= 8 && packet[0] == doipVersion && packet[1] == byte(^byte(doipVersion)) && binary.BigEndian.Uint16(packet[2:4]) == doipPayloadVinResp
}

func parseDoIPDiscoveryResponse(packet []byte, ip string) (vehicleCandidate, bool) {
	if len(packet) < 27 {
		return vehicleCandidate{}, false
	}
	if packet[0] != doipVersion || packet[1] != byte(^byte(doipVersion)) {
		return vehicleCandidate{}, false
	}
	if binary.BigEndian.Uint16(packet[2:4]) != doipPayloadVinResp {
		return vehicleCandidate{}, false
	}
	payloadLength := int(binary.BigEndian.Uint32(packet[4:8]))
	if payloadLength <= 0 || len(packet) < 8+payloadLength {
		return vehicleCandidate{}, false
	}
	payload := packet[8 : 8+payloadLength]
	if len(payload) < 19 {
		return vehicleCandidate{}, false
	}
	vin := sanitizeVIN(string(payload[:17]))
	if vin == "" {
		return vehicleCandidate{}, false
	}
	return vehicleCandidate{
		IP:              ip,
		VIN:             vin,
		Protocol:        "doip",
		DiscoverySource: "doip_vehicle_identification",
	}, true
}

func extractBMWVIN(text string) string {
	index := strings.Index(text, "BMWVIN")
	if index < 0 {
		return ""
	}
	return sanitizeVIN(text[index+6:])
}

func sanitizeVIN(value string) string {
	if len(value) < 17 {
		return ""
	}
	candidate := strings.ToUpper(strings.TrimSpace(value[:17]))
	for _, char := range candidate {
		switch {
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		default:
			return ""
		}
	}
	return candidate
}

func detectProtocol(ctx context.Context, ip string) (string, error) {
	dialer := net.Dialer{Timeout: 1200 * time.Millisecond}
	hsfzConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, "6801"))
	if err == nil {
		_ = hsfzConn.Close()
		return "hsfz", nil
	}
	doipConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, "13400"))
	if err == nil {
		_ = doipConn.Close()
		return "doip", nil
	}
	return "", fmt.Errorf("no diagnostic ports reachable for %s", ip)
}

func probeDiagnosticPorts(ctx context.Context, candidate vehicleCandidate) {
	for _, port := range []string{"6801", "13400"} {
		dialer := net.Dialer{Timeout: 1200 * time.Millisecond}
		start := time.Now()
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(candidate.IP, port))
		fields := map[string]any{
			"ip":          candidate.IP,
			"port":        port,
			"duration_ms": time.Since(start).Milliseconds(),
		}
		if err != nil {
			fields["error"] = err.Error()
			emitDiscovery("DEBUG", "discover.tcp", "tcp probe failed", fields)
			continue
		}
		_ = conn.Close()
		fields["result"] = "connected"
		emitDiscovery("DEBUG", "discover.tcp", "tcp probe succeeded", fields)
	}
}

func bytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
