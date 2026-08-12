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

Run with no flags for the wizard. These apply to the interactive run:

| Flag | Does |
|---|---|
| `--dir <path>` | Start the folder picker at a given parent directory. |
| `--version` | Print the version and exit. |
| `--help` | Print usage and exit. |

## Headless (scripts & CI)

Pass `--mode` to run without prompts. A missing or invalid flag exits non-zero with a clear message — it never falls back to a prompt. Before doing any work, quackvps checks the version and mods you asked for actually exist for your loader, so a typo fails fast instead of half-installing.

```sh
# Install a NeoForge server with a live map and voice chat, no domain (localhost + ssh -L).
sudo ./quackvps --mode install --parent /home/ubuntu/mc --instance survival \
  --loader neoforge --mcversion 1.21.8 --server-port 25565 --heap-min 2 --heap-max 4 \
  --bluemap --bluemap-port 8100 --voicechat --voicechat-port 24454

# Update that server to a newer Minecraft version (keeps your world and mods).
sudo ./quackvps --mode update --parent /home/ubuntu/mc --instance survival --mcversion 26.1.2

# Restore a world backup from the server's backups/ folder.
sudo ./quackvps --mode restore --parent /home/ubuntu/mc --instance survival \
  --backup world-20260610-161024.zip
```

Every run needs `--mode`, `--parent`, and `--instance`.

**Install** (`--mode install`):

| Flag | Does |
|---|---|
| `--loader` | `fabric`, `neoforge`, `forge`, `quilt`, or `vanilla`. |
| `--mcversion` | Minecraft version, e.g. `1.21.8`. |
| `--heap-min` / `--heap-max` | JVM heap in GB (`-Xms` / `-Xmx`). |
| `--server-port` | Minecraft game port. |
| `--modpack` | Modpack slug (optional). |
| `--dashboard` / `--votifier` / `--bluemap` / `--voicechat` | Enable an add-on; each needs its `-port`. |
| `--dashboard-port` / `--votifier-port` / `--bluemap-port` / `--voicechat-port` | Port for the matching add-on. |
| `--domain` / `--email` | Serve web add-ons at `<sub>.<domain>` over HTTPS instead of localhost. |
| `--dashboard-subdomain` / `--bluemap-subdomain` | Subdomain per web add-on (with `--domain`). |
| `--harden-ssh` / `--ssh-pubkey` | Harden SSH to key-only. Requires `--ssh-pubkey` or it refuses, so you can't lock yourself out. |

**Update** (`--mode update`):

| Flag | Does |
|---|---|
| `--mcversion` | Target Minecraft version. The loader is detected from disk, not passed. |
| `--empty-mods` | Start fresh with an empty `mods/` instead of upgrading the existing mods. |

**Restore** (`--mode restore`):

| Flag | Does |
|---|---|
| `--backup` | Backup zip to restore, by filename or full path, from the server's `backups/` folder. |

<details>
<summary>Roadmap (not yet available)</summary>

- **Remove a server.** Clean teardown: stop and disable its service, drop its firewall rules, remove its Caddy config, and delete its folder, after a confirmation.

</details>

## Build from source

```sh
go build -o quackvps ./cmd/quackvps
```

Releases target `linux/amd64` and `linux/arm64`.

## License

[GPLv3](LICENSE). You may use, run, and modify it freely; if you distribute a modified version, it must also be GPLv3 with source available.
