package transport

import (
	"context"
	"errors"
	"os"
	"strings"
)

type AutoTransport struct {
	preferred string
	doip      *DoIPTransport
	hsfz      *HSFZTransport
	active    AdapterTransport
	sink      FrameSink
}

func NewAutoTransportFromEnv() *AutoTransport {
	preferred := strings.ToLower(strings.TrimSpace(getEnv("BRIDGE_PROTOCOL", "auto")))
	targetHost := strings.TrimSpace(getEnv("BRIDGE_TARGET_HOST", ""))
	doipHost := strings.TrimSpace(getEnv("BRIDGE_DOIP_HOST", targetHost))
	hsfzHost := strings.TrimSpace(getEnv("BRIDGE_HSFZ_HOST", targetHost))

	return &AutoTransport{
		preferred: preferred,
		doip:      NewDoIPTransport(doipHost),
		hsfz:      NewHSFZTransport(hsfzHost),
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
	if a.active == nil {
		if err := a.Connect(ctx); err != nil {
			return DiscoveryResult{}, err
		}
	}
	return a.active.Discover(ctx)
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
