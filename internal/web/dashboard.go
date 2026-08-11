package web

import "github.com/rvhoyos/quackvps/internal/config"

// dashboard is the QuackedSMP web panel: a Caddy-fronted service whose port lives
// in quackedsmp.json under "dashboard". Auth is built into the mod, so it's safe
// behind a plain reverse proxy with no extra auth from us.
type dashboard struct{}

func (dashboard) Key() string              { return config.PortDashboard }
func (dashboard) IsWeb() bool              { return true }
func (dashboard) DefaultPort() int         { return 8125 }
func (dashboard) DefaultSubdomain() string { return "status" }
func (dashboard) Proto() string            { return "" }

func (d dashboard) WritePort(dir string, port int) error {
	return setSectionPort(dir, keysDashboard, port)
}

func (d dashboard) CaddyBlock(subdomain, domain string, port int) string {
	return caddyBlock(subdomain, domain, port)
}
