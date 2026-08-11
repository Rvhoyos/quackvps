package web

import "github.com/rvhoyos/quackvps/internal/config"

// votifier is QuackedSMP's bundled Votifier v2 listener — a plain TCP port opened
// in the firewall, not a web service. Its config lives in quackedsmp.json under
// "votifier".
type votifier struct{}

func (votifier) Key() string              { return config.PortVotifier }
func (votifier) IsWeb() bool              { return false }
func (votifier) DefaultPort() int         { return 8192 }
func (votifier) DefaultSubdomain() string { return "" }
func (votifier) Proto() string            { return "tcp" }

func (v votifier) WritePort(dir string, port int) error {
	return setSectionPort(dir, keysVotifier, port)
}

func (votifier) CaddyBlock(string, string, int) string { return "" }
