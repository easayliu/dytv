package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/easayliu/dytv/internal/api"
	"github.com/easayliu/dytv/internal/config"
	"github.com/easayliu/dytv/internal/model"
	qrcode "github.com/skip2/go-qrcode"
)

var (
	douyuClient    = api.NewDouyuClient()
	bilibiliClient = api.NewBilibiliClient()
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Douyu routes (default platform)
	mux.HandleFunc("/live/douyu/", handleDouyuLive)
	mux.HandleFunc("/live/", handleLive) // Default to Douyu for backward compatibility

	// Bilibili routes
	mux.HandleFunc("/live/bilibili/", handleBilibiliLive)

	// Login routes
	mux.HandleFunc("/login/qrcode", handleLoginQRCode)
	mux.HandleFunc("/login/poll", handleLoginPoll)
	mux.HandleFunc("/login", handleLoginPage)

	mux.HandleFunc("/playlist.m3u", handlePlaylist)
	mux.HandleFunc("/playlist/douyu/rec.m3u", handleDouyuRecPlaylist)
	mux.HandleFunc("/playlist/douyu/follow.m3u", handleDouyuFollowPlaylist)
	mux.HandleFunc("/health", handleHealth)

	addr := ":" + port
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	fmt.Printf("Starting server on %s\n", addr)
	fmt.Println("Endpoints:")
	fmt.Println("  Douyu (斗鱼):")
	fmt.Println("    GET /live/{room_id}              - Redirect to FLV stream (default: Douyu)")
	fmt.Println("    GET /live/douyu/{room_id}        - Redirect to Douyu FLV stream")
	fmt.Println("    GET /live/douyu/{room_id}?format=json  - Return stream info as JSON")
	fmt.Println("    GET /live/douyu/{room_id}?rate=0 - Specify quality (0=原画, 2=超清, 3=流畅)")
	fmt.Println("  Bilibili (哔哩哔哩):")
	fmt.Println("    GET /live/bilibili/{room_id}     - Redirect to Bilibili FLV stream")
	fmt.Println("    GET /live/bilibili/{room_id}?format=json  - Return stream info as JSON")
	fmt.Println("    GET /live/bilibili/{room_id}?qn=10000     - Specify quality (80=流畅, 150=高清, 400=蓝光, 10000=原画)")
	fmt.Println("  Playlist:")
	fmt.Println("    GET /playlist.m3u?rooms=1,2,3&platform=douyu   - Generate M3U playlist")
	fmt.Println("    GET /playlist.m3u?rooms=1,2,3&platform=bilibili")
	fmt.Println("    GET /playlist/douyu/rec.m3u                    - Douyu recommended rooms playlist")
	fmt.Println("    GET /playlist/douyu/follow.m3u                 - Douyu followed rooms playlist")
	fmt.Println("  Login (登录):")
	fmt.Println("    GET /login                       - Douyu QR code login page")
	fmt.Println("  Health:")
	fmt.Println("    GET /health                      - Health check")
	fmt.Println()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-quit
	fmt.Println("\nShutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server forced to shutdown: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Server stopped")
}

// handleLive handles default live requests (backward compatible, defaults to Douyu)
func handleLive(w http.ResponseWriter, r *http.Request) {
	// 提取 roomID 后委托给斗鱼 handler
	path := strings.TrimPrefix(r.URL.Path, "/live/")
	roomID := strings.TrimSuffix(path, "/")

	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	serveDouyuStream(w, r, roomID)
}

// handleDouyuLive handles Douyu-specific live requests
func handleDouyuLive(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/live/douyu/")
	roomID := strings.TrimSuffix(path, "/")

	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	serveDouyuStream(w, r, roomID)
}

// serveDouyuStream is the shared logic for Douyu stream handlers
func serveDouyuStream(w http.ResponseWriter, r *http.Request, roomID string) {
	rate := 0
	if rateStr := r.URL.Query().Get("rate"); rateStr != "" {
		fmt.Sscanf(rateStr, "%d", &rate)
	} else if q := r.URL.Query().Get("quality"); q != "" {
		if rateVal, ok := model.QualityMap[strings.ToLower(q)]; ok {
			rate = rateVal
		}
	}

	info, err := douyuClient.GetStreamURL(roomID, rate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !info.IsLive || info.StreamURL == "" {
		http.Error(w, "room is offline", http.StatusNotFound)
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"room_id":    info.RoomID,
			"platform":   "douyu",
			"is_live":    info.IsLive,
			"flv_url":    info.FlvURL,
			"stream_url": info.StreamURL,
			"multirates": info.Multirates,
		})
		return
	}

	http.Redirect(w, r, info.StreamURL, http.StatusFound)
}

