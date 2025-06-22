package main

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chirpstack-httpserver/services"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// commandHandlerFunc 定义了处理上行命令的函数签名
type commandHandlerFunc func(h *Handler, devEUI string, data []byte) error

// handleTimeSync 处理时间同步请求 (原 case 0x06)
func handleTimeSync(h *Handler, devEUI string, data []byte) error {
	log.Info().Str("devEUI", devEUI).Msg("处理延迟测量请求")

	// 【1】 获取当前UTC
	nowUTC := time.Now().UTC()

	// 【2】 计算当天午夜UTC时间点
	midnightUTC := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)

	// 【3】 计算自午夜以来经过的毫秒数
	durationSinceMidnight := nowUTC.Sub(midnightUTC)
	msSinceMidnight := uint32(durationSinceMidnight.Milliseconds())

	// 【4】 将毫秒数(uint32)序列化为4个字节 (Big Endian)
	timeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(timeBytes, msSinceMidnight)

	// 【5】 构建最终的下行数据包：命令码 + 时间数据
	payload := append([]byte{0x06}, timeBytes...)

	// 【6】 日志
	log.Info().
		Str("devEUI", devEUI).
		Time("nowUTC", nowUTC).
		Uint32("msSinceMidnight", msSinceMidnight).
		Hex("payload", payload). // 以十六进制格式记录最终的数据包
		Msg("准备发送时间同步下行数据")

	downlinkID, err := h.csClient.SendDownlink(devEUI, 1, false, payload)
	if err != nil {
		// 返回错误，由上层统一处理日志
		return fmt.Errorf("发送下行消息失败: %w", err)
	}

	log.Info().Str("devEUI", devEUI).Str("downlinkID", downlinkID).Msg("已发送时间同步响应")
	return nil
}

// handleManualAlarm 处理人工报警 (原 case 0x07)
func handleManualAlarm(h *Handler, devEUI string, data []byte) error {
	log.Info().Str("devEUI", devEUI).Msg("处理人工报警")
	if err := h.statusClient.SendWarnInfo(devEUI, 1); err != nil {
		return fmt.Errorf("转发人工报警到状态服务器失败: %w", err)
	}
	log.Info().Str("devEUI", devEUI).Msg("成功转发人工报警")
	return nil
}

// handleAccidentAlarm 处理事故报警 (原 case 0x08)
func handleAccidentAlarm(h *Handler, devEUI string, data []byte) error {
	log.Info().Str("devEUI", devEUI).Msg("处理事故报警")
	if err := h.statusClient.SendWarnInfo(devEUI, 2); err != nil {
		return fmt.Errorf("转发事故报警到状态服务器失败: %w", err)
	}
	log.Info().Str("devEUI", devEUI).Msg("成功转发事故报警")
	return nil
}

// handleHeartbeat 处理心跳 (原 case 0x09)
func handleHeartbeat(h *Handler, devEUI string, data []byte) error {
	log.Info().Str("devEUI", devEUI).Msg("处理心跳数据")
	if err := h.statusClient.SendHeartbeat(devEUI); err != nil {
		return fmt.Errorf("转发心跳到状态服务器失败: %w", err)
	}
	log.Info().Str("devEUI", devEUI).Msg("成功转发心跳")
	return nil
}

// commandHandlers 是一个从命令码到其处理函数的映射（注册表）
var commandHandlers = map[byte]commandHandlerFunc{
	0x06: handleTimeSync,
	0x07: handleManualAlarm,
	0x08: handleAccidentAlarm,
	0x09: handleHeartbeat,
}

// Handler 结构体持有所有依赖，如服务客户端
type Handler struct {
	csClient     *services.ChirpStackClient
	statusClient *services.StatusServerClient
}

// NewHandler 创建一个新的 Handler
func NewHandler(cs *services.ChirpStackClient, ss *services.StatusServerClient) *Handler {
	return &Handler{
		csClient:     cs,
		statusClient: ss,
	}
}

