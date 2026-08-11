package system

import (
	"context"
	"os/exec"
	"regexp"
)

// ScreenExists reports whether the named screen session is running for a user.
// Sessions are per-user and our servers run under the login user, so we query as
// that user. It's a secondary stop-gate on updates, backing up systemctl state.
func ScreenExists(ctx context.Context, user, session string) bool {
	// `screen -ls` exits non-zero when there are no sessions, so we parse output
	// rather than the exit code. Session lines look like "\t12345.name\t(Detached)".
	out, _ := exec.CommandContext(ctx, "sudo", "-u", user, "screen", "-ls").CombinedOutput()
	re := regexp.MustCompile(`(?m)^\s*\d+\.` + regexp.QuoteMeta(session) + `\b`)
	return re.Match(out)
}
