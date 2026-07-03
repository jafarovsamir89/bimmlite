package transport

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func decodeHex(s string) ([]byte, error) {
	cleaned := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), " ", ""), "0x", "")
	if cleaned == "" {
		return nil, errors.New("empty hex string")
	}
	if len(cleaned)%2 == 1 {
		cleaned = "0" + cleaned
	}
	return hex.DecodeString(cleaned)
}

func encodeHex(b []byte) string {
	return strings.ToUpper(hex.EncodeToString(b))
}

func formatU16(v uint16) string {
	return fmt.Sprintf("%04X", v)
}

func formatU8(v uint8) string {
	return fmt.Sprintf("%02X", v)
}

func parseMaybeHexU16(value any) (uint16, error) {
	switch v := value.(type) {
	case string:
		cleaned := strings.TrimSpace(v)
		if cleaned == "" {
			return 0, errors.New("empty string")
		}
		cleaned = strings.TrimPrefix(strings.ToLower(cleaned), "0x")
		parsed, err := strconv.ParseUint(cleaned, 16, 16)
		if err != nil {
			return 0, err
		}
		return uint16(parsed), nil
	case float64:
		return uint16(v), nil
	case int:
		return uint16(v), nil
	default:
		return 0, fmt.Errorf("unsupported address type %T", value)
	}
}
