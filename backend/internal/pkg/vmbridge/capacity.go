package vmbridge

import (
	"context"
	"net/http"
)

type ProxyCapacity struct {
	Used      int  `json:"used"`
	Limit     int  `json:"limit"`
	Available bool `json:"available"`
}

type ManagedCapacityProvider interface {
	ManagedProxyCapacity(context.Context, []string) ([]ProxyCapacity, error)
}

func (m *Manager) serveCapacity(w http.ResponseWriter, r *http.Request) {
	p, ok := m.provider.(ManagedCapacityProvider)
	if !ok {
		writeError(w, bridgeError(501, "capacity_unavailable", "Proxy capacity is unavailable"))
		return
	}
	var in struct {
		Proxies []string `json:"proxies"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if len(in.Proxies) == 0 || len(in.Proxies) > 32 {
		writeError(w, bridgeError(400, "invalid_proxies", "Select 1 to 32 proxies"))
		return
	}
	result, err := p.ManagedProxyCapacity(r.Context(), in.Proxies)
	if err != nil {
		writeError(w, bridgeError(502, "capacity_unavailable", "Unable to check VM2 proxy capacity"))
		return
	}
	writeJSON(w, 200, result)
}
