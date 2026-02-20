package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/easayliu/dytv/internal/cache"
	"github.com/easayliu/dytv/internal/model"
)

const (
	BilibiliRoomInitAPI = "https://api.live.bilibili.com/room/v1/Room/room_init?id=%s"
	BilibiliRoomInfoAPI = "https://api.live.bilibili.com/room/v1/Room/get_info?room_id=%s"
	BilibiliPlayInfoAPI = "https://api.live.bilibili.com/xlive/web-room/v2/index/getRoomPlayInfo"
)

// BilibiliClient handles Bilibili live stream API interactions
type BilibiliClient struct {
	httpClient  *HTTPClient
	verbose     bool
	roomIDCache *cache.Cache[string]          // displayRoomID → realRoomID
	streamCache *cache.Cache[*model.RoomInfo] // roomID:qn → RoomInfo
}

// NewBilibiliClient creates a new Bilibili API client
func NewBilibiliClient() *BilibiliClient {
	client := &HTTPClient{
		client: NewHTTPClient().client,
		headers: map[string]string{
			"User-Agent": UserAgent,
			"Accept":     "application/json, text/plain, */*",
			"Origin":     "https://live.bilibili.com",
			"Referer":    "https://live.bilibili.com/",
		},
	}

	return &BilibiliClient{
		httpClient:  client,
		verbose:     false,
		roomIDCache: cache.New[string](24 * time.Hour),
		streamCache: cache.New[*model.RoomInfo](5 * time.Minute),
	}
}

// StartCacheCleanup starts background goroutines to purge expired cache entries.
func (c *BilibiliClient) StartCacheCleanup(stop <-chan struct{}) {
	c.roomIDCache.StartCleanup(1*time.Hour, stop)
	c.streamCache.StartCleanup(1*time.Minute, stop)
}

// SetVerbose enables or disables verbose logging
func (c *BilibiliClient) SetVerbose(verbose bool) {
	c.verbose = verbose
}

// bilibiliRoomInitResponse represents the room_init API response
type bilibiliRoomInitResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		RoomID     int  `json:"room_id"`
		ShortID    int  `json:"short_id"`
		UID        int  `json:"uid"`
		LiveStatus int  `json:"live_status"`
		IsHidden   bool `json:"is_hidden"`
		IsLocked   bool `json:"is_locked"`
		Encrypted  bool `json:"encrypted"`
	} `json:"data"`
}

// bilibiliPlayInfoResponse represents the getRoomPlayInfo API response
type bilibiliPlayInfoResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		RoomID      int                  `json:"room_id"`
		ShortID     int                  `json:"short_id"`
		UID         int                  `json:"uid"`
		LiveStatus  int                  `json:"live_status"`
		PlayURLInfo *bilibiliPlayURLInfo `json:"playurl_info"`
	} `json:"data"`
}

type bilibiliPlayURLInfo struct {
	ConfJSON string           `json:"conf_json"`
	Playurl  *bilibiliPlayURL `json:"playurl"`
}

type bilibiliPlayURL struct {
	Cid     int              `json:"cid"`
	GQnDesc []bilibiliQnDesc `json:"g_qn_desc"`
	Stream  []bilibiliStream `json:"stream"`
}

type bilibiliQnDesc struct {
	Qn   int    `json:"qn"`
	Desc string `json:"desc"`
}

type bilibiliStream struct {
	ProtocolName string           `json:"protocol_name"`
	Format       []bilibiliFormat `json:"format"`
}

type bilibiliFormat struct {
	FormatName string          `json:"format_name"`
	Codec      []bilibiliCodec `json:"codec"`
}

type bilibiliCodec struct {
	CodecName string            `json:"codec_name"`
	CurrentQn int               `json:"current_qn"`
	AcceptQn  []int             `json:"accept_qn"`
	BaseURL   string            `json:"base_url"`
	URLInfo   []bilibiliURLInfo `json:"url_info"`
}

type bilibiliURLInfo struct {
	Host      string `json:"host"`
	Extra     string `json:"extra"`
	StreamTTL int    `json:"stream_ttl"`
}

// GetRealRoomID resolves a short room ID to the real room ID
func (c *BilibiliClient) GetRealRoomID(roomID string) (string, error) {
	apiURL := fmt.Sprintf(BilibiliRoomInitAPI, url.QueryEscape(roomID))
	body, err := c.httpClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch room init: %w", err)
	}

	if c.verbose {
		fmt.Printf("Room init response: %s\n", string(body))
	}

	var resp bilibiliRoomInitResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse room init response: %w", err)
	}

	if resp.Code != 0 {
		return "", fmt.Errorf("room init API error %d: %s", resp.Code, resp.Message)
	}

	return strconv.Itoa(resp.Data.RoomID), nil
}

// GetStreamURL fetches the stream URL for a Bilibili live room
func (c *BilibiliClient) GetStreamURL(roomID string, qn int) (*model.RoomInfo, error) {
	// Default to highest quality if not specified
	if qn == 0 {
		qn = 10000 // 原画
	}

	streamKey := fmt.Sprintf("%s:%d", roomID, qn)
	return c.streamCache.GetOrLoad(streamKey, func() (*model.RoomInfo, error) {
		return c.fetchStreamURL(roomID, qn)
	})
}

