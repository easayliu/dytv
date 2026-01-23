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
	http.HandleFunc("/health", handleHealth)

	addr := ":" + port
	fmt.Printf("Starting server on %s\n", addr)
	fmt.Println("Endpoints:")
	fmt.Println("  GET /live/{room_id}         - Redirect to stream URL")
	fmt.Println("  GET /live/{room_id}?rate=0  - Redirect with specific rate")
	fmt.Println("  GET /health                 - Health check")
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
		if r, ok := model.QualityMap[strings.ToLower(q)]; ok {
			rate = r
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

	http.Redirect(w, r, info.StreamURL, http.StatusFound)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
