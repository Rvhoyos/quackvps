package system

import (
	"context"

	"github.com/rvhoyos/quackvps/internal/dl"
)

// PublicIP returns the box's public IPv4 as seen from the internet, or "" if it
// can't be determined. It's used to sanity-check a domain's DNS and to build the
// SSH-tunnel command shown at the end; callers treat "" as "unknown" and degrade
// gracefully rather than failing.
func PublicIP(ctx context.Context) string {
	ip, err := dl.GetBytes(ctx, "https://api.ipify.org")
	if err != nil {
		return ""
	}
	return string(ip)
}
