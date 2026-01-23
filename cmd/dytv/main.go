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

	http.HandleFunc("/live/", handleLive)
	http.HandleFunc("/playlist.m3u", handlePlaylist)
	http.HandleFunc("/health", handleHealth)

	addr := ":" + port
	fmt.Printf("Starting server on %s\n", addr)
	fmt.Println("Endpoints:")
	fmt.Println("  GET /live/{room_id}              - Redirect to FLV stream")
	fmt.Println("  GET /live/{room_id}?format=json  - Return stream info as JSON")
	fmt.Println("  GET /live/{room_id}?rate=0       - Specify quality (0=原画, 2=超清, 3=流畅)")
	fmt.Println("  GET /playlist.m3u?rooms=1,2,3    - Generate M3U playlist for multiple rooms")
	fmt.Println("  GET /health                      - Health check")
	fmt.Println()

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func handleLive(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/live/")
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

	// 根据 format 参数选择返回格式
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"room_id":    info.RoomID,
			"is_live":    info.IsLive,
			"flv_url":    info.FlvURL,
			"stream_url": info.StreamURL,
			"multirates": info.Multirates,
		})
		return
	}

	// 默认返回 FLV 流
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

	// 构建 M3U 播放列表
	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n")

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

		// 添加频道信息
		channelName := fmt.Sprintf("斗鱼-%s", info.RoomID)
		if info.RoomName != "" {
			channelName = info.RoomName
		}

		playlist.WriteString(fmt.Sprintf("#EXTINF:-1,%s\n", channelName))
		playlist.WriteString(info.FlvURL + "\n")
	}

	w.Header().Set("Content-Type", "audio/x-mpegurl")
	w.Header().Set("Content-Disposition", "attachment; filename=\"douyu.m3u\"")
	_, _ = w.Write([]byte(playlist.String()))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
