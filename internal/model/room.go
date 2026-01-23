package model

// RoomInfo represents the room information from Douyu API
type RoomInfo struct {
	RoomID     string       `json:"room_id"`
	RoomName   string       `json:"room_name"`
	OwnerName  string       `json:"owner_name"`
	IsLive     bool         `json:"is_live"`
	StreamURL  string       `json:"stream_url"`
	FlvURL     string       `json:"flv_url"`
	Multirates []StreamRate `json:"multirates"`
}

// StreamRate represents a stream quality option
type StreamRate struct {
	Name    string `json:"name"`
	Rate    int    `json:"rate"`
	HighBit int    `json:"highBit"`
	Bit     int    `json:"bit"`
}

// APIResponse represents the Douyu API response structure
type APIResponse struct {
	Error   int         `json:"error"`
	Msg     string      `json:"msg"`
	Data    *StreamData `json:"-"` // Custom unmarshaling
	RawData interface{} `json:"data"`
}

// StreamData contains the stream information from API
type StreamData struct {
	RoomID       int          `json:"room_id"`
	IsMixed      bool         `json:"is_mixed"`
	MixedLive    string       `json:"mixed_live"`
	MixedURL     string       `json:"mixed_url"`
	RtmpURL      string       `json:"rtmp_url"`
	RtmpLive     string       `json:"rtmp_live"`
	HlsURL       string       `json:"hls_url"`
	FlvURL       string       `json:"flv_url"`
	ClientIP     string       `json:"client_ip"`
	InNA         int          `json:"inNA"`
	RateSwitch   int          `json:"rateSwitch"`
	Multirates   []StreamRate `json:"multirates"`
	Cdns         []string     `json:"cdns"`
	CdnsWithName []CDNInfo    `json:"cdnsWithName"`
	P2P          int          `json:"p2p"`
	P2PPath      string       `json:"p2ppath"`
}

// CDNInfo represents CDN node information
type CDNInfo struct {
	Name   string `json:"name"`
	CDN    string `json:"cdn"`
	IsH265 bool   `json:"isH265"`
}

// QualityMap maps quality names to rate values
var QualityMap = map[string]int{
	"high": 0, // 原画/高清
	"mid":  2, // 超清
	"low":  3, // 流畅
}
