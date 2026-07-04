package transport

import "context"

type ECUInfo struct {
	Address  string `json:"address"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Present  bool   `json:"present"`
}

type DTCInfo struct {
	ECUAddress  string `json:"ecu_address"`
	ECUName     string `json:"ecu_name"`
	Code        string `json:"code"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Raw         string `json:"raw"`
}

type ParameterInfo struct {
	ECUAddress string `json:"ecu_address"`
	ECUName    string `json:"ecu_name"`
	DID        string `json:"did"`
	ValueHex   string `json:"value_hex"`
	ValueText  string `json:"value_text"`
}

type DiscoveryResult struct {
	Protocol        string    `json:"protocol"`
	VIN             string    `json:"vin"`
	BatteryVoltage  float64   `json:"battery_voltage,omitempty"`
	ECUs            []ECUInfo `json:"ecus"`
	TargetAddress   string    `json:"target_address,omitempty"`
	SessionControl  string    `json:"session_control,omitempty"`
	DiscoverySource string    `json:"discovery_source,omitempty"`
}

type FrameRecord struct {
	Protocol  string         `json:"protocol"`
	Direction string         `json:"direction"`
	FrameHex  string         `json:"frame_hex"`
	Source    string         `json:"source,omitempty"`
	Target    string         `json:"target,omitempty"`
	ServiceID string         `json:"service_id,omitempty"`
	NRC       string         `json:"nrc,omitempty"`
	RTTMS     int64          `json:"rtt_ms,omitempty"`
	Message   string         `json:"message,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type FrameSink func(FrameRecord)

type AdapterTransport interface {
	Name() string
	Connect(ctx context.Context) error
	Discover(ctx context.Context) (DiscoveryResult, error)
	ScanECUs(ctx context.Context) ([]ECUInfo, error)
	ReadDTC(ctx context.Context, ecu ECUInfo) ([]DTCInfo, error)
	ReadParameters(ctx context.Context, ecu ECUInfo, dids []string) ([]ParameterInfo, error)
	TesterPresent(ctx context.Context, ecu ECUInfo) error
	Close() error
	SetFrameSink(FrameSink)
}
