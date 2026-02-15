#!/bin/bash

set -e

APP_NAME="dytv"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="dytv"
PORT="${PORT:-8080}"
VERSION="${VERSION:-latest}"
DOUYU_COOKIE="${DOUYU_COOKIE:-}"

# GitHub repository
GITHUB_REPO="${GITHUB_REPO:-easayliu/dytv}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

is_service_active() {
    if [[ -f /etc/systemd/system/${SERVICE_NAME}.service ]]; then
        systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null && return 0
    fi
    PLIST_PATH="$HOME/Library/LaunchAgents/com.dytv.plist"
    if [[ -f "${PLIST_PATH}" ]]; then
        launchctl list 2>/dev/null | grep -q "com.dytv" && return 0
    fi
    return 1
}

detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"

    case "${OS}" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      error "Unsupported OS: ${OS}" ;;
    esac

    case "${ARCH}" in
        x86_64|amd64)  ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *)             error "Unsupported architecture: ${ARCH}" ;;
    esac

    BINARY_NAME="${APP_NAME}-${OS}-${ARCH}"
    log "Detected platform: ${OS}/${ARCH}"
}

get_latest_version() {
    if [[ "${VERSION}" == "latest" ]]; then
        log "Fetching latest version..."
        VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | \
            grep -o '"tag_name": "[^"]*"' | cut -d'"' -f4)
        if [[ -z "${VERSION}" ]]; then
            error "Failed to get latest version. Please specify VERSION manually."
        fi
    fi
    log "Version: ${VERSION}"
}

download_binary() {
    detect_platform
    get_latest_version

    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${BINARY_NAME}"
    TMP_FILE="/tmp/${BINARY_NAME}"

    log "Downloading from: ${DOWNLOAD_URL}"
    curl -fsSL -o "${TMP_FILE}" "${DOWNLOAD_URL}" || error "Download failed"

    chmod +x "${TMP_FILE}"
    log "Downloaded to ${TMP_FILE}"
}

install_binary() {
    local was_active=false
    if is_service_active; then
        was_active=true
        log "Service is running, stopping before binary replacement..."
        stop_service
    fi

    log "Installing to ${INSTALL_DIR}..."
    sudo mv "${TMP_FILE}" "${INSTALL_DIR}/${APP_NAME}"
    sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"
    log "Installed to ${INSTALL_DIR}/${APP_NAME}"

    if [[ "${was_active}" == true ]]; then
        log "Restarting service with new binary..."
        start_service
    fi
}

check_go() {
    if ! command -v go &> /dev/null; then
        error "Go is not installed. Please install Go first: https://golang.org/dl/"
    fi
    log "Go version: $(go version)"
}

build() {
    log "Building ${APP_NAME}..."
    go build -o ${APP_NAME} ./cmd/dytv/
    log "Build successful: ./${APP_NAME}"
}

install_from_source() {
    check_go
    build
    TMP_FILE="./${APP_NAME}"
    install_binary
}

install_systemd() {
    if [[ ! -d /etc/systemd/system ]]; then
        warn "systemd not found, skipping service installation"
        return
    fi

    log "Creating config directory /etc/dytv..."
    sudo mkdir -p /etc/dytv
    sudo chown nobody:nogroup /etc/dytv
    sudo chmod 700 /etc/dytv

    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"
    local effective_port="${PORT}"
    local effective_cookie="${DOUYU_COOKIE}"

    # 覆盖安装时保留已有配置（除非用户显式指定了新值）
    if [[ -f "${service_file}" ]]; then
        log "Existing service file found, preserving configuration..."
        if [[ "${PORT}" == "8080" && -z "${DYTV_PORT_SET:-}" ]]; then
            local existing_port
            existing_port=$(grep -oP 'Environment=PORT=\K.*' "${service_file}" 2>/dev/null || echo "")
            if [[ -n "${existing_port}" ]]; then
                effective_port="${existing_port}"
            fi
        fi
        if [[ -z "${DOUYU_COOKIE}" ]]; then
            local existing_cookie
            existing_cookie=$(grep -oP 'Environment=DOUYU_COOKIE=\K.*' "${service_file}" 2>/dev/null || echo "")
            if [[ -n "${existing_cookie}" ]]; then
                effective_cookie="${existing_cookie}"
            fi
        fi
    fi

    log "Creating systemd service (port: ${effective_port})..."
    sudo tee "${service_file}" > /dev/null <<EOF
[Unit]
Description=Live Stream URL Service
After=network.target

[Service]
Type=simple
User=nobody
Environment=PORT=${effective_port}
Environment=DOUYU_COOKIE=${effective_cookie}
Environment=DYTV_CONFIG_DIR=/etc/dytv
ExecStart=${INSTALL_DIR}/${APP_NAME}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable ${SERVICE_NAME}
    sudo systemctl restart ${SERVICE_NAME}
    log "Service started: ${SERVICE_NAME}"
    log "Status: sudo systemctl status ${SERVICE_NAME}"
}

