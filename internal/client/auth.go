package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mezon-checkin-bot/mezon-protobuf/go/api"
	"mezon-checkin-bot/models"
	"net/http"
	"strconv"
	"time"
)

// ============================================================
// AUTHENTICATION
// ============================================================

func (c *MezonClient) Authenticate() error {
	log.Println("🔐 Authenticating bot...")

	authEndpoint := c.buildAuthEndpoint()
	authBody := c.buildAuthBody()

	req, err := c.createAuthRequest(authEndpoint, authBody)
	if err != nil {
		return err
	}

	resp, err := c.executeAuthRequest(req)
	if err != nil {
		return err
	}

	if err := c.processAuthResponse(resp); err != nil {
		return err
	}

	log.Println("✅ Bot authenticated successfully!")
	return nil
}

// ============================================================
// AUTH HELPERS
// ============================================================

func (c *MezonClient) buildAuthEndpoint() string {
	basePath := c.buildBasePath()
	return fmt.Sprintf("%s/v2/apps/authenticate/token", basePath)
}

func (c *MezonClient) buildBasePath() string {
	scheme := c.getScheme()
	host := c.config.Host
	port := c.config.Port

	if c.isDefaultPort() {
		return fmt.Sprintf("%s%s", scheme, host)
	}
	return fmt.Sprintf("%s%s:%s", scheme, host, port)
}

func (c *MezonClient) getScheme() string {
	if c.config.UseSSL {
		return "https://"
	}
	return "http://"
}

func (c *MezonClient) buildAuthBody() models.AuthRequest {
	authBody := models.AuthRequest{}
	authBody.Account.Appid = strconv.FormatInt(c.config.BotID, 10)
	authBody.Account.Token = c.config.BotToken
	return authBody
}

func (c *MezonClient) createAuthRequest(endpoint string, authBody models.AuthRequest) (*http.Request, error) {
	bodyJSON, err := json.Marshal(authBody)
	if err != nil {
		return nil, fmt.Errorf("marshal auth body failed: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create auth request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	basicAuth := base64.StdEncoding.EncodeToString([]byte(c.config.BotToken + ":"))
	req.Header.Set("Authorization", "Basic "+basicAuth)

	return req, nil
}

func (c *MezonClient) executeAuthRequest(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: DefaultTimeout * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("authentication request failed: %w", err)
	}
	return resp, nil
}

func (c *MezonClient) processAuthResponse(resp *http.Response) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ Response status: %d", resp.StatusCode)
		log.Printf("❌ Response body: %s", string(body))
		return fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp models.AuthResponse

	if err := json.Unmarshal(body, &authResp); err != nil {
		return fmt.Errorf("parse auth response failed: %w", err)
	}

	if authResp.Token == "" {
		return fmt.Errorf("no session token received")
	}

	c.handleAPIURLSwitch(authResp.SockerURL)
	c.createSession(authResp)

	return nil
}

func (c *MezonClient) handleAPIURLSwitch(socketURL string) {
	if socketURL == "" {
		return
	}

	newHost, newPort, newSSL, err := parseAPIURL(socketURL)
	if err == nil {
		log.Printf("   🔄 Switching to API server: %s:%s (SSL: %v)", newHost, newPort, newSSL)
		c.config.SocketHost = newHost
		c.config.SocketPort = newPort
		c.config.SocketUseSSL = newSSL
	}
}

func (c *MezonClient) createSession(authResp models.AuthResponse) {
	c.session = &api.Session{
		Token:        authResp.Token,
		RefreshToken: authResp.RefreshToken,
		Created:      authResp.Created,
	}
	c.ClientID = c.config.BotID
}

func parseAPIURL(socketURL string) (host string, port string, useSSL bool, err error) {
	useSSL = true
	host = socketURL
	port = "443"

	if host == "" {
		return "", "", false, fmt.Errorf("invalid api_url format")
	}
	return host, port, useSSL, nil
}
