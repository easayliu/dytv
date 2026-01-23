package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/easayliu/dytv/internal/api"
	"github.com/easayliu/dytv/internal/model"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Douyu routes (default platform)
	http.HandleFunc("/live/douyu/", handleDouyuLive)
	http.HandleFunc("/live/", handleLive) // Default to Douyu for backward compatibility

	// Bilibili routes
	http.HandleFunc("/live/bilibili/", handleBilibiliLive)

	http.HandleFunc("/playlist.m3u", handlePlaylist)
	http.HandleFunc("/health", handleHealth)

	addr := ":" + port
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
	fmt.Println("  Health:")
	fmt.Println("    GET /health                      - Health check")
	fmt.Println()

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

// handleLive handles default live requests (backward compatible, defaults to Douyu)
func handleLive(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/live/")

	// Check if it's a platform-specific route (handled by other handlers)
	if strings.HasPrefix(path, "douyu/") || strings.HasPrefix(path, "bilibili/") {
		return
	}

	roomID := strings.TrimSuffix(path, "/")

	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	rate := 0
	if rateStr := r.URL.Query().Get("rate"); rateStr != "" {
		fmt.Sscanf(rateStr, "%d", &rate)
	} else if q := r.URL.Query().Get("quality"); q != "" {
		if rateVal, ok := model.QualityMap[strings.ToLower(q)]; ok {
			rate = rateVal
		}
	}

	client := api.NewDouyuClient()
	info, err := client.GetStreamURL(roomID, rate)
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

// handleDouyuLive handles Douyu-specific live requests
func handleDouyuLive(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/live/douyu/")
	roomID := strings.TrimSuffix(path, "/")

	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	rate := 0
	if rateStr := r.URL.Query().Get("rate"); rateStr != "" {
		fmt.Sscanf(rateStr, "%d", &rate)
	} else if q := r.URL.Query().Get("quality"); q != "" {
		if rateVal, ok := model.QualityMap[strings.ToLower(q)]; ok {
			rate = rateVal
		}
	}

	client := api.NewDouyuClient()
	info, err := client.GetStreamURL(roomID, rate)
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

	client := api.NewBilibiliClient()
	info, err := client.GetStreamURL(roomID, qn)
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
	if len(roomIDs) == 0 {
		http.Error(w, "at least one room ID is required", http.StatusBadRequest)
		return
	}

	// 获取平台参数，默认为 douyu
	platform := strings.ToLower(r.URL.Query().Get("platform"))
	if platform == "" {
		platform = "douyu"
	}

	// 获取基础 URL 用于构建代理地址
	baseURL := r.URL.Query().Get("base_url")
	if baseURL == "" {
		scheme := "http"
		if r.TLS != nil {
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

		client := api.NewBilibiliClient()
		for _, roomID := range roomIDs {
			roomID = strings.TrimSpace(roomID)
			if roomID == "" {
				continue
			}

			info, err := client.GetStreamURL(roomID, qn)
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

		client := api.NewDouyuClient()
		for _, roomID := range roomIDs {
			roomID = strings.TrimSpace(roomID)
			if roomID == "" {
				continue
			}

			info, err := client.GetStreamURL(roomID, rate)
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
