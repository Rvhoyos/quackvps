package web

import "fmt"

// caddyBlock renders the standard reverse-proxy site block shared by the web
// components. The dashboard also serves world-backup downloads through this port,
// and a plain reverse_proxy passes file downloads through fine, so one shape fits
// both.
func caddyBlock(subdomain, domain string, port int) string {
	return fmt.Sprintf("%s.%s {\n\treverse_proxy 127.0.0.1:%d\n}\n", subdomain, domain, port)
}
