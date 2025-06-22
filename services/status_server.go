package services

import (
	"bytes"
	"chirpstack-httpserver/config"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// StatusServerClient 封装了与状态服务器的交互
type StatusServerClient struct {
	client  *http.Client
	baseURL string
}

// NewStatusServerClient 创建一个新的状态服务器客户端
func NewStatusServerClient(cfg config.Config) *StatusServerClient {
	return &StatusServerClient{
		client: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
		baseURL: cfg.StatusServerURL,
	}
}

// SendWarnInfo 发送报警信息
func (c *StatusServerClient) SendWarnInfo(stakeNo string, warnType int) error {
	url := fmt.Sprintf("%s/warn/warnInfo", c.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	q := req.URL.Query()
	q.Add("stakeNo", stakeNo)
	q.Add("eventDate", time.Now().Format("2006-01-02 15:04:05"))
	q.Add("warnType", fmt.Sprintf("%d", warnType))
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回非 200 状态码: %d", resp.StatusCode)
	}
	return nil
}

// SendHeartbeat 发送心跳信息
func (c *StatusServerClient) SendHeartbeat(stakeNo string) error {
	url := fmt.Sprintf("%s/equipmentfailure/sendBeat", c.baseURL)
	data := map[string]string{
		"stakeNo":    stakeNo,
		"updateDate": time.Now().Format("2006-01-02 15:04:05"),
		"loraStatus": "Online",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	resp, err := c.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回非 200 状态码: %d", resp.StatusCode)
	}
	return nil
}
