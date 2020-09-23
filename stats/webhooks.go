// And here we scope creep a bit.. all this code is part of the stats
// modules, but really it's about monitoring the incoming RCON feed, or
// sending internal alerts, or whatever.

package stats

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/d5/tengo"
)

// DiscordWebhookData contains a simple webhook message, no embed support (yet, maybe I dunno)
type DiscordWebhookData struct {
	Username string `json:"username,omitempty"`
	Content  string `json:"content"`
}

// SlackWebhookData contains the webhook message for slack
type SlackWebhookData struct {
	Text string `json:"text"`
}

// CanCall returns true since we're a function type.
func (o *TengoSlackWebhook) CanCall() bool {
	return true
}

// TypeName returns the function name
func (o *TengoSlackWebhook) TypeName() string {
	return "slack_webhook"
}

// String returns the function name
func (o *TengoSlackWebhook) String() string {
	return "slack_webhook"
}

// Call provides the logger functionality.
// slack_webhook(hookurl, text)
func (o *TengoSlackWebhook) Call(args ...tengo.Object) (ret tengo.Object, err error) {
	if len(args) != 2 {
		return nil, tengo.ErrWrongNumArguments
	}

	dmsg := SlackWebhookData{}

	url, ok := tengo.ToString(args[0])
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "first",
			Expected: "string",
			Found:    args[0].TypeName(),
		}
	}

	s2, ok := tengo.ToString(args[1])
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "second",
			Expected: "string",
			Found:    args[0].TypeName(),
		}
	}
	dmsg.Text = s2

	dbody, _ := json.Marshal(dmsg)
	resp, err := sendWebhookData(url, dbody)

	if err != nil {
		return &tengo.Int{Value: -1}, nil
	}

	return &tengo.Int{Value: int64(resp.StatusCode)}, nil
}

// CanCall returns true since we're a function type.
func (o *TengoDiscordWebhook) CanCall() bool {
	return true
}

// TypeName returns the function name
func (o *TengoDiscordWebhook) TypeName() string {
	return "discord_webhook"
}

// String returns the function name
func (o *TengoDiscordWebhook) String() string {
	return "discord_webhook"
}

// Call provides the logger functionality.
// discord_webhook(hookurl, content, [optional]username)
func (o *TengoDiscordWebhook) Call(args ...tengo.Object) (ret tengo.Object, err error) {
	if len(args) > 3 || len(args) < 2 {
		return nil, tengo.ErrWrongNumArguments
	}

	dmsg := DiscordWebhookData{}
	if len(args) > 2 {
		s3, ok := tengo.ToString(args[2])
		if !ok {
			return nil, tengo.ErrInvalidArgumentType{
				Name:     "third",
				Expected: "string",
				Found:    args[2].TypeName(),
			}
		}
		dmsg.Username = s3
	}

	url, ok := tengo.ToString(args[0])
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "first",
			Expected: "string",
			Found:    args[0].TypeName(),
		}
	}

	s2, ok := tengo.ToString(args[1])
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "second",
			Expected: "string",
			Found:    args[0].TypeName(),
		}
	}
	dmsg.Content = s2

	dbody, _ := json.Marshal(dmsg)
	resp, err := sendWebhookData(url, dbody)

	if err != nil {
		return &tengo.Int{Value: -1}, nil
	}

	return &tengo.Int{Value: int64(resp.StatusCode)}, nil
}

func sendWebhookData(url string, data []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}
