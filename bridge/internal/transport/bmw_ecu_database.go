package transport

type bmwEcuDefinition struct {
	Address uint8
	Name    string
}

// BMW ECU database is migrated as data from the existing BimmerApp catalog.
// It is intentionally kept as a read-only address/name map for diagnostics.
var bmwECUDatabase = []bmwEcuDefinition{
	{Address: 0x01, Name: "ACSM"},
	{Address: 0x05, Name: "CDM"},
	{Address: 0x06, Name: "TRSVC"},
	{Address: 0x07, Name: "SME"},
	{Address: 0x08, Name: "HC2"},
	{Address: 0x09, Name: "RXE_RDME"},
	{Address: 0x0A, Name: "REME"},
	{Address: 0x0B, Name: "SCR"},
	{Address: 0x0D, Name: "HKFM"},
	{Address: 0x0E, Name: "SVT"},
	{Address: 0x0F, Name: "QSG"},
	{Address: 0x10, Name: "ZGW_FEM_GW_BDC15"},
	{Address: 0x12, Name: "DDE_DME"},
	{Address: 0x13, Name: "DME2_DDE2"},
	{Address: 0x14, Name: "LIM"},
	{Address: 0x15, Name: "KLE"},
	{Address: 0x16, Name: "ASA"},
	{Address: 0x17, Name: "EKPM"},
	{Address: 0x18, Name: "EGS"},
	{Address: 0x19, Name: "LMV"},
	{Address: 0x1A, Name: "AE_EME"},
	{Address: 0x1C, Name: "ICM"},
	{Address: 0x1D, Name: "TFM"},
	{Address: 0x20, Name: "RDC"},
	{Address: 0x21, Name: "FRR"},
	{Address: 0x22, Name: "SAS_01"},
	{Address: 0x24, Name: "CVM"},
	{Address: 0x26, Name: "RSE"},
	{Address: 0x27, Name: "CGW"},
	{Address: 0x29, Name: "DSC"},
	{Address: 0x2A, Name: "EMF"},
	{Address: 0x2B, Name: "HSR"},
	{Address: 0x2C, Name: "PMA"},
	{Address: 0x2E, Name: "PCU"},
	{Address: 0x30, Name: "EPS"},
	{Address: 0x31, Name: "MMC"},
	{Address: 0x35, Name: "TBX"},
	{Address: 0x36, Name: "TEL_MULF_COMBOX"},
	{Address: 0x37, Name: "AMP"},
	{Address: 0x38, Name: "EHC"},
	{Address: 0x39, Name: "ICM_V"},
	{Address: 0x3A, Name: "EME2"},
	{Address: 0x3C, Name: "CDC"},
	{Address: 0x3D, Name: "HUD"},
	{Address: 0x3E, Name: "ACP_IHKA"},
	{Address: 0x3F, Name: "ASD"},
	{Address: 0x40, Name: "CAS_FEM_BODY"},
	{Address: 0x41, Name: "TMS_LEFT"},
	{Address: 0x42, Name: "TMS_RIGHT"},
	{Address: 0x44, Name: "LHM"},
	{Address: 0x46, Name: "GZA"},
	{Address: 0x48, Name: "VSW"},
	{Address: 0x49, Name: "SEC1"},
	{Address: 0x4A, Name: "SEC2"},
	{Address: 0x4B, Name: "TVM"},
	{Address: 0x4D, Name: "EMA_FA_REMA_FA"},
	{Address: 0x4E, Name: "EMA_BF_REMA_BF"},
	{Address: 0x54, Name: "SDARS"},
	{Address: 0x55, Name: "MULF"},
	{Address: 0x56, Name: "FZD"},
	{Address: 0x57, Name: "NIVI"},
	{Address: 0x59, Name: "ALBV_FA"},
	{Address: 0x5A, Name: "ALBV_BF"},
	{Address: 0x5D, Name: "KAFAS"},
	{Address: 0x5E, Name: "GWS"},
	{Address: 0x5F, Name: "FLA"},
	{Address: 0x60, Name: "KOMBI"},
	{Address: 0x61, Name: "CB_EC"},
	{Address: 0x62, Name: "HEAD_UNIT"},
	{Address: 0x63, Name: "HU_RAD"},
	{Address: 0x64, Name: "PDC"},
	{Address: 0x67, Name: "ZBE"},
	{Address: 0x68, Name: "ZBE_FOND"},
	{Address: 0x69, Name: "SM_REH"},
	{Address: 0x6A, Name: "SM_LIH"},
	{Address: 0x6B, Name: "HKL"},
	{Address: 0x6D, Name: "SM_RE"},
	{Address: 0x6E, Name: "SM_LI"},
	{Address: 0x71, Name: "AHM"},
	{Address: 0x72, Name: "FRM_REM"},
	{Address: 0x73, Name: "CID"},
	{Address: 0x74, Name: "CID_R1"},
	{Address: 0x75, Name: "CID_R2"},
	{Address: 0x76, Name: "VDC"},
	{Address: 0x77, Name: "RFK"},
	{Address: 0x78, Name: "IHKA_IHKH"},
	{Address: 0x79, Name: "FKA"},
	{Address: 0x7B, Name: "HKA"},
	{Address: 0x7D, Name: "EVALBOARD_ETH"},
	{Address: 0x86, Name: "KOMBI_DEV"},
	{Address: 0xA0, Name: "HEAD_UNIT_A0"},
	{Address: 0xA5, Name: "RK_VL"},
	{Address: 0xA6, Name: "RK_VR"},
	{Address: 0xA7, Name: "RK_HL"},
	{Address: 0xA8, Name: "RK_HR"},
	{Address: 0xA9, Name: "CDC_2PROZ"},
	{Address: 0xAB, Name: "MMC_HW"},
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