// RegisterRoutes 注册所有 API 路由
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	// ChirpStack 事件回调
	router.POST("/integration/uplink", h.handleChirpStackEvent)

	// 外部 API
	apiGroup := router.Group("/api")
	{
		lights := apiGroup.Group("/induction-lights")
		{
			lights.POST("/set-color", h.handleSetColor)
			lights.POST("/set-frequency", h.handleSetFrequency)
			lights.POST("/set-level", h.handleSetLevel)
			lights.POST("/set-manner", h.handleSetManner)
			lights.POST("/set-switch", h.handleSetSwitch)
			lights.POST("/overall-setting", h.handleOverallSetting)
		}
	}
}

// handleChirpStackEvent 处理来自 ChirpStack 的上行数据
func (h *Handler) handleChirpStackEvent(c *gin.Context) {
	event := c.Query("event")
	if event != "up" {
		log.Warn().Str("event", event).Msg("接收到非 up 事件，已忽略")
		c.JSON(http.StatusOK, gin.H{"message": "event ignored"})
		return
	}

	var uplink UplinkEvent
	if err := c.ShouldBindJSON(&uplink); err != nil {
		log.Error().Err(err).Msg("解析上行事件 JSON 失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	devEUI := uplink.DeviceInfo.DevEui
	log.Info().Str("devEUI", devEUI).Msg("收到上行数据")

	decodedData, err := base64.StdEncoding.DecodeString(uplink.Data)
	if err != nil {
		log.Error().Err(err).Str("devEUI", devEUI).Msg("Base64 解码失败")
		return
	}

	if len(decodedData) == 0 {
		log.Warn().Str("devEUI", devEUI).Msg("数据负载为空")
		return
	}

	cmdCode := decodedData[0]
	handlerFunc, found := commandHandlers[cmdCode]
	if !found {
		log.Warn().Int("cmdCode", int(cmdCode)).Str("devEUI", devEUI).Msg("未知的命令码")
		c.Status(http.StatusOK)
		return
	}

	// 3. 执行具体的处理器
	if err := handlerFunc(h, devEUI, decodedData); err != nil {
		// 处理器内部已经记录了详细错误，这里只记录分派层面的失败信息
		log.Error().Err(err).Str("devEUI", devEUI).Int("cmdCode", int(cmdCode)).Msg("命令处理失败")
	}
	c.Status(http.StatusOK)
}

// 统一的下行处理逻辑
func (h *Handler) processDownlinkCommands(c *gin.Context, commands interface{}, processFunc func(cmd map[string]interface{}) error) {
	var cmdList []map[string]interface{}
	if err := c.ShouldBindJSON(&cmdList); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的JSON格式或请求体不是数组"})
		return
	}

	for _, cmd := range cmdList {
		if err := processFunc(cmd); err != nil {
			// 具体的错误已在 processFunc 中记录，这里直接返回
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Setting applied successfully."})
}

// handleSetColor 处理设置颜色请求
func (h *Handler) handleSetColor(c *gin.Context) {
	// log.Info().Msg("here")
	var commands []SetColorCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	for _, cmd := range commands {
		devEUIs := strings.Split(cmd.StakeNo, ",")
		for _, devEUI := range devEUIs {
			data := []byte{byte(cmd.Color)}
			id, err := h.csClient.SendDownlink(devEUI, 11, false, data)
			if err != nil {
				log.Error().Err(err).Str("devEUI", devEUI).Msg("发送颜色设置失败")
				continue // 继续处理下一个
			}
			log.Info().Str("devEUI", devEUI).Int("color", cmd.Color).Str("downlinkID", id).Msg("颜色设置下行已发送")
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Color setting applied successfully."})
}

// handleSetFrequency 处理设置频率请求
func (h *Handler) handleSetFrequency(c *gin.Context) {
	var commands []SetFrequencyCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	freqMap := map[int]byte{30: 0x1E, 60: 0x3C, 120: 0x78}

	for _, cmd := range commands {
		devEUIs := strings.Split(cmd.StakeNo, ",")
		for _, devEUI := range devEUIs {
			data := []byte{freqMap[cmd.Frequency]}
			id, err := h.csClient.SendDownlink(devEUI, 10, false, data)
			if err != nil {
				log.Error().Err(err).Str("devEUI", devEUI).Msg("发送频率设置失败")
				continue
			}
			log.Info().Str("devEUI", devEUI).Int("frequency", cmd.Frequency).Str("downlinkID", id).Msg("频率设置下行已发送")
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Frequency setting applied successfully."})
}

// handleSetLevel 处理设置亮度请求
func (h *Handler) handleSetLevel(c *gin.Context) {
	var commands []SetLevelCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	for _, cmd := range commands {
		devEUIs := strings.Split(cmd.StakeNo, ",")
		for _, devEUI := range devEUIs {
			highByte := byte(cmd.Level >> 8 & 0xFF)
			lowByte := byte(cmd.Level & 0xFF)
			data := []byte{highByte, lowByte}
			id, err := h.csClient.SendDownlink(devEUI, 13, false, data)
			if err != nil {
				log.Error().Err(err).Str("devEUI", devEUI).Msg("发送亮度设置失败")
				continue
			}
			log.Info().Str("devEUI", devEUI).Int("level", cmd.Level).Str("downlinkID", id).Msg("亮度设置下行已发送")
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Level setting applied successfully."})
}

// handleSetManner 处理设置亮灯方式请求
func (h *Handler) handleSetManner(c *gin.Context) {
	var commands []SetMannerCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	for _, cmd := range commands {
		devEUIs := strings.Split(cmd.StakeNo, ",")
		for _, devEUI := range devEUIs {
			data := []byte{byte(cmd.Manner)}
			id, err := h.csClient.SendDownlink(devEUI, 12, false, data)
			if err != nil {
				log.Error().Err(err).Str("devEUI", devEUI).Msg("发送亮灯方式设置失败")
				continue
			}
			log.Info().Str("devEUI", devEUI).Int("manner", cmd.Manner).Str("downlinkID", id).Msg("亮灯方式设置下行已发送")
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Manner setting applied successfully."})
}

// handleSetSwitch 处理设置开关请求
func (h *Handler) handleSetSwitch(c *gin.Context) {
	var commands []SetSwitchCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	for _, cmd := range commands {
		devEUIs := strings.Split(cmd.StakeNo, ",")
		for _, devEUI := range devEUIs {
			data := []byte{byte(cmd.Switch)}
			id, err := h.csClient.SendDownlink(devEUI, 14, false, data)
			if err != nil {
				log.Error().Err(err).Str("devEUI", devEUI).Msg("发送开关设置失败")
				continue
			}
			log.Info().Str("devEUI", devEUI).Int("switch", cmd.Switch).Str("downlinkID", id).Msg("开关设置下行已发送")
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Switch setting applied successfully."})
}

// handleOverallSetting 处理整体设置请求
func (h *Handler) handleOverallSetting(c *gin.Context) {
	var commands []OverallSettingCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	freqMap := map[int]byte{30: 0x1E, 60: 0x3C, 120: 0x78}

	for _, cmd := range commands {
		payload := []byte{
			byte(cmd.Color),
			freqMap[cmd.Frequency],
			byte(cmd.Level >> 8 & 0xFF),
			byte(cmd.Level & 0xFF),
			byte(cmd.Manner),
		}

		devEUIs := strings.Split(cmd.StakeNo, ",")
		for _, devEUI := range devEUIs {
			id, err := h.csClient.SendDownlink(devEUI, 15, false, payload)
			if err != nil {
				log.Error().Err(err).Str("devEUI", devEUI).Msg("发送整体设置失败")
				continue
			}
			log.Info().Str("devEUI", devEUI).Str("downlinkID", id).Msg("整体设置下行已发送")
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Overall setting applied successfully."})
}
