package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/easayliu/dytv/internal/crypto"
	"github.com/easayliu/dytv/internal/model"
)

var reDigitsOnly = regexp.MustCompile(`^\d+$`)

const (
	DouyuRoomURL      = "https://www.douyu.com/%s"
	DouyuAPIURL       = "https://www.douyu.com/lapi/live/getH5Play/%s"
	DouyuBetardAPI    = "https://www.douyu.com/betard/%s"
	DouyuSwfAPI       = "https://www.douyu.com/swf_api/homeH5Enc?rids=%s"
	DouyuOpenAPI      = "https://open.douyucdn.cn/api/RoomApi/room/%s"
	DouyuSearchRecAPI  = "https://www.douyu.com/wgapi/livenc/search/searchWordRec"
	DouyuFollowListAPI = "https://www.douyu.com/wgapi/livenc/liveweb/follow/list"
)

type DouyuClient struct {
	httpClient *HTTPClient
	verbose    bool
}

func NewDouyuClient() *DouyuClient {
	return &DouyuClient{
		httpClient: NewHTTPClient(),
		verbose:    false,
	}
}

func (c *DouyuClient) SetVerbose(verbose bool) {
	c.verbose = verbose
}

func (c *DouyuClient) GetRealRoomID(roomID string) (string, error) {
	roomURL := fmt.Sprintf(DouyuRoomURL, url.PathEscape(roomID))
	body, err := c.httpClient.Get(roomURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch room page: %w", err)
	}

	html := string(body)

	patterns := []string{
		`\$ROOM\.room_id\s*=\s*(\d+)`,
		`"room_id"\s*:\s*(\d+)`,
		`rid['"]*\s*[:=]\s*['"]?(\d+)`,
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p)
		matches := re.FindStringSubmatch(html)
		if len(matches) >= 2 {
			return matches[1], nil
		}
	}

	return roomID, nil
}

func (c *DouyuClient) GetStreamURL(roomID string, rate int) (*model.RoomInfo, error) {
	realRoomID, err := c.GetRealRoomID(roomID)
	if err != nil {
		if c.verbose {
			fmt.Printf("Warning: failed to resolve real room ID: %v\n", err)
		}
		realRoomID = roomID
	}

	if c.verbose {
		fmt.Printf("Real room ID: %s\n", realRoomID)
	}

	// Get encrypted JS from swf_api
	jsCode, err := c.getEncryptedJS(realRoomID)
	if err != nil {
		if c.verbose {
			fmt.Printf("Warning: failed to get encrypted JS: %v\n", err)
		}
		return nil, fmt.Errorf("failed to get signing function: %w", err)
	}

	// Execute JS to get sign params
	signParams, err := c.executeSigningJS(realRoomID, jsCode)
	if err != nil {
		if c.verbose {
			fmt.Printf("Warning: JS execution failed: %v\n", err)
		}
		return nil, fmt.Errorf("failed to execute signing: %w", err)
	}

	// Build request data
	reqData := fmt.Sprintf("%s&cdn=&rate=%d&ver=Douyu_223061005&iar=0&ive=0&hevc=0&fa=0", signParams, rate)

	if c.verbose {
		fmt.Printf("Request data: %s\n", reqData)
	}

	// Call the API
	apiURL := fmt.Sprintf(DouyuAPIURL, url.PathEscape(realRoomID))
	respBody, err := c.httpClient.Post(apiURL, reqData)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("API response: %s\n", string(respBody))
	}

	var apiResp model.APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if apiResp.Error != 0 {
		return nil, fmt.Errorf("API error %d: %s", apiResp.Error, apiResp.Msg)
	}

	// Parse the data field which can be either an object or empty string
	var streamData model.StreamData
	if dataMap, ok := apiResp.RawData.(map[string]interface{}); ok {
		dataBytes, _ := json.Marshal(dataMap)
		if err := json.Unmarshal(dataBytes, &streamData); err != nil {
			return nil, fmt.Errorf("failed to parse stream data: %w", err)
		}
	} else {
		// Room is offline or no stream available
		return &model.RoomInfo{
			RoomID: realRoomID,
			IsLive: false,
		}, nil
	}

	streamURL := ""
	flvURL := ""

	if streamData.RtmpURL != "" && streamData.RtmpLive != "" {
		streamURL = fmt.Sprintf("%s/%s", streamData.RtmpURL, streamData.RtmpLive)
		flvURL = streamURL
	}

	roomInfo := &model.RoomInfo{
		RoomID:     realRoomID,
		StreamURL:  streamURL,
		FlvURL:     flvURL,
		Multirates: streamData.Multirates,
		IsLive:     streamURL != "",
	}

	return roomInfo, nil
}