// handleBilibiliLive handles Bilibili-specific live requests
func handleBilibiliLive(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/live/bilibili/")
	roomID := strings.TrimSuffix(path, "/")

	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	// Parse quality parameter (qn for Bilibili)
	qn := 0
	if qnStr := r.URL.Query().Get("qn"); qnStr != "" {
		fmt.Sscanf(qnStr, "%d", &qn)
	} else if rateStr := r.URL.Query().Get("rate"); rateStr != "" {
		fmt.Sscanf(rateStr, "%d", &qn)
	} else if q := r.URL.Query().Get("quality"); q != "" {
		if qnVal, ok := model.BilibiliQualityMap[strings.ToLower(q)]; ok {
			qn = qnVal
		}
	}

	info, err := bilibiliClient.GetStreamURL(roomID, qn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !info.IsLive || info.StreamURL == "" {
		http.Error(w, "room is offline", http.StatusNotFound)
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"room_id":    info.RoomID,
			"platform":   "bilibili",
			"is_live":    info.IsLive,
			"flv_url":    info.FlvURL,
			"stream_url": info.StreamURL,
			"multirates": info.Multirates,
		})
		return
	}

	http.Redirect(w, r, info.StreamURL, http.StatusFound)
}

func handlePlaylist(w http.ResponseWriter, r *http.Request) {
	roomsParam := r.URL.Query().Get("rooms")
	if roomsParam == "" {
		http.Error(w, "rooms parameter is required (e.g., ?rooms=1,2,3)", http.StatusBadRequest)
		return
	}

	roomIDs := strings.Split(roomsParam, ",")

	// 获取平台参数，默认为 douyu
	platform := strings.ToLower(r.URL.Query().Get("platform"))
	if platform == "" {
		platform = "douyu"
	}

	// 获取基础 URL 用于构建代理地址
	baseURL := r.URL.Query().Get("base_url")
	if baseURL == "" {
		scheme := "http"
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else if r.TLS != nil {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 构建 M3U 播放列表
	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n")
	var filename string

	switch platform {
	case "bilibili":
		filename = "bilibili.m3u"
		// 获取清晰度参数
		qn := 0
		if qnStr := r.URL.Query().Get("qn"); qnStr != "" {
			fmt.Sscanf(qnStr, "%d", &qn)
		} else if rateStr := r.URL.Query().Get("rate"); rateStr != "" {
			fmt.Sscanf(rateStr, "%d", &qn)
		} else if q := r.URL.Query().Get("quality"); q != "" {
			if qnVal, ok := model.BilibiliQualityMap[strings.ToLower(q)]; ok {
				qn = qnVal
			}
		}

		for _, roomID := range roomIDs {
			roomID = strings.TrimSpace(roomID)
			if roomID == "" {
				continue
			}

			info, err := bilibiliClient.GetStreamURL(roomID, qn)
			if err != nil {
				continue
			}

			if !info.IsLive || info.FlvURL == "" {
				continue
			}

			channelName := fmt.Sprintf("哔哩哔哩-%s", info.RoomID)
			if info.RoomName != "" {
				channelName = info.RoomName
			}

			// 使用代理地址
			proxyURL := fmt.Sprintf("%s/live/bilibili/%s", baseURL, info.RoomID)
			if qn > 0 {
				proxyURL = fmt.Sprintf("%s?qn=%d", proxyURL, qn)
			}

			playlist.WriteString(fmt.Sprintf("#EXTINF:-1,%s\n", channelName))
			playlist.WriteString(proxyURL + "\n")
		}

	default: // douyu
		filename = "douyu.m3u"
		// 获取清晰度参数
		rate := 0
		if rateStr := r.URL.Query().Get("rate"); rateStr != "" {
			fmt.Sscanf(rateStr, "%d", &rate)
		} else if q := r.URL.Query().Get("quality"); q != "" {
			if rateVal, ok := model.QualityMap[strings.ToLower(q)]; ok {
				rate = rateVal
			}
		}

		for _, roomID := range roomIDs {
			roomID = strings.TrimSpace(roomID)
			if roomID == "" {
				continue
			}

			info, err := douyuClient.GetStreamURL(roomID, rate)
			if err != nil {
				continue
			}

			if !info.IsLive || info.FlvURL == "" {
				continue
			}

			channelName := fmt.Sprintf("斗鱼-%s", info.RoomID)
			if info.RoomName != "" {
				channelName = info.RoomName
			}

			// 使用代理地址
			proxyURL := fmt.Sprintf("%s/live/%s", baseURL, info.RoomID)
			if rate > 0 {
				proxyURL = fmt.Sprintf("%s?rate=%d", proxyURL, rate)
			}

			playlist.WriteString(fmt.Sprintf("#EXTINF:-1,%s\n", channelName))
			playlist.WriteString(proxyURL + "\n")
		}
	}

	w.Header().Set("Content-Type", "audio/x-mpegurl")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	_, _ = w.Write([]byte(playlist.String()))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleDouyuRecPlaylist(w http.ResponseWriter, r *http.Request) {
	// 优先从请求头获取 cookie，避免 URL 泄露凭据
	cookie := r.Header.Get("X-Douyu-Cookie")
	if cookie == "" {
		cookie = os.Getenv("DOUYU_COOKIE")
	}
	if cookie == "" {
		if saved, err := config.LoadCookie(); err == nil && saved != "" {
			cookie = saved
		}
	}
	if cookie == "" {
		http.Error(w, "cookie is required (X-Douyu-Cookie header, DOUYU_COOKIE env, or login via /login)", http.StatusBadRequest)
		return
	}

	items, err := douyuClient.SearchRecommend(cookie)
	if err != nil {
		http.Error(w, "failed to get recommendations", http.StatusInternalServerError)
		return
	}

	baseURL := r.URL.Query().Get("base_url")
	if baseURL == "" {
		scheme := "http"
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else if r.TLS != nil {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n")

	for _, item := range items {
		if item.ShowStatus != 1 {
			continue
		}
		roomID := strconv.Itoa(item.BizID)
		proxyURL := fmt.Sprintf("%s/live/douyu/%s", baseURL, roomID)
		// 过滤换行符防止 M3U 内容注入
		name := strings.NewReplacer("\n", "", "\r", "").Replace(item.Keyword)
		playlist.WriteString(fmt.Sprintf("#EXTINF:-1,%s\n", name))
		playlist.WriteString(proxyURL + "\n")
	}

	w.Header().Set("Content-Type", "audio/x-mpegurl")
	w.Header().Set("Content-Disposition", `attachment; filename="douyu_rec.m3u"`)
	_, _ = w.Write([]byte(playlist.String()))
}

func handleDouyuFollowPlaylist(w http.ResponseWriter, r *http.Request) {
	cookie := r.Header.Get("X-Douyu-Cookie")
	if cookie == "" {
		cookie = os.Getenv("DOUYU_COOKIE")
	}
	if cookie == "" {
		if saved, err := config.LoadCookie(); err == nil && saved != "" {
			cookie = saved
		}
	}
	if cookie == "" {
		http.Error(w, "cookie is required (X-Douyu-Cookie header, DOUYU_COOKIE env, or login via /login)", http.StatusBadRequest)
		return
	}

	rooms, err := douyuClient.FollowList(cookie)
	if err != nil {
		http.Error(w, "failed to get follow list: "+err.Error(), http.StatusInternalServerError)
		return
	}

	online := 0
	for _, r := range rooms {
		if r.ShowStatus == 1 {
			online++
		}
	}
	fmt.Printf("[follow] total=%d online=%d\n", len(rooms), online)

	baseURL := r.URL.Query().Get("base_url")
	if baseURL == "" {
		scheme := "http"
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else if r.TLS != nil {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n")

	for _, room := range rooms {
		if room.ShowStatus != 1 {
			continue
		}
		roomID := strconv.Itoa(room.RoomID)
		proxyURL := fmt.Sprintf("%s/live/douyu/%s", baseURL, roomID)
		name := room.Nickname
		if room.RoomName != "" {
			name = room.RoomName
		}
		name = strings.NewReplacer("\n", "", "\r", "").Replace(name)
		logo := room.RoomSrc
		if logo == "" {
			logo = room.AvatarSmall
		}
		if logo != "" {
			playlist.WriteString(fmt.Sprintf("#EXTINF:-1 tvg-logo=\"%s\",%s\n", logo, name))
		} else {
			playlist.WriteString(fmt.Sprintf("#EXTINF:-1,%s\n", name))
		}
		playlist.WriteString(proxyURL + "\n")
	}

	w.Header().Set("Content-Type", "audio/x-mpegurl")
	w.Header().Set("Content-Disposition", `attachment; filename="douyu_follow.m3u"`)
	_, _ = w.Write([]byte(playlist.String()))
}

// handleLoginPage serves the QR code login HTML page.
func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginHTML))
}

// handleLoginQRCode generates a new QR code for login.
func handleLoginQRCode(w http.ResponseWriter, r *http.Request) {
	result, err := api.GenerateQRCode()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	png, err := qrcode.Encode(result.URL, qrcode.Medium, 256)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to generate qrcode image",
		})
		return
	}

	image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code":   result.Code,
		"image":  image,
		"expire": result.Expire,
	})
}

