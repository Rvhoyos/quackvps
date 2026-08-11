package minecraft

import (
	"fmt"

	"github.com/rvhoyos/quackvps/internal/config"
)

// UnitName returns the systemd unit name for an instance.
func UnitName(instance string) string { return "mc-" + instance }

// UnitFile builds the systemd unit that runs an instance inside a screen session
// — the shape verified on the reference box. Type=forking because screen -dm
// backgrounds itself; the graceful ExecStop stuffs "stop" into the console so the
// world saves before exit; TimeoutStopSec gives that save time to finish before
// systemd would resort to SIGKILL.
func UnitFile(cfg *config.Config) string {
	instance := cfg.Instance
	return fmt.Sprintf(`[Unit]
Description=Minecraft server (%[1]s), managed by quackvps
After=network-online.target
Wants=network-online.target

[Service]
Type=forking
User=%[2]s
WorkingDirectory=%[3]s
Restart=always
RestartSec=5s
TimeoutStopSec=120
ExecStart=/usr/bin/screen -S %[1]s -dm bash -lc './run.sh'
ExecStop=/usr/bin/screen -S %[1]s -p 0 -X stuff "say Server stopping...\rstop\r"

[Install]
WantedBy=multi-user.target
`, instance, cfg.RunAsUser, cfg.Dir)
}