func (c *DouyuClient) getEncryptedJS(roomID string) (string, error) {
	swfURL := fmt.Sprintf(DouyuSwfAPI, url.QueryEscape(roomID))
	body, err := c.httpClient.Get(swfURL)
	if err != nil {
		return "", fmt.Errorf("swf API request failed: %w", err)
	}

	var swfResp struct {
		Error int               `json:"error"`
		Data  map[string]string `json:"data"`
	}

	if err := json.Unmarshal(body, &swfResp); err != nil {
		return "", fmt.Errorf("failed to parse swf API response: %w", err)
	}

	if swfResp.Error != 0 {
		return "", fmt.Errorf("swf API error: %d", swfResp.Error)
	}

	jsCode, ok := swfResp.Data["room"+roomID]
	if !ok {
		return "", fmt.Errorf("no JS code found for room %s", roomID)
	}

	return jsCode, nil
}

func (c *DouyuClient) executeSigningJS(roomID string, jsCode string) (string, error) {
	// 校验 roomID 只包含数字，防止 JS 注入
	if !reDigitsOnly.MatchString(roomID) {
		return "", fmt.Errorf("invalid room ID: %s", roomID)
	}

	vm := goja.New()

	// 设置执行超时，防止恶意 JS 导致 goroutine 阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			vm.Interrupt("execution timeout")
		}
	}()

	// Simple standalone MD5 implementation for CryptoJS compatibility
	cryptoJSCode := `
var CryptoJS = {
    MD5: function(string) {
        function md5cycle(x, k) {
            var a = x[0], b = x[1], c = x[2], d = x[3];
            a = ff(a, b, c, d, k[0], 7, -680876936);
            d = ff(d, a, b, c, k[1], 12, -389564586);
            c = ff(c, d, a, b, k[2], 17,  606105819);
            b = ff(b, c, d, a, k[3], 22, -1044525330);
            a = ff(a, b, c, d, k[4], 7, -176418897);
            d = ff(d, a, b, c, k[5], 12,  1200080426);
            c = ff(c, d, a, b, k[6], 17, -1473231341);
            b = ff(b, c, d, a, k[7], 22, -45705983);
            a = ff(a, b, c, d, k[8], 7,  1770035416);
            d = ff(d, a, b, c, k[9], 12, -1958414417);
            c = ff(c, d, a, b, k[10], 17, -42063);
            b = ff(b, c, d, a, k[11], 22, -1990404162);
            a = ff(a, b, c, d, k[12], 7,  1804603682);
            d = ff(d, a, b, c, k[13], 12, -40341101);
            c = ff(c, d, a, b, k[14], 17, -1502002290);
            b = ff(b, c, d, a, k[15], 22,  1236535329);
            a = gg(a, b, c, d, k[1], 5, -165796510);
            d = gg(d, a, b, c, k[6], 9, -1069501632);
            c = gg(c, d, a, b, k[11], 14,  643717713);
            b = gg(b, c, d, a, k[0], 20, -373897302);
            a = gg(a, b, c, d, k[5], 5, -701558691);
            d = gg(d, a, b, c, k[10], 9,  38016083);
            c = gg(c, d, a, b, k[15], 14, -660478335);
            b = gg(b, c, d, a, k[4], 20, -405537848);
            a = gg(a, b, c, d, k[9], 5,  568446438);
            d = gg(d, a, b, c, k[14], 9, -1019803690);
            c = gg(c, d, a, b, k[3], 14, -187363961);
            b = gg(b, c, d, a, k[8], 20,  1163531501);
            a = gg(a, b, c, d, k[13], 5, -1444681467);
            d = gg(d, a, b, c, k[2], 9, -51403784);
            c = gg(c, d, a, b, k[7], 14,  1735328473);
            b = gg(b, c, d, a, k[12], 20, -1926607734);
            a = hh(a, b, c, d, k[5], 4, -378558);
            d = hh(d, a, b, c, k[8], 11, -2022574463);
            c = hh(c, d, a, b, k[11], 16,  1839030562);
            b = hh(b, c, d, a, k[14], 23, -35309556);
            a = hh(a, b, c, d, k[1], 4, -1530992060);
            d = hh(d, a, b, c, k[4], 11,  1272893353);
            c = hh(c, d, a, b, k[7], 16, -155497632);
            b = hh(b, c, d, a, k[10], 23, -1094730640);
            a = hh(a, b, c, d, k[13], 4,  681279174);
            d = hh(d, a, b, c, k[0], 11, -358537222);
            c = hh(c, d, a, b, k[3], 16, -722521979);
            b = hh(b, c, d, a, k[6], 23,  76029189);
            a = hh(a, b, c, d, k[9], 4, -640364487);
            d = hh(d, a, b, c, k[12], 11, -421815835);
            c = hh(c, d, a, b, k[15], 16,  530742520);
            b = hh(b, c, d, a, k[2], 23, -995338651);
            a = ii(a, b, c, d, k[0], 6, -198630844);
            d = ii(d, a, b, c, k[7], 10,  1126891415);
            c = ii(c, d, a, b, k[14], 15, -1416354905);
            b = ii(b, c, d, a, k[5], 21, -57434055);
            a = ii(a, b, c, d, k[12], 6,  1700485571);
            d = ii(d, a, b, c, k[3], 10, -1894986606);
            c = ii(c, d, a, b, k[10], 15, -1051523);
            b = ii(b, c, d, a, k[1], 21, -2054922799);
            a = ii(a, b, c, d, k[8], 6,  1873313359);
            d = ii(d, a, b, c, k[15], 10, -30611744);
            c = ii(c, d, a, b, k[6], 15, -1560198380);
            b = ii(b, c, d, a, k[13], 21,  1309151649);
            a = ii(a, b, c, d, k[4], 6, -145523070);
            d = ii(d, a, b, c, k[11], 10, -1120210379);
            c = ii(c, d, a, b, k[2], 15,  718787259);
            b = ii(b, c, d, a, k[9], 21, -343485551);
            x[0] = add32(a, x[0]);
            x[1] = add32(b, x[1]);
            x[2] = add32(c, x[2]);
            x[3] = add32(d, x[3]);
        }
        function cmn(q, a, b, x, s, t) {
            a = add32(add32(a, q), add32(x, t));
            return add32((a << s) | (a >>> (32 - s)), b);
        }
        function ff(a, b, c, d, x, s, t) { return cmn((b & c) | ((~b) & d), a, b, x, s, t); }
        function gg(a, b, c, d, x, s, t) { return cmn((b & d) | (c & (~d)), a, b, x, s, t); }
        function hh(a, b, c, d, x, s, t) { return cmn(b ^ c ^ d, a, b, x, s, t); }
        function ii(a, b, c, d, x, s, t) { return cmn(c ^ (b | (~d)), a, b, x, s, t); }
        function md5blk(s) {
            var md5blks = [], i;
            for (i = 0; i < 64; i += 4) {
                md5blks[i >> 2] = s.charCodeAt(i)
                    + (s.charCodeAt(i + 1) << 8)
                    + (s.charCodeAt(i + 2) << 16)
                    + (s.charCodeAt(i + 3) << 24);
            }
            return md5blks;
        }
        var hex_chr = '0123456789abcdef'.split('');
        function rhex(n) {
            var s = '', j = 0;
            for (; j < 4; j++)
                s += hex_chr[(n >> (j * 8 + 4)) & 0x0F] + hex_chr[(n >> (j * 8)) & 0x0F];
            return s;
        }
        function hex(x) {
            for (var i = 0; i < x.length; i++)
                x[i] = rhex(x[i]);
            return x.join('');
        }
        function md5(s) {
            return hex(md51(s));
        }
        function add32(a, b) {
            return (a + b) & 0xFFFFFFFF;
        }
        function md51(s) {
            var n = s.length,
                state = [1732584193, -271733879, -1732584194, 271733878], i;
            for (i = 64; i <= s.length; i += 64) {
                md5cycle(state, md5blk(s.substring(i - 64, i)));
            }
            s = s.substring(i - 64);
            var tail = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0];
            for (i = 0; i < s.length; i++)
                tail[i >> 2] |= s.charCodeAt(i) << ((i % 4) << 3);
            tail[i >> 2] |= 0x80 << ((i % 4) << 3);
            if (i > 55) {
                md5cycle(state, tail);
                for (i = 0; i < 16; i++) tail[i] = 0;
            }
            tail[14] = n * 8;
            md5cycle(state, tail);
            return state;
        }
        return md5(String(string));
    }
};
`

	if _, err := vm.RunString(cryptoJSCode); err != nil {
		return "", fmt.Errorf("failed to setup CryptoJS: %w", err)
	}

	// Run the encrypted JS code to define the signing function
	if _, err := vm.RunString(jsCode); err != nil {
		return "", fmt.Errorf("failed to execute JS code: %w", err)
	}

	did := crypto.DefaultDID
	tt := time.Now().Unix()

	// Call ub98484234 function
	callCode := fmt.Sprintf("ub98484234(%s, '%s', %d)", roomID, did, tt)
	result, err := vm.RunString(callCode)
	if err != nil {
		return "", fmt.Errorf("failed to execute signing function: %w", err)
	}

	return result.String(), nil
}