// handleLoginPoll polls the QR code scan status.
func handleLoginPoll(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status": "error", "message": "code parameter is required",
		})
		return
	}

	status, err := api.CheckQRStatus(code)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"status": "error", "message": err.Error(),
		})
		return
	}

	if status.Status == "success" && status.URL != "" {
		cookie, err := api.DoLoginCallback(status.URL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error", "message": "login callback failed: " + err.Error(),
			})
			return
		}
		if err := config.SaveCookie(cookie); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error", "message": "failed to save cookie: " + err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "success", "message": "登录成功，cookie 已保存",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": status.Status, "message": status.Message,
	})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

const loginHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>斗鱼扫码登录 - dytv</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
display:flex;justify-content:center;align-items:center;min-height:100vh;
background:#f5f5f5;color:#333}
@media(prefers-color-scheme:dark){
body{background:#1a1a1a;color:#e0e0e0}
.card{background:#2a2a2a;box-shadow:0 2px 12px rgba(0,0,0,.4)}
}
.card{background:#fff;border-radius:12px;padding:32px;text-align:center;
box-shadow:0 2px 12px rgba(0,0,0,.1);max-width:360px;width:90%}
h1{font-size:20px;margin-bottom:8px}
.subtitle{font-size:14px;color:#888;margin-bottom:24px}
#qr-img{width:256px;height:256px;margin:0 auto 16px;border-radius:8px;
display:block;image-rendering:pixelated}
#status{font-size:15px;min-height:24px;margin-bottom:16px}
.btn{display:inline-block;padding:10px 24px;border:none;border-radius:6px;
background:#ff5d23;color:#fff;font-size:15px;cursor:pointer}
.btn:hover{background:#e64d18}
.hidden{display:none}
.success{color:#52c41a}
.scanned{color:#1890ff}
</style>
</head>
<body>
<div class="card">
<h1>斗鱼扫码登录</h1>
<p class="subtitle">使用斗鱼 APP 扫描二维码</p>
<img id="qr-img" class="hidden" alt="QR Code">
<div id="loading">加载中...</div>
<p id="status"></p>
<button id="refresh-btn" class="btn hidden" onclick="fetchQR()">重新生成</button>
</div>
<script>
let pollTimer=null,currentCode="";
async function fetchQR(){
  clearInterval(pollTimer);
  document.getElementById("loading").textContent="加载中...";
  document.getElementById("loading").classList.remove("hidden");
  document.getElementById("qr-img").classList.add("hidden");
  document.getElementById("refresh-btn").classList.add("hidden");
  document.getElementById("status").textContent="";
  document.getElementById("status").className="";
  try{
    const r=await fetch("/login/qrcode");
    const d=await r.json();
    if(d.error){document.getElementById("loading").textContent="错误: "+d.error;return}
    currentCode=d.code;
    document.getElementById("qr-img").src=d.image;
    document.getElementById("qr-img").classList.remove("hidden");
    document.getElementById("loading").classList.add("hidden");
    document.getElementById("status").textContent="等待扫码...";
    pollTimer=setInterval(pollStatus,3000);
  }catch(e){document.getElementById("loading").textContent="请求失败，请刷新页面"}
}
async function pollStatus(){
  try{
    const r=await fetch("/login/poll?code="+encodeURIComponent(currentCode));
    const d=await r.json();
    const st=document.getElementById("status");
    console.log("poll:",d);
    if(d.status==="success"){
      clearInterval(pollTimer);
      st.textContent=d.message;
      st.className="success";
      document.getElementById("qr-img").style.opacity="0.3";
    }else if(d.status==="scanned"){
      st.textContent=d.message;
      st.className="scanned";
    }else if(d.status==="expired"){
      clearInterval(pollTimer);
      st.textContent=d.message;
      st.className="";
      document.getElementById("refresh-btn").classList.remove("hidden");
    }else if(d.status==="error"){
      st.textContent=d.message;
      st.className="";
    }else{
      st.textContent=d.message||"等待扫码...";
    }
  }catch(e){console.error("poll error:",e)}
}
fetchQR();
</script>
</body>
</html>`
