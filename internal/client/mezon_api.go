package client

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	mzapi "mezon-checkin-bot/mezon-protobuf/go/api"

	"google.golang.org/protobuf/proto"
)

const apiSocketTimeout = 5 * time.Second

func (c *MezonClient) callAPI(apiName string, req proto.Message, resp proto.Message) error {
	reqBody, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Prefer HTTP to api.mezon.ai — reliable and avoids 30s socket timeout.
	if err := c.postProtobufHTTP("/mezon.api.Mezon/"+apiName, reqBody, resp); err == nil {
		return nil
	} else if c.verbose {
		log.Printf("   ⚠️  HTTP API %s failed, trying socket: %v", apiName, err)
	}

	if c.IsConnected() {
		respBody, err := c.sendApiRequest(apiName, reqBody, apiSocketTimeout)
		if err == nil {
			if len(respBody) == 0 {
				return nil
			}
			if err := proto.Unmarshal(respBody, resp); err != nil {
				return fmt.Errorf("unmarshal socket API response: %w", err)
			}
			return nil
		}
		return fmt.Errorf("API %s failed over HTTP and socket: %w", apiName, err)
	}

	return fmt.Errorf("API %s failed over HTTP and socket is disconnected", apiName)
}

func (c *MezonClient) postProtobufHTTP(path string, body []byte, resp proto.Message) error {
	if c.session == nil || c.session.Token == "" {
		return fmt.Errorf("no session available")
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.buildAPIBasePath()+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/proto")
	httpReq.Header.Set("Content-Type", "application/proto")
	httpReq.Header.Set("Authorization", "Bearer "+c.session.Token)

	client := &http.Client{Timeout: DefaultTimeout * time.Second}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	if len(respBody) == 0 {
		return fmt.Errorf("empty API response")
	}

	if err := proto.Unmarshal(respBody, resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	return nil
}

func (c *MezonClient) buildAPIBasePath() string {
	host := c.config.APIHost
	if host == "" {
		host = c.config.Host
	}
	port := c.config.APIPort
	if port == "" {
		port = c.config.Port
	}
	useSSL := c.config.APIUseSSL
	if host == c.config.Host && port == "" {
		port = c.config.Port
		useSSL = c.config.UseSSL
	}

	scheme := "http://"
	if useSSL {
		scheme = "https://"
	}

	if (useSSL && port == "443") || (!useSSL && port == "80") {
		return fmt.Sprintf("%s%s", scheme, host)
	}
	return fmt.Sprintf("%s%s:%s", scheme, host, port)
}

func (c *MezonClient) CreateDMChannel(userID int64) (*mzapi.ChannelDescription, error) {
	req := &mzapi.CreateChannelDescRequest{
		ClanId:         DMClanID,
		ChannelId:      0,
		CategoryId:     0,
		Type:           DMChannelType,
		ChannelPrivate: 1,
		UserIds:        []int64{userID},
	}

	var desc mzapi.ChannelDescription
	if err := c.callAPI("CreateChannelDesc", req, &desc); err != nil {
		return nil, fmt.Errorf("create DM channel: %w", err)
	}

	if desc.ChannelId == 0 {
		return nil, fmt.Errorf("create DM channel: missing channel_id in response")
	}

	return &desc, nil
}

func (c *MezonClient) ListDMChannels() ([]*mzapi.ChannelDescription, error) {
	req := &mzapi.ListChannelDescsRequest{
		ChannelType: DMChannelType,
	}

	var list mzapi.ChannelDescList
	if err := c.callAPI("ListChannelDescs", req, &list); err != nil {
		return nil, err
	}

	result := make([]*mzapi.ChannelDescription, 0, len(list.GetChanneldesc()))
	for _, ch := range list.GetChanneldesc() {
		if ch.GetType() == DMChannelType && ch.GetChannelId() != 0 {
			result = append(result, ch)
		}
	}
	return result, nil
}

func parseServiceURL(raw string) (host, port string, useSSL bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false, fmt.Errorf("empty service URL")
	}

	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false, err
	}

	host = u.Hostname()
	port = u.Port()
	useSSL = u.Scheme == "https"

	if port == "" {
		if useSSL {
			port = "443"
		} else {
			port = "80"
		}
	}

	if host == "" {
		return "", "", false, fmt.Errorf("invalid service URL host")
	}

	return host, port, useSSL, nil
}
