package transport

import (
	"fmt"
	"strings"
)

func parseDTCResponse(ecu ECUInfo, raw []byte) []DTCInfo {
	if len(raw) == 0 {
		return nil
	}

	result := make([]DTCInfo, 0, len(raw)/4)
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

	buf := make([]byte, 0, len(raw))
	for _, value := range raw {
		if value >= 0x20 && value <= 0x7E {
			buf = append(buf, value)
		}
	}
	return strings.TrimSpace(string(buf))
}