install_launchd() {
    if [[ "$(uname)" != "Darwin" ]]; then
        return
    fi

    PLIST_PATH="$HOME/Library/LaunchAgents/com.dytv.plist"
    mkdir -p "$HOME/Library/LaunchAgents"

    local effective_port="${PORT}"
    local effective_cookie="${DOUYU_COOKIE}"
    local config_dir="$HOME/.config/dytv"

    # 覆盖安装时保留已有配置（除非用户显式指定了新值）
    if [[ -f "${PLIST_PATH}" ]]; then
        log "Existing plist found, preserving configuration..."
        launchctl unload "${PLIST_PATH}" 2>/dev/null || true

        if [[ "${PORT}" == "8080" && -z "${DYTV_PORT_SET:-}" ]]; then
            local existing_port
            existing_port=$(/usr/libexec/PlistBuddy -c "Print :EnvironmentVariables:PORT" "${PLIST_PATH}" 2>/dev/null || echo "")
            if [[ -n "${existing_port}" ]]; then
                effective_port="${existing_port}"
            fi
        fi
        if [[ -z "${DOUYU_COOKIE}" ]]; then
            local existing_cookie
            existing_cookie=$(/usr/libexec/PlistBuddy -c "Print :EnvironmentVariables:DOUYU_COOKIE" "${PLIST_PATH}" 2>/dev/null || echo "")
            if [[ -n "${existing_cookie}" ]]; then
                effective_cookie="${existing_cookie}"
            fi
        fi
    fi

    log "Creating launchd service (port: ${effective_port})..."
    cat > "${PLIST_PATH}" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.dytv</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/${APP_NAME}</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PORT</key>
        <string>${effective_port}</string>
        <key>DOUYU_COOKIE</key>
        <string>${effective_cookie}</string>
        <key>DYTV_CONFIG_DIR</key>
        <string>${config_dir}</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
EOF

    launchctl load "${PLIST_PATH}"
    log "Service started: ${PLIST_PATH}"
    log "Status: launchctl list | grep dytv"
}

uninstall() {
    log "Uninstalling ${APP_NAME}..."

    # Stop and remove systemd service
    if [[ -f /etc/systemd/system/${SERVICE_NAME}.service ]]; then
        sudo systemctl stop ${SERVICE_NAME} 2>/dev/null || true
        sudo systemctl disable ${SERVICE_NAME} 2>/dev/null || true
        sudo rm -f /etc/systemd/system/${SERVICE_NAME}.service
        sudo systemctl daemon-reload
        log "Removed systemd service"
    fi

    # Remove launchd service (macOS)
    PLIST_PATH="$HOME/Library/LaunchAgents/com.dytv.plist"
    if [[ -f "${PLIST_PATH}" ]]; then
        launchctl unload "${PLIST_PATH}" 2>/dev/null || true
        rm -f "${PLIST_PATH}"
        log "Removed launchd service"
    fi

    # Remove config directory
    if [[ -d /etc/dytv ]]; then
        sudo rm -rf /etc/dytv
        log "Removed /etc/dytv"
    fi

    # Remove binary
    if [[ -f "${INSTALL_DIR}/${APP_NAME}" ]]; then
        sudo rm -f "${INSTALL_DIR}/${APP_NAME}"
        log "Removed ${INSTALL_DIR}/${APP_NAME}"
    fi

    log "Uninstall complete"
}