func (c *DouyuClient) GetRoomStatus(roomID string) (bool, error) {
	openURL := fmt.Sprintf(DouyuOpenAPI, url.PathEscape(roomID))
	body, err := c.httpClient.Get(openURL)
	if err != nil {
		return false, fmt.Errorf("failed to check room status: %w", err)
	}

	var result struct {
		Error int `json:"error"`
		Data  struct {
			RoomStatus string `json:"room_status"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	return result.Data.RoomStatus == "1", nil
}

func (c *DouyuClient) ListQualities(roomID string) ([]model.StreamRate, error) {
	info, err := c.GetStreamURL(roomID, 0)
	if err != nil {
		return nil, err
	}
	return info.Multirates, nil
}

// DouyuSearchRecItem represents a recommended room from search API.
type DouyuSearchRecItem struct {
	Keyword    string `json:"kw"`
	BizID      int    `json:"bizId"`
	ShowStatus int    `json:"showStatus"`
}

// SearchRecommend fetches recommended live rooms from Douyu search API.
func (c *DouyuClient) SearchRecommend(cookie string) ([]DouyuSearchRecItem, error) {
	headers := map[string]string{
		"Cookie": cookie,
	}

	body, err := c.httpClient.GetWithHeaders(DouyuSearchRecAPI, headers)
	if err != nil {
		return nil, fmt.Errorf("search recommend request failed: %w", err)
	}

	var resp struct {
		Error int `json:"error"`
		Data  struct {
			List []DouyuSearchRecItem `json:"list"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse search recommend response: %w", err)
	}

	if resp.Error != 0 {
		return nil, fmt.Errorf("search recommend API error: %d", resp.Error)
	}

	return resp.Data.List, nil
}

// DouyuFollowRoom represents a room in the follow list.
type DouyuFollowRoom struct {
	RoomID      int    `json:"room_id"`
	RoomName    string `json:"room_name"`
	Nickname    string `json:"nickname"`
	GameName    string `json:"game_name"`
	ShowStatus  int    `json:"show_status"` // 1=在线, 2=离线
	Online      string `json:"online"`
	RoomSrc     string `json:"room_src"`
	AvatarSmall string `json:"avatar_small"`
}

// FollowList fetches the user's followed rooms from Douyu.
func (c *DouyuClient) FollowList(cookie string) ([]DouyuFollowRoom, error) {
	headers := map[string]string{
		"Cookie": cookie,
	}

	body, err := c.httpClient.GetWithHeaders(DouyuFollowListAPI, headers)
	if err != nil {
		return nil, fmt.Errorf("follow list request failed: %w", err)
	}

	var resp struct {
		Error int    `json:"error"`
		Msg   string `json:"msg"`
		Data  struct {
			List []DouyuFollowRoom `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse follow list response: %w", err)
	}
	if resp.Error != 0 {
		return nil, fmt.Errorf("follow list API error(%d): %s", resp.Error, resp.Msg)
	}

	return resp.Data.List, nil
}

func ParseRoomID(input string) string {
	if strings.Contains(input, "douyu.com") {
		pattern := regexp.MustCompile(`douyu\.com/(\d+)`)
		matches := pattern.FindStringSubmatch(input)
		if len(matches) >= 2 {
			return matches[1]
		}

		pattern2 := regexp.MustCompile(`rid=(\d+)`)
		matches = pattern2.FindStringSubmatch(input)
		if len(matches) >= 2 {
			return matches[1]
		}
	}

	if _, err := strconv.Atoi(input); err == nil {
		return input
	}

	return input
}
