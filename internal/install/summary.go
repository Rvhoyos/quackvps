package install

import (
	"fmt"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/minecraft"
	"github.com/rvhoyos/quackvps/internal/ui"
	"github.com/rvhoyos/quackvps/internal/web"
)

// printSummary reports how to reach and manage the new server: the game address,
// the console command, and the single web-access block (HTTPS URLs, or one SSH
// tunnel for the no-domain path).
func printSummary(cfg *config.Config) {
	ui.Step("Done")

	host := cfg.PublicIP
	if host == "" {
		host = "<your-server-ip>"
	}

	ui.Success("Server %q is running.", cfg.Instance)
	ui.Bullet(
		fmt.Sprintf("Connect in Minecraft: %s:%d", host, cfg.ServerPort),
		fmt.Sprintf("Live console: screen -r %s (as %s)", cfg.Instance, cfg.RunAsUser),
		fmt.Sprintf("Service: systemctl status %s", minecraft.UnitName(cfg.Instance)),
	)

	if summary := web.AccessSummary(cfg, cfg.RunAsUser+"@"+host); summary != "" {
		fmt.Println()
		fmt.Print(summary)
	}
	if dns := web.DNSRecordGuidance(cfg, host); dns != "" {
		fmt.Println()
		fmt.Print(dns)
	}
}
