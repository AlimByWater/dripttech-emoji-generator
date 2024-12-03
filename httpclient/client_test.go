package httpclient

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
	"net/http"
	"testing"
	"time"
)

func TestDo(t *testing.T) {
	rl := rate.NewLimiter(rate.Every(1*time.Second), 30) // 50 request every 10 seconds
	c := NewClient(rl)
	reqURL := "https://api.telegram.org/bot7486051673:AAEg2bzMqec1NkFK8tHycLn8gvGxK6xQ6ww/getFile"

	body := []byte(`{
    "user_id": "-1001934236726",
    "title": "радио",
    "name": "test_by_demethra_test_polygon_bot",
    "sticker_type": "custom_emoji",
    "stickers": [
        {
            "sticker": "BQACAgIAAxUHZzXoyDT-gWfio2wrt0wtcXEH5moAAp9ZAAKHFLFJO1aXvhfhoLs2BA",
            "format": "video",
            "emoji_list": ["🎥"]

        }
    ],
    "file_id": "AgACAgIAAxkBAAEVS4RnNfjvejo2CN9oCOJQQZhvyVf-EwACJewxG5G_-UjRQ1FpAejWfAEAAwIAA3MAAzYE"
}
`)

	req, _ := http.NewRequest("POST", reqURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i < 300; i++ {
		resp, err := c.Do(req)
		assert.NoError(t, err)
		if resp.StatusCode == 429 {
			assert.Fail(t, "Rate limit reached after %d requests", i)
		}
	}
}
