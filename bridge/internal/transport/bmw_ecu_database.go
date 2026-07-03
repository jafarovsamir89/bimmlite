package transport

type bmwEcuDefinition struct {
	Address uint8
	Name    string
}

// BMW ECU database is a clean, editable resource for live-first discovery.
// It intentionally contains only read-only diagnostics targets and avoids any
// coding or programming concepts.
var bmwECUDatabase = []bmwEcuDefinition{
	{Address: 0x10, Name: "ZGW"},
	{Address: 0x12, Name: "CAS"},
	{Address: 0x18, Name: "KOMBI"},
	{Address: 0x1B, Name: "ACSM"},
	{Address: 0x1D, Name: "PDC"},
	{Address: 0x1F, Name: "HUD"},
	{Address: 0x23, Name: "DSC"},
	{Address: 0x25, Name: "EPS"},
	{Address: 0x28, Name: "IHKA"},
	{Address: 0x29, Name: "EGS"},
	{Address: 0x2C, Name: "BDC"},
	{Address: 0x2E, Name: "DME"},
	{Address: 0x30, Name: "FEM_BODY"},
	{Address: 0x31, Name: "BDC_BODY"},
	{Address: 0x39, Name: "REM"},
	{Address: 0x3B, Name: "FZD"},
	{Address: 0x40, Name: "VIN_FALLBACK"},
	{Address: 0x43, Name: "HU_NBT"},
	{Address: 0x44, Name: "HU_ENTRY"},
	{Address: 0x4C, Name: "PDC_REAR"},
	{Address: 0x50, Name: "SAS"},
	{Address: 0x5B, Name: "RDC"},
	{Address: 0x60, Name: "GWS"},
	{Address: 0x67, Name: "KAFAS"},
	{Address: 0x6F, Name: "ICM"},
	{Address: 0x71, Name: "EPS2"},
	{Address: 0xDF, Name: "FUNCTIONAL_BROADCAST"},
}

func bmwEcuName(address uint8) string {
	for _, ecu := range bmwECUDatabase {
		if ecu.Address == address {
			return ecu.Name
		}
	}
	return "ECU_" + formatU8(address)
}

func bmwEcuCandidates() []uint8 {
	candidates := make([]uint8, 0, len(bmwECUDatabase))
	for _, ecu := range bmwECUDatabase {
		if ecu.Address == 0xDF {
			continue
		}
		candidates = append(candidates, ecu.Address)
	}
	return candidates
}