// fetchStreamURL does the actual work of obtaining a stream URL (no caching).
func (c *BilibiliClient) fetchStreamURL(roomID string, qn int) (*model.RoomInfo, error) {
	// Room ID cache
	realRoomID, err := c.roomIDCache.GetOrLoad(roomID, func() (string, error) {
		return c.GetRealRoomID(roomID)
	})
	if err != nil {
		if c.verbose {
			fmt.Printf("Warning: failed to resolve real room ID: %v\n", err)
		}
		realRoomID = roomID
	}

	if c.verbose {
		fmt.Printf("Real room ID: %s\n", realRoomID)
	}

	// Build API URL with parameters
	params := url.Values{}
	params.Set("room_id", realRoomID)
	params.Set("protocol", "0,1") // 0=http_stream, 1=http_hls
	params.Set("format", "0,1,2") // 0=flv, 1=ts, 2=fmp4
	params.Set("codec", "0,1")    // 0=avc, 1=hevc
	params.Set("qn", strconv.Itoa(qn))
	params.Set("platform", "h5")
	params.Set("ptype", "8")

	apiURL := BilibiliPlayInfoAPI + "?" + params.Encode()

	body, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch play info: %w", err)
	}

	if c.verbose {
		fmt.Printf("Play info response: %s\n", string(body))
	}

	var resp bilibiliPlayInfoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse play info response: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("play info API error %d: %s", resp.Code, resp.Message)
	}

	// Check if room is live — return error so offline results are not cached
	if resp.Data.LiveStatus != 1 {
		return nil, ErrRoomOffline
	}

	// Extract stream URL
	streamURL, multirates, err := c.extractStreamURL(resp.Data.PlayURLInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to extract stream URL: %w", err)
	}

	roomInfo := &model.RoomInfo{
		RoomID:     realRoomID,
		StreamURL:  streamURL,
		FlvURL:     streamURL,
		Multirates: multirates,
		IsLive:     streamURL != "",
	}

	return roomInfo, nil
}

// extractStreamURL extracts the best stream URL from play info
func (c *BilibiliClient) extractStreamURL(playURLInfo *bilibiliPlayURLInfo) (string, []model.StreamRate, error) {
	if playURLInfo == nil || playURLInfo.Playurl == nil {
		return "", nil, fmt.Errorf("no play URL info available")
	}

	playURL := playURLInfo.Playurl

	// Build multirates from quality descriptions
	var multirates []model.StreamRate
	for _, qnDesc := range playURL.GQnDesc {
		multirates = append(multirates, model.StreamRate{
			Name: qnDesc.Desc,
			Rate: qnDesc.Qn,
		})
	}

	// Find the best stream URL (prefer FLV format)
	for _, stream := range playURL.Stream {
		// Prefer http_stream protocol
		if stream.ProtocolName != "http_stream" {
			continue
		}

		for _, format := range stream.Format {
			// Prefer FLV format
			if format.FormatName != "flv" {
				continue
			}

			for _, codec := range format.Codec {
				// Prefer AVC codec for better compatibility
				if codec.CodecName != "avc" {
					continue
				}

				if len(codec.URLInfo) > 0 && codec.BaseURL != "" {
					urlInfo := codec.URLInfo[0]
					streamURL := urlInfo.Host + codec.BaseURL + urlInfo.Extra
					return streamURL, multirates, nil
				}
			}
		}
	}

	// Fallback: try any available stream
	for _, stream := range playURL.Stream {
		for _, format := range stream.Format {
			for _, codec := range format.Codec {
				if len(codec.URLInfo) > 0 && codec.BaseURL != "" {
					urlInfo := codec.URLInfo[0]
					streamURL := urlInfo.Host + codec.BaseURL + urlInfo.Extra
					return streamURL, multirates, nil
				}
			}
		}
	}

	return "", multirates, fmt.Errorf("no stream URL found")
}

// GetRoomStatus checks if a room is currently live
func (c *BilibiliClient) GetRoomStatus(roomID string) (bool, error) {
	realRoomID, err := c.GetRealRoomID(roomID)
	if err != nil {
		return false, err
	}

	apiURL := fmt.Sprintf(BilibiliRoomInfoAPI, url.QueryEscape(realRoomID))
	body, err := c.httpClient.Get(apiURL)
	if err != nil {
		return false, fmt.Errorf("failed to fetch room info: %w", err)
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			LiveStatus int `json:"live_status"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return false, fmt.Errorf("failed to parse room info: %w", err)
	}

	return resp.Data.LiveStatus == 1, nil
}

// ListQualities returns available stream qualities for a room
func (c *BilibiliClient) ListQualities(roomID string) ([]model.StreamRate, error) {
	info, err := c.GetStreamURL(roomID, 0)
	if err != nil {
		return nil, err
	}
	return info.Multirates, nil
}

// ParseBilibiliRoomID extracts room ID from URL or returns input as-is
func ParseBilibiliRoomID(input string) string {
	if strings.Contains(input, "bilibili.com") || strings.Contains(input, "live.bilibili.com") {
		// Match patterns like live.bilibili.com/12345
		pattern := regexp.MustCompile(`live\.bilibili\.com/(\d+)`)
		matches := pattern.FindStringSubmatch(input)
		if len(matches) >= 2 {
			return matches[1]
		}
	}

	// If input is a number, return as-is
	if _, err := strconv.Atoi(input); err == nil {
		return input
	}

	return input
}