update_port() {
    local new_port="$1"
    if [[ -z "${new_port}" ]]; then
        error "Please specify a port number"
    fi

    if [[ -f /etc/systemd/system/${SERVICE_NAME}.service ]]; then
        sudo sed -i "s/Environment=PORT=.*/Environment=PORT=${new_port}/" /etc/systemd/system/${SERVICE_NAME}.service
        sudo systemctl daemon-reload
        sudo systemctl restart ${SERVICE_NAME}
        log "Updated systemd service port to ${new_port}"
    fi

    PLIST_PATH="$HOME/Library/LaunchAgents/com.dytv.plist"
    if [[ -f "${PLIST_PATH}" ]]; then
        sed -i '' "/<key>PORT<\/key>/{ n; s|<string>[0-9]*</string>|<string>${new_port}</string>|; }" "${PLIST_PATH}"
        launchctl unload "${PLIST_PATH}" 2>/dev/null || true
        launchctl load "${PLIST_PATH}"
        log "Updated launchd service port to ${new_port}"
    fi
}

update_cookie() {
    local new_cookie="$1"
    if [[ -z "${new_cookie}" ]]; then
        error "Please specify a cookie value"
    fi

    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"
    if [[ -f "${service_file}" ]]; then
        # 使用 awk 安全替换，避免 cookie 中特殊字符破坏 sed
        local tmp_file
        tmp_file="$(mktemp)"
        if grep -q "Environment=DOUYU_COOKIE=" "${service_file}"; then
            sudo awk -v cookie="${new_cookie}" '{
                if ($0 ~ /^Environment=DOUYU_COOKIE=/) print "Environment=DOUYU_COOKIE=" cookie;
                else print
            }' "${service_file}" > "${tmp_file}" && sudo mv "${tmp_file}" "${service_file}"
        else
            sudo awk -v cookie="${new_cookie}" '{
                print;
                if ($0 ~ /^\[Service\]/) print "Environment=DOUYU_COOKIE=" cookie
            }' "${service_file}" > "${tmp_file}" && sudo mv "${tmp_file}" "${service_file}"
        fi
        sudo systemctl daemon-reload
        sudo systemctl restart ${SERVICE_NAME}
        log "Updated DOUYU_COOKIE and restarted service"
    fi

    PLIST_PATH="$HOME/Library/LaunchAgents/com.dytv.plist"
    if [[ -f "${PLIST_PATH}" ]]; then
        # 使用 python3 安全操作 plist，避免 sed 特殊字符问题
        python3 -c "
import plistlib, sys
with open('${PLIST_PATH}', 'rb') as f:
    pl = plistlib.load(f)
pl.setdefault('EnvironmentVariables', {})['DOUYU_COOKIE'] = sys.argv[1]
with open('${PLIST_PATH}', 'wb') as f:
    plistlib.dump(pl, f)
" "${new_cookie}"
        launchctl unload "${PLIST_PATH}" 2>/dev/null || true
        launchctl load "${PLIST_PATH}"
        log "Updated DOUYU_COOKIE and restarted service"
    fi
}

start_service() {
    if [[ -f /etc/systemd/system/${SERVICE_NAME}.service ]]; then
        sudo systemctl start ${SERVICE_NAME}
        log "Service started"
    fi

    PLIST_PATH="$HOME/Library/LaunchAgents/com.dytv.plist"
    if [[ -f "${PLIST_PATH}" ]]; then
        launchctl load "${PLIST_PATH}" 2>/dev/null || true
        log "Service started"
    fi
}

