package transport

import (
	"fmt"
	"strings"
)

var dtcStatusBits = []struct {
	mask uint8
	name string
}{
	{mask: 0x01, name: "testFailed"},
	{mask: 0x02, name: "testFailedThisOperationCycle"},
	{mask: 0x04, name: "pendingDTC"},
	{mask: 0x08, name: "confirmedDTC"},
	{mask: 0x10, name: "testNotCompletedSinceLastClear"},
	{mask: 0x20, name: "testFailedSinceLastClear"},
	{mask: 0x40, name: "testNotCompletedThisOperationCycle"},
	{mask: 0x80, name: "warningIndicatorRequested"},
}

func parseDTCResponse(ecu ECUInfo, raw []byte) []DTCInfo {
	if len(raw) == 0 {
		return nil
	}

	result := make([]DTCInfo, 0, len(raw)/4)
	for i := 0; i+3 < len(raw); i += 4 {
		code := fmt.Sprintf("%02X%02X%02X", raw[i], raw[i+1], raw[i+2])
		status := decodeDTCStatus(raw[i+3])
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

func decodeDTCStatus(value uint8) string {
	labels := make([]string, 0, len(dtcStatusBits))
	for _, item := range dtcStatusBits {
		if value&item.mask != 0 {
			labels = append(labels, item.name)
		}
	}
	if len(labels) == 0 {
		return fmt.Sprintf("0x%02X", value)
	}
	return fmt.Sprintf("0x%02X %s", value, strings.Join(labels, "|"))
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
