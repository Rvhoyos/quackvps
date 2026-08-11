package system

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// NextFree returns the lowest port at or above base that isn't in used. It's how
// every port prompt derives its collision-checked default.
func NextFree(base int, used map[int]bool) int {
	p := base
	for used[p] && p < 65535 {
		p++
	}
	return p
}

// CollisionScan gathers every port already spoken for on the box so the wizard
// can offer a free default. It unions three sources, per the design: other
// instances' configs under parent, ufw's rules, and live listening sockets.
func CollisionScan(ctx context.Context, parent string) (map[int]bool, error) {
	used := map[int]bool{}

	ufwPorts, err := Firewall.UsedPorts(ctx)
	if err != nil {
		return nil, err
	}
	addAll(used, ufwPorts)

	listening, err := Listening(ctx)
	if err != nil {
		return nil, err
	}
	addAll(used, listening)

	addAll(used, instancePorts(parent))
	return used, nil
}

var ssPortRE = regexp.MustCompile(`:(\d+)$`)

// Listening returns the ports with a listening socket, via `ss -tuln`.
func Listening(ctx context.Context) ([]int, error) {
	if !HasCommand("ss") {
		return nil, nil
	}
	out, err := Capture(ctx, "ss", "-tuln")
	if err != nil {
		return nil, err
	}
	var ports []int
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 5 || (fields[0] != "tcp" && fields[0] != "udp") {
			continue
		}
		if m := ssPortRE.FindStringSubmatch(fields[4]); m != nil {
			if p, err := strconv.Atoi(m[1]); err == nil {
				ports = append(ports, p)
			}
		}
	}
	return ports, nil
}

// configPortRE tolerantly pulls port numbers out of the instance config formats
// we know (properties `port=`, HOCON `port:`, JSON `"port":`). Over-matching is
// safe here: a false positive only makes a default skip a port it needn't.
var configPortRE = regexp.MustCompile(`(?i)port"?\s*[:=]\s*"?(\d{2,5})`)

// instancePorts scans sibling instances under parent for ports already in use,
// so a second server never defaults onto a first server's port.
func instancePorts(parent string) []int {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil
	}
	relFiles := []string{
		"server.properties",
		"config/quackedsmp.json",
		"config/bluemap/webserver.conf",
		"config/voicechat/voicechat-server.properties",
	}
	var ports []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for _, rel := range relFiles {
			ports = append(ports, scanPortsInFile(filepath.Join(parent, e.Name(), rel))...)
		}
	}
	return ports
}

func scanPortsInFile(path string) []int {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var ports []int
	for _, m := range configPortRE.FindAllStringSubmatch(string(data), -1) {
		if p, err := strconv.Atoi(m[1]); err == nil {
			ports = append(ports, p)
		}
	}
	return ports
}

func addAll(set map[int]bool, ports []int) {
	for _, p := range ports {
		set[p] = true
	}
}

func splitLines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}
