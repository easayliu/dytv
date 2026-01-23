# dytv

Live stream URL parser service.

## Quick Install

```bash
# Install latest version
curl -fsSL https://raw.githubusercontent.com/easayliu/dytv/main/install.sh | bash -s install

# Install and start service (default port 8080)
curl -fsSL https://raw.githubusercontent.com/easayliu/dytv/main/install.sh | bash -s service

# Install and start service (port 9000)
curl -fsSL https://raw.githubusercontent.com/easayliu/dytv/main/install.sh | PORT=9000 bash -s service
```

## Installation Options

### Install from Release

```bash
# Download script
curl -fsSL https://raw.githubusercontent.com/easayliu/dytv/main/install.sh -o install.sh
chmod +x install.sh

# Install binary
./install.sh install

# Install and create system service
./install.sh service

# Specify version and port
VERSION=v1.0.0 PORT=9000 ./install.sh service
```

### Install from Source

```bash
git clone https://github.com/easayliu/dytv.git
cd dytv
./install.sh install-source
```

## Command Reference

| Command | Description |
|---------|-------------|
| `install` | Download from release and install |
| `install-source` | Build from source and install |
| `service` | Download, install and create system service |
| `service-source` | Build from source and create system service |
| `set-port <port>` | Update service port and restart |
| `uninstall` | Remove binary and service |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | Service listen port |
| `VERSION` | latest | Version to install |
| `GITHUB_REPO` | easayliu/dytv | GitHub repository |

## Service Management

### Linux (systemd)

```bash
sudo systemctl start dytv    # Start
sudo systemctl stop dytv     # Stop
sudo systemctl restart dytv  # Restart
sudo systemctl status dytv   # Status
sudo journalctl -u dytv -f   # Logs
```

### macOS (launchd)

```bash
launchctl load ~/Library/LaunchAgents/com.dytv.plist    # Start
launchctl unload ~/Library/LaunchAgents/com.dytv.plist  # Stop
```

## Change Port

```bash
# Option 1: Use script command
./install.sh set-port 9000

# Option 2: Reinstall service
PORT=9000 ./install.sh service
```

## Uninstall

```bash
./install.sh uninstall
```

## License

GPL-3.0
