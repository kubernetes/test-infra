package dingtalk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
)

// Logger provides an interface to log debug messages.
type Logger interface {
	Debugf(s string, v ...interface{})
}

// Client allows you to provide connection to Slack API Server
// It contains a token that allows to authenticate connection to post and work with channels in the domain
type Client struct {
	// If logger is non-nil, log all method calls with it.
	logger Logger
	fake   bool
}

type dingTalkMsg struct {
	MsgType  string   `json:"msgtype"`
	Markdown markdown `json:"markdown"`
}

type markdown struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

const (
	//https://oapi.dingtalk.com/robot/send?access_token=5c8422af69cd510952936d240e260842c5d2e20b10966ee76ac93cc10a2e233b
	chatPostMessage = "https://oapi.dingtalk.com/robot/send"
)

// NewClient creates a slack client with an API token.
func NewClient() *Client {
	return &Client{
		logger: logrus.WithField("client", "dingTalk"),
	}
}

// NewFakeClient returns a client that takes no actions.
func NewFakeClient() *Client {
	return &Client{
		fake: true,
	}
}

func (sl *Client) log(methodName string, args ...interface{}) {
	if sl.logger == nil {
		return
	}
	var as []string
	for _, arg := range args {
		as = append(as, fmt.Sprintf("%v", arg))
	}
	sl.logger.Debugf("%s(%s)", methodName, strings.Join(as, ", "))
}

func (sl *Client) postMessage(msg, token string) error {
	u, _ := url.Parse(chatPostMessage)
	var uv = url.Values{}
	uv.Add("access_token", token)
	u.RawQuery = uv.Encode()

	dtMsg := dingTalkMsg{
		MsgType: "markdown",
		Markdown: markdown{
			Title: "notify",
			Text:  msg,
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(dtMsg); err != nil {
		return err
	}

	resp, err := http.Post(u.String(), "application/json", &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	apiResponse := struct {
		Code  int    `json:"errcode"`
		Error string `json:"errmsg"`
	}{}

	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return fmt.Errorf("API returned invalid JSON (%q): %w", string(body), err)
	}

	if resp.StatusCode != 200 ||
		apiResponse.Code != 0 && apiResponse.Error != "ok" {
		return fmt.Errorf("request failed: %s", apiResponse.Error)
	}

	return nil
}

// WriteMessage adds text to channel
func (sl *Client) WriteMessage(msg, token string) error {
	sl.log("WriteMessage", msg, token)
	if sl.fake {
		return nil
	}

	if err := sl.postMessage(msg, token); err != nil {
		return fmt.Errorf("failed to post message to %s: %w", token, err)
	}
	return nil
}
