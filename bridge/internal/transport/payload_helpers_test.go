package transport

import "testing"

func TestDecodeDTCStatus(t *testing.T) {
	status := decodeDTCStatus(0x2F)
	expected := "0x2F testFailed|testFailedThisOperationCycle|pendingDTC|confirmedDTC|testFailedSinceLastClear"
	if status != expected {
		t.Fatalf("unexpected status decode: %s", status)
	}
}

func TestParseDoIPDiscoveryResponse(t *testing.T) {
	payload := append([]byte("WBA12345678901234"), 0x00, 0x10)
	packet := append([]byte{0x02, 0xFD, 0x00, 0x04, 0x00, 0x00, 0x00, byte(len(payload))}, payload...)

	candidate, ok := parseDoIPDiscoveryResponse(packet, "169.254.10.10")
	if !ok {
		t.Fatal("expected DoIP discovery response to parse")
	}
	if candidate.Protocol != "doip" {
		t.Fatalf("unexpected protocol: %s", candidate.Protocol)
	}
	if candidate.VIN != "WBA12345678901234" {
		t.Fatalf("unexpected VIN: %s", candidate.VIN)
	}
}

func TestParseBMWDiscoveryResponse(t *testing.T) {
	packet := append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x11}, []byte("BMWVINWBA12345678901234")...)

	candidate, ok := parseBMWDiscoveryResponse(packet, "169.254.20.20")
	if !ok {
		t.Fatal("expected BMW discovery response to parse")
	}
	if candidate.VIN != "WBA12345678901234" {
		t.Fatalf("unexpected VIN: %s", candidate.VIN)
	}
}
