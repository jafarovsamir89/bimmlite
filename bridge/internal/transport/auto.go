package transport

import (
	"context"
	"errors"
	"os"
	"strings"
)

type AutoTransport struct {
	preferred  string
	doip       *DoIPTransport
	hsfz       *HSFZTransport
	active     AdapterTransport
	sink       FrameSink
	targetHost string
	discovered *vehicleCandidate
}

func NewAutoTransportFromEnv() *AutoTransport {
	preferred := strings.ToLower(strings.TrimSpace(getEnv("BRIDGE_PROTOCOL", "auto")))
	targetHost := strings.TrimSpace(getEnv("BRIDGE_TARGET_HOST", ""))
	doipHost := strings.TrimSpace(getEnv("BRIDGE_DOIP_HOST", targetHost))
	hsfzHost := strings.TrimSpace(getEnv("BRIDGE_HSFZ_HOST", targetHost))

	return &AutoTransport{
		preferred:  preferred,
		doip:       NewDoIPTransport(doipHost),
		hsfz:       NewHSFZTransport(hsfzHost),
		targetHost: targetHost,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (a *AutoTransport) Name() string {
	if a.active != nil {
		return a.active.Name()
	}
	return "auto"
}

func (a *AutoTransport) SetFrameSink(sink FrameSink) {
	a.sink = sink
	if a.doip != nil {
		a.doip.SetFrameSink(sink)
	}
	if a.hsfz != nil {
		a.hsfz.SetFrameSink(sink)
	}
	if a.active != nil {
		a.active.SetFrameSink(sink)
	}
}

func (a *AutoTransport) Connect(ctx context.Context) error {
	if err := a.ensureTarget(ctx); err != nil {
		return err
	}
	tryOrder := []AdapterTransport{}
	switch a.preferred {
	case "doip":
		tryOrder = append(tryOrder, a.doip, a.hsfz)
	case "hsfz":
		tryOrder = append(tryOrder, a.hsfz, a.doip)
	default:
		tryOrder = append(tryOrder, a.doip, a.hsfz)
	}

	var errs []string
	for _, candidate := range tryOrder {
		if candidate == nil {
			continue
		}
		candidate.SetFrameSink(a.sink)
		if err := candidate.Connect(ctx); err == nil {
			a.active = candidate
			return nil
		} else {
			errs = append(errs, candidate.Name()+": "+err.Error())
		}
	}
	return errors.New(strings.Join(errs, "; "))
}

func (a *AutoTransport) Discover(ctx context.Context) (DiscoveryResult, error) {
	if err := a.ensureTarget(ctx); err != nil {
		return DiscoveryResult{}, err
	}
	if a.active == nil {
		if a.discovered != nil {
			switch a.discovered.Protocol {
			case "doip":
				a.active = a.doip
			case "hsfz":
				a.active = a.hsfz
			}
		}
		if a.active == nil {
			if err := a.Connect(ctx); err != nil {
				return DiscoveryResult{}, err
			}
		}
	}
	discovery, err := a.active.Discover(ctx)
	if err != nil {
		return DiscoveryResult{}, err
	}
	if a.discovered != nil {
		discovery.IP = a.discovered.IP
		if discovery.VIN == "" {
			discovery.VIN = a.discovered.VIN
		}
		if discovery.DiscoverySource == "" {
			discovery.DiscoverySource = a.discovered.DiscoverySource
		}
	}
	return discovery, nil
}

func (a *AutoTransport) ScanECUs(ctx context.Context) ([]ECUInfo, error) {
	if a.active == nil {
		if err := a.Connect(ctx); err != nil {
			return nil, err
		}
	}
	return a.active.ScanECUs(ctx)
}

func (a *AutoTransport) ReadDTC(ctx context.Context, ecu ECUInfo) ([]DTCInfo, error) {
	if a.active == nil {
		if err := a.Connect(ctx); err != nil {
			return nil, err
		}
	}
	return a.active.ReadDTC(ctx, ecu)
}

func (a *AutoTransport) ReadParameters(ctx context.Context, ecu ECUInfo, dids []string) ([]ParameterInfo, error) {
	if a.active == nil {
		if err := a.Connect(ctx); err != nil {
			return nil, err
		}
	}
	return a.active.ReadParameters(ctx, ecu, dids)
}

func (a *AutoTransport) ClearDTC(ctx context.Context, ecu ECUInfo) (map[string]any, error) {
	if a.active == nil {
		if err := a.Connect(ctx); err != nil {
			return nil, err
		}
	}
	return a.active.ClearDTC(ctx, ecu)
}

func (a *AutoTransport) TesterPresent(ctx context.Context, ecu ECUInfo) error {
	if a.active == nil {
		if err := a.Connect(ctx); err != nil {
			return err
		}
	}
	return a.active.TesterPresent(ctx, ecu)
}

func (a *AutoTransport) Close() error {
	if a.active != nil {
		return a.active.Close()
	}
	return nil
}

func (a *AutoTransport) ensureTarget(ctx context.Context) error {
	if a.targetHost != "" {
		a.doip.host = strings.TrimSpace(getEnv("BRIDGE_DOIP_HOST", a.targetHost))
		a.hsfz.host = strings.TrimSpace(getEnv("BRIDGE_HSFZ_HOST", a.targetHost))
		return nil
	}
	if a.discovered != nil {
		return nil
	}
	discovery, err := discoverVehicle(ctx)
	if err != nil {
		return err
	}
	a.discovered = &discovery
	a.doip.host = discovery.IP
	a.hsfz.host = discovery.IP
	if a.preferred == "auto" || a.preferred == "" {
		a.preferred = discovery.Protocol
	}
	return nil
}
