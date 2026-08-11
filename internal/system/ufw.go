package system

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// EnsureInstalled installs ufw if the box doesn't ship it (some minimal Debian
// images don't).
func (UFW) EnsureInstalled(ctx context.Context) error {
	if HasCommand("ufw") {
		return nil
	}
	return AptInstall(ctx, "ufw")
}

// UFW groups the firewall operations. It's a zero-size receiver purely to give
// the ufw calls a clear namespace (system.Firewall.Allow...) at the call site.
type UFW struct{}

// Firewall is the entry point for ufw operations.
var Firewall UFW

// AllowSSHFirst opens the SSH port. It must run before Enable so turning the
// firewall on can never drop the session that's driving the install.
func (UFW) AllowSSHFirst(ctx context.Context, port int) error {
	return Run(ctx, "ufw", "allow", fmt.Sprintf("%d/tcp", port))
}

// Allow opens a port for a protocol ("tcp" or "udp").
func (UFW) Allow(ctx context.Context, port int, proto string) error {
	return Run(ctx, "ufw", "allow", fmt.Sprintf("%d/%s", port, proto))
}

// Enable turns the firewall on. --force skips the interactive confirmation.
func (UFW) Enable(ctx context.Context) error {
	return Run(ctx, "ufw", "--force", "enable")
}

var ufwRuleRE = regexp.MustCompile(`^(\d+)/(tcp|udp)`)

// UsedPorts returns the ports ufw already has rules for, feeding the collision
// scan. A box without ufw yet simply contributes nothing.
func (UFW) UsedPorts(ctx context.Context) ([]int, error) {
	if !HasCommand("ufw") {
		return nil, nil
	}
	out, err := Capture(ctx, "ufw", "status")
	if err != nil {
		return nil, err
	}
	var ports []int
	for _, line := range splitLines(out) {
		if m := ufwRuleRE.FindStringSubmatch(line); m != nil {
			if p, err := strconv.Atoi(m[1]); err == nil {
				ports = append(ports, p)
			}
		}
	}
	return ports, nil
}
