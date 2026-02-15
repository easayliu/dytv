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
| `set-cookie <cookie>` | Update DOUYU_COOKIE and restart |
| `uninstall` | Remove binary and service |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | Service listen port |
| `VERSION` | latest | Version to install |
| `GITHUB_REPO` | easayliu/dytv | GitHub repository |
| `DOUYU_COOKIE` | | Douyu cookie for search recommend API |

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

## API Endpoints

### Douyu (斗鱼)

| Endpoint | Description |
|----------|-------------|
| `GET /live/{room_id}` | Redirect to FLV stream (default: Douyu) |
| `GET /live/douyu/{room_id}` | Redirect to Douyu FLV stream |
| `GET /live/douyu/{room_id}?format=json` | Return stream info as JSON |
| `GET /live/douyu/{room_id}?rate=0` | Specify quality (0=原画, 2=超清, 3=流畅) |

### Bilibili (哔哩哔哩)

| Endpoint | Description |
|----------|-------------|
| `GET /live/bilibili/{room_id}` | Redirect to Bilibili FLV stream |
| `GET /live/bilibili/{room_id}?format=json` | Return stream info as JSON |
| `GET /live/bilibili/{room_id}?qn=10000` | Specify quality (80=流畅, 150=高清, 400=蓝光, 10000=原画) |

### Playlist

| Endpoint | Description |
|----------|-------------|
| `GET /playlist.m3u?rooms=1,2,3&platform=douyu` | Generate M3U playlist (Douyu) |
| `GET /playlist.m3u?rooms=1,2,3&platform=bilibili` | Generate M3U playlist (Bilibili) |
| `GET /playlist/douyu/rec.m3u` | Douyu recommended rooms playlist |
| `GET /playlist/douyu/follow.m3u` | Douyu followed rooms playlist (requires cookie) |

### Login (登录)

| Endpoint | Description |
|----------|-------------|
| `GET /login` | Douyu QR code login page |

### Health

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check |

## Douyu Login & Cookie

### 扫码登录（推荐）

访问 `http://localhost:8080/login`，用斗鱼 APP 扫码登录，cookie 自动保存到 `~/.config/dytv/cookie`。

适合无浏览器的服务器——从任意设备的浏览器访问即可完成登录。

### Cookie 优先级

请求需要 cookie 的端点（`rec.m3u`、`follow.m3u`）时，按以下顺序查找：

1. 请求头 `X-Douyu-Cookie`
2. 环境变量 `DOUYU_COOKIE`
3. 文件 `~/.config/dytv/cookie`（扫码登录自动保存）

### 手动配置 Cookie

```bash
# 方式 1: 安装时设置
DOUYU_COOKIE='your_cookie' ./install.sh service

# 方式 2: 运行后更新
./install.sh set-cookie 'your_cookie'

# 方式 3: 直接运行
DOUYU_COOKIE='your_cookie' dytv
```

### 请求方式

```bash
# 关注列表（使用已保存的 cookie）
curl http://localhost:8080/playlist/douyu/follow.m3u

# 搜索推荐
curl http://localhost:8080/playlist/douyu/rec.m3u

# 使用请求头传入 cookie
curl -H "X-Douyu-Cookie: your_cookie" http://localhost:8080/playlist/douyu/follow.m3u
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
