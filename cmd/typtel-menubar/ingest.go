//go:build darwin
// +build darwin

package main

import "strings"

// defaultBindAddr is the listener's bind address when device_ingest_bind_addr
// is unset.
//
// LOOPBACK ONLY. On macOS the Tailscale client does not route inbound tailnet
// connections to a listener bound on the utun IP, so binding the tailnet IP (or
// 0.0.0.0) does not work and needlessly exposes the port to the LAN. The tailnet
// reaches this loopback listener via `tailscale serve` (raw TCP passthrough),
// which is configured outside this app — see SLAVE-DEVICE-INGEST.md "Closing the
// loop". Keeping the listener on 127.0.0.1 is both the working and the safe
// default.
const defaultBindAddr = "127.0.0.1:8889"

// splitCSV splits a comma-separated setting into trimmed, non-empty entries.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
