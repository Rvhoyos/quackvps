# quackvps

Set up a modded Minecraft server and its web layer (panel, live map, HTTPS, firewall) on a fresh Ubuntu/Debian VPS.

SSH into your VPS and paste:

```sh
curl -fsSL https://github.com/rvhoyos/quackvps/releases/latest/download/quackvps-linux-$(dpkg --print-architecture) -o quackvps && chmod +x quackvps && sudo ./quackvps
```

Answer the prompts. Every one has a safe default, so you can hit Enter through the whole thing.

## What it does

- Installs the server (Fabric, NeoForge, Forge, Quilt, or Vanilla) with the matching Java, side by side, without touching your system Java.
- Runs it as a `systemd` service inside `screen`: restarts on crash, starts on boot, attach a console with `screen -r <name>`.
- Installs mods and modpacks from Modrinth, filtered live to ones that run on your loader and version.
- Sets up the optional web layer, picked on one screen:
  - **QuackedSMP** management panel (plus Votifier v2).
  - **BlueMap** live world map.
  - **Simple Voice Chat** proximity voice.
- Configures Caddy as a reverse proxy. With a domain, each web service gets a subdomain with automatic HTTPS. Without one, services stay on localhost and you get a copy-paste `ssh -L` tunnel command.
- Configures UFW so enabling it never drops your SSH session, opening only the ports that must be public.
- Optionally hardens SSH (opt-in, runs first) without locking you out: it walks you through creating a key and verifies it works before disabling passwords.

Run it again to add another server. Each one gets its own folder, service, ports, and subdomains, and coexists with the rest. Ports are scanned every run and the next free one is the default.

## Requirements

- An Ubuntu/Debian-family VPS, any release. quackvps checks for `apt`, `systemd`, and `ufw` and stops with a clear message if the box is unsupported.
- Run it with `sudo`. The server itself runs as your login user, never as root.

## Flags

v1 is interactive by design.

| Flag | Does |
|---|---|
| `--dir <path>` | Start the folder picker at a given parent directory. |
| `--version` | Print the version and exit. |
| `--help` | Print usage and exit. |

<details>
<summary>Roadmap (not yet available)</summary>

- **Restore a backup.** Pick a server and a QuackedSMP world backup; quackvps stops the server, moves the current world aside, unzips the backup in its place, and rolls back on failure.
- **Remove a server.** Clean teardown: stop and disable its service, drop its firewall rules, remove its Caddy config, and delete its folder, after a confirmation.
- **Non-interactive CLI.** Every action driven by flags with zero prompts, for scripts and CI. A missing required flag fails clearly instead of prompting.

</details>

## Build from source

```sh
go build -o quackvps ./cmd/quackvps
```

Releases target `linux/amd64` and `linux/arm64`.
