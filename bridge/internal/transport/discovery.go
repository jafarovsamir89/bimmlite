package transport

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
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

func discoverVehicle(ctx context.Context) (vehicleCandidate, error) {
	interfaces, err := localIPv4Addrs()
	if err != nil {
		return vehicleCandidate{}, err
	}

	var candidates []vehicleCandidate
	seen := map[string]struct{}{}
	for _, ip := range interfaces {
		found, err := discoverOnInterface(ctx, ip)
		if err != nil {
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
		return vehicleCandidate{}, errors.New("no BMW vehicle discovered on ENET/DoIP")
	}

	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if best.Protocol == "" && candidate.Protocol != "" {
			best = candidate
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
	return best, nil
}

func localIPv4Addrs() ([]string, error) {
	netIfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	var result []string
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
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("no active IPv4 interfaces found")
	}
	return result, nil
}

func discoverOnInterface(ctx context.Context, bindIP string) ([]vehicleCandidate, error) {
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
	packetConn, err := listenConfig.ListenPacket(ctx, "udp4", net.JoinHostPort(bindIP, "0"))
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

	targets := []struct {
		addr    *net.UDPAddr
		payload []byte
	}{
		{addr: &net.UDPAddr{IP: net.IPv4bcast, Port: 6811}, payload: zgwSearchPacket},
		{addr: &net.UDPAddr{IP: net.IPv4bcast, Port: 13400}, payload: doipSearchPacket},
	}
	if strings.HasPrefix(bindIP, "169.254.") {
		targets = append(targets,
			struct {
				addr    *net.UDPAddr
				payload []byte
			}{addr: &net.UDPAddr{IP: net.ParseIP("169.254.255.255"), Port: 6811}, payload: zgwSearchPacket},
			struct {
				addr    *net.UDPAddr
				payload []byte
			}{addr: &net.UDPAddr{IP: net.ParseIP("169.254.255.255"), Port: 13400}, payload: doipSearchPacket},
		)
	}

	deadline := time.Now().Add(1500 * time.Millisecond)
	for _, target := range targets {
		_ = conn.SetWriteDeadline(time.Now().Add(300 * time.Millisecond))
		_, _ = conn.WriteToUDP(target.payload, target.addr)
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
		if candidate, ok := parseBMWDiscoveryResponse(packet, addr.IP.String()); ok {
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
		}
	}
	return result, nil
}

func parseBMWDiscoveryResponse(packet []byte, ip string) (vehicleCandidate, bool) {
	if len(packet) < 12 || !bytesEqual(packet[8:12], []byte{0x00, 0x00, 0x00, 0x11}) {
		return vehicleCandidate{}, false
	}
	text := string(packet[12:])
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
		DiscoverySource: "enet_bmwvin",
	}, true
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
	doipConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, "13400"))
	if err == nil {
		_ = doipConn.Close()
		return "doip", nil
	}
	hsfzConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, "6801"))
	if err == nil {
		_ = hsfzConn.Close()
		return "hsfz", nil
	}
	return "", fmt.Errorf("no diagnostic ports reachable for %s", ip)
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