stop_service() {
    if [[ -f /etc/systemd/system/${SERVICE_NAME}.service ]]; then
        sudo systemctl stop ${SERVICE_NAME}
        log "Service stopped"
    fi

    PLIST_PATH="$HOME/Library/LaunchAgents/com.dytv.plist"
    if [[ -f "${PLIST_PATH}" ]]; then
        launchctl unload "${PLIST_PATH}" 2>/dev/null || true
        log "Service stopped"
    fi
}

restart_service() {
    if [[ -f /etc/systemd/system/${SERVICE_NAME}.service ]]; then
        sudo systemctl restart ${SERVICE_NAME}
        log "Service restarted"
    fi

    PLIST_PATH="$HOME/Library/LaunchAgents/com.dytv.plist"
    if [[ -f "${PLIST_PATH}" ]]; then
        launchctl unload "${PLIST_PATH}" 2>/dev/null || true
        launchctl load "${PLIST_PATH}"
        log "Service restarted"
    fi
}

status_service() {
    if [[ -f /etc/systemd/system/${SERVICE_NAME}.service ]]; then
        sudo systemctl status ${SERVICE_NAME} --no-pager
        return
    fi

    PLIST_PATH="$HOME/Library/LaunchAgents/com.dytv.plist"
    if [[ -f "${PLIST_PATH}" ]]; then
        launchctl list | grep -E "PID|dytv" || log "Service not running"
        return
    fi

    warn "Service not installed"
}

usage() {
    cat <<EOF
Usage: $0 <command> [options]

Commands:
  install          Download from release and install to ${INSTALL_DIR}
  install-source   Build from source and install
  service          Download, install, create and start system service
  service-source   Build from source, install, create and start system service
  upgrade          Upgrade binary and restart service (preserves config)
  uninstall        Remove binary and service
  start            Start the service
  stop             Stop the service
  restart          Restart the service
  status           Show service status
  set-port <port>      Update service port and restart
  set-cookie <cookie>  Update DOUYU_COOKIE and restart

Environment Variables:
  PORT             Server port (default: 8080)
  VERSION          Release version (default: latest)
  GITHUB_REPO      GitHub repository (default: easayliu/dytv)
  DOUYU_COOKIE     Douyu cookie for search recommend API

Examples:
  # Install latest release
  $0 install

  # Install specific version
  VERSION=v1.0.0 $0 install

  # Install and start service on port 9000 with cookie
  PORT=9000 DOUYU_COOKIE='your_cookie' $0 service

  # Build from source and install
  $0 install-source

  # Service management
  $0 start
  $0 stop
  $0 restart
  $0 status

  # Change service port
  $0 set-port 9000

  # Update cookie
  $0 set-cookie 'your_cookie_value'

  # Upgrade to latest version (preserves config)
  $0 upgrade

  # Uninstall
  $0 uninstall
EOF
}

case "${1:-}" in
    install)
        download_binary
        install_binary
        log "Installation complete. Run with: ${APP_NAME}"
        ;;
    install-source)
        install_from_source
        log "Installation complete. Run with: ${APP_NAME}"
        ;;
    service)
        download_binary
        install_binary
        if [[ "$(uname)" == "Darwin" ]]; then
            install_launchd
        else
            install_systemd
        fi
        log "Service is running at http://localhost:${PORT}"
        ;;
    service-source)
        install_from_source
        if [[ "$(uname)" == "Darwin" ]]; then
            install_launchd
        else
            install_systemd
        fi
        log "Service is running at http://localhost:${PORT}"
        ;;
    upgrade)
        log "Upgrading ${APP_NAME}..."
        download_binary
        install_binary
        if [[ "$(uname)" == "Darwin" ]]; then
            install_launchd
        else
            install_systemd
        fi
        log "Upgrade complete! Service is running at http://localhost:${PORT}"
        ;;
    uninstall)
        uninstall
        ;;
    start)
        start_service
        ;;
    stop)
        stop_service
        ;;
    restart)
        restart_service
        ;;
    status)
        status_service
        ;;
    set-port)
        update_port "$2"
        ;;
    set-cookie)
        update_cookie "$2"
        ;;
    *)
        usage
        ;;
esac
