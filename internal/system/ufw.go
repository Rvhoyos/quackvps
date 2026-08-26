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

// Rule is one port ufw has open, as `ufw status` reports it.
type Rule struct {
	Port  int
	Proto string // "tcp" or "udp"
}

func (r Rule) String() string { return fmt.Sprintf("%d/%s", r.Port, r.Proto) }

var ufwRuleRE = regexp.MustCompile(`^(\d+)/(tcp|udp)`)

// Rules returns the port rules ufw currently has. A box without ufw yet simply
// has none. Rules are listed once per direction/family, so duplicates are dropped
// here: callers care which ports are open, not how many lines say so.
func (UFW) Rules(ctx context.Context) ([]Rule, error) {
	if !HasCommand("ufw") {
		return nil, nil
	}
	out, err := Capture(ctx, "ufw", "status")
	if err != nil {
		return nil, err
	}
	var rules []Rule
	seen := map[Rule]bool{}
	for _, line := range splitLines(out) {
		m := ufwRuleRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		port, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		r := Rule{Port: port, Proto: m[2]}
		if !seen[r] {
			seen[r] = true
			rules = append(rules, r)
		}
	}
	return rules, nil
}

// UsedPorts returns the ports ufw already has rules for, feeding the collision
// scan.
func (u UFW) UsedPorts(ctx context.Context) ([]int, error) {
	rules, err := u.Rules(ctx)
	if err != nil {
		return nil, err
	}
	ports := make([]int, 0, len(rules))
	for _, r := range rules {
		ports = append(ports, r.Port)
	}
	return ports, nil
}

// Delete closes a port ufw has open. It's the exact counterpart of Allow, so a
// rule this tool added is removed the same way it was written.
func (UFW) Delete(ctx context.Context, r Rule) error {
	return Run(ctx, "ufw", "delete", "allow", r.String())
}
