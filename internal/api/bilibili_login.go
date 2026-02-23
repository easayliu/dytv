package api

import (
	"encoding/json"
	"fmt"
)

const (
	bilibiliFollowListAPI = "https://api.live.bilibili.com/xlive/web-ucenter/user/following?page=%d&page_size=29"
)

// BilibiliFollowRoom represents a room in the Bilibili follow list.
type BilibiliFollowRoom struct {
	RoomID     int    `json:"roomid"`
	UID        int    `json:"uid"`
	Uname      string `json:"uname"`
	Title      string `json:"title"`
	Face       string `json:"face"`
	LiveStatus int    `json:"live_status"` // 1=直播中, 0=未开播
}

// BilibiliFollowList fetches the user's followed live rooms from Bilibili.
// Automatically paginates to fetch all followed rooms.
func (c *BilibiliClient) BilibiliFollowList(cookie string) ([]BilibiliFollowRoom, error) {
	headers := map[string]string{
		"Cookie": cookie,
	}

	var allRooms []BilibiliFollowRoom
	for page := 1; ; page++ {
		apiURL := fmt.Sprintf(bilibiliFollowListAPI, page)
		body, err := c.httpClient.GetWithHeaders(apiURL, headers)
		if err != nil {
			return nil, fmt.Errorf("follow list request failed: %w", err)
		}

		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				List      []BilibiliFollowRoom `json:"list"`
				TotalPage int                  `json:"totalPage"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parse follow list response: %w", err)
		}
		if resp.Code != 0 {
			return nil, fmt.Errorf("follow list API error(%d): %s", resp.Code, resp.Message)
		}

		allRooms = append(allRooms, resp.Data.List...)

		if page >= resp.Data.TotalPage {
			break
		}
	}

	return allRooms, nil
}
