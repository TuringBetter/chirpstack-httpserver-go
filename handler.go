package main

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"time"

	"chirpstack-httpserver/config"
	"chirpstack-httpserver/services"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// commandHandlerFunc 定义了处理上行命令的函数签名
type commandHandlerFunc func(h *Handler, devEUI string, data []byte) error

// commandHandlers 是一个从命令码到其处理函数的映射（注册表）
var commandHandlers = map[byte]commandHandlerFunc{
	0x06: handleTimeSync,
	0x07: handleManualAlarm,
	0x08: handleAccidentAlarm,
	0x09: handleHeartbeat,
}

// handleTimeSync 处理时间同步请求 (原 case 0x06)
func handleTimeSync(h *Handler, devEUI string, data []byte) error {
	log.Info().Str("devEUI", devEUI).Msg("处理延迟测量请求")

	// 【1】 获取当前CTS
	nowCST := time.Now().In(time.FixedZone("CST", 8*60*60))

	// 【2】 计算当天午夜CTS时间点
	midnightCST := time.Date(nowCST.Year(), nowCST.Month(), nowCST.Day(), 0, 0, 0, 0, nowCST.Location())

	// 【3】 计算自午夜以来经过的毫秒数
	durationSinceMidnight := nowCST.Sub(midnightCST)
	msSinceMidnight := uint32(durationSinceMidnight.Milliseconds())

	// 【4】 将毫秒数(uint32)序列化为4个字节 (Big Endian)
	timeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(timeBytes, msSinceMidnight)

	// 【5】 构建最终的下行数据包：命令码 + 时间数据
	payload := timeBytes

	// 【6】 日志
	log.Info().
		Str("devEUI", devEUI).
		Time("nowCTS", nowCST).
		Uint32("msSinceMidnight", msSinceMidnight).
		Hex("payload", payload). // 以十六进制格式记录最终的数据包
		Msg("准备发送时间同步下行数据")

	downlinkID, err := h.csClient.SendDownlink(devEUI, 9, false, payload)
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

// Handler 结构体持有所有依赖，如服务客户端
type Handler struct {
	csClient     *services.ChirpStackClient
	statusClient *services.StatusServerClient
	config       config.Config
}

// NewHandler 创建一个新的 Handler
func NewHandler(cs *services.ChirpStackClient, ss *services.StatusServerClient, cfg config.Config) *Handler {
	return &Handler{
		csClient:     cs,
		statusClient: ss,
		config:       cfg,
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

	// 新增：多播 API
	multicastGroup := apiGroup.Group("/multicast-groups")
	{
		multicastGroup.POST("/set-color", h.handleMulticastSetColor)
		multicastGroup.POST("/set-frequency", h.handleMulticastSetFrequency)
		multicastGroup.POST("/set-level", h.handleMulticastSetLevel)
		multicastGroup.POST("/set-manner", h.handleMulticastSetManner)
		multicastGroup.POST("/set-switch", h.handleMulticastSetSwitch)
		multicastGroup.POST("/overall-setting", h.handleMulticastSetOverall)
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

// handleSetColor 处理设置颜色请求
func (h *Handler) handleSetColor(c *gin.Context) {
	// log.Info().Msg("here")
	var commands []SetColorCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// 只能处理一个命令
	if len(commands) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Request body must contain at least one command."})
		return
	}
	cmd := commands[0]

	// 取出控制命令参数
	devEUI := cmd.StakeNo
	data := []byte{byte(cmd.Color)}
	id, err := h.csClient.SendDownlink(devEUI, 11, false, data)
	if err != nil {
		log.Error().Err(err).Str("devEUI", devEUI).Msg("发送颜色设置失败")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to send downlink."})
		return
	}
	log.Info().Str("devEUI", devEUI).Int("color", cmd.Color).Str("downlinkID", id).Msg("颜色设置下行已发送")
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

	// 只能处理一个命令
	if len(commands) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Request body must contain at least one command."})
		return
	}
	cmd := commands[0]

	// 取出控制命令参数
	devEUI := cmd.StakeNo
	data := []byte{freqMap[cmd.Frequency]}
	id, err := h.csClient.SendDownlink(devEUI, 10, false, data)
	if err != nil {
		log.Error().Err(err).Str("devEUI", devEUI).Msg("发送频率设置失败")
	}
	log.Info().Str("devEUI", devEUI).Int("frequency", cmd.Frequency).Str("downlinkID", id).Msg("频率设置下行已发送")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Frequency setting applied successfully."})
}

// handleSetLevel 处理设置亮度请求
func (h *Handler) handleSetLevel(c *gin.Context) {
	var commands []SetLevelCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	// 只能处理一个命令
	if len(commands) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Request body must contain at least one command."})
		return
	}
	// 取出控制命令参数
	cmd := commands[0]

	devEUI := cmd.StakeNo
	highByte := byte(cmd.Level >> 8 & 0xFF)
	lowByte := byte(cmd.Level & 0xFF)
	data := []byte{highByte, lowByte}
	id, err := h.csClient.SendDownlink(devEUI, 13, false, data)
	if err != nil {
		log.Error().Err(err).Str("devEUI", devEUI).Msg("发送亮度设置失败")
	}
	log.Info().Str("devEUI", devEUI).Int("level", cmd.Level).Str("downlinkID", id).Msg("亮度设置下行已发送")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Level setting applied successfully."})
}

// handleSetManner 处理设置亮灯方式请求
func (h *Handler) handleSetManner(c *gin.Context) {
	var commands []SetMannerCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// 只能处理一个命令
	if len(commands) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Request body must contain at least one command."})
		return
	}
	// 取出控制命令参数
	cmd := commands[0]
	devEUI := cmd.StakeNo
	data := []byte{byte(cmd.Manner)}
	id, err := h.csClient.SendDownlink(devEUI, 12, false, data)
	if err != nil {
		log.Error().Err(err).Str("devEUI", devEUI).Msg("发送亮灯方式设置失败")
	}
	log.Info().Str("devEUI", devEUI).Int("manner", cmd.Manner).Str("downlinkID", id).Msg("亮灯方式设置下行已发送")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Manner setting applied successfully."})
}

// handleSetSwitch 处理设置开关请求
func (h *Handler) handleSetSwitch(c *gin.Context) {
	var commands []SetSwitchCommand
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	// 只能处理一个命令
	if len(commands) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Request body must contain at least one command."})
		return
	}
	// 取出控制命令参数
	cmd := commands[0]
	devEUI := cmd.StakeNo
	data := []byte{byte(cmd.Switch)}
	id, err := h.csClient.SendDownlink(devEUI, 14, false, data)
	if err != nil {
		log.Error().Err(err).Str("devEUI", devEUI).Msg("发送开关设置失败")
	}
	log.Info().Str("devEUI", devEUI).Int("switch", cmd.Switch).Str("downlinkID", id).Msg("开关设置下行已发送")
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

	// 只能处理一个命令
	if len(commands) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Request body must contain at least one command."})
		return
	}
	// 取出控制命令参数
	cmd := commands[0]
	devEUI := cmd.StakeNo
	payload := []byte{
		byte(cmd.Color),
		freqMap[cmd.Frequency],
		byte(cmd.Level >> 8 & 0xFF),
		byte(cmd.Level & 0xFF),
		byte(cmd.Manner),
	}

	id, err := h.csClient.SendDownlink(devEUI, 15, false, payload)
	if err != nil {
		log.Error().Err(err).Str("devEUI", devEUI).Msg("发送整体设置失败")
	}
	log.Info().Str("devEUI", devEUI).Str("downlinkID", id).Msg("整体设置下行已发送")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Overall setting applied successfully."})
}

// handleMulticastSetColor 处理多播组的颜色设置请求
func (h *Handler) handleMulticastSetColor(c *gin.Context) {
	var cmd MulticastSetColorCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Invalid request: " + err.Error()})
		return
	}

	// 从映射中查找多播组 UUID
	multicastGroupID, found := h.config.MulticastGroups[cmd.GroupID]
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Unknown groupId: " + cmd.GroupID})
		return
	}

	// 准备数据包并发送
	data := []byte{byte(cmd.Color)}
	_, err := h.csClient.EnqueueMulticast(multicastGroupID, 11, data)
	if err != nil {
		log.Error().Err(err).Str("multicastGroupID", multicastGroupID).Msg("发送多播颜色设置失败")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to enqueue multicast downlink."})
		return
	}

	// log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("color", cmd.Color).Str("downlinkID", id).Msg("多播颜色设置已入队")
	log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("color", cmd.Color).Msg("多播颜色设置已入队")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Multicast color setting enqueued successfully."})
}

// handleMulticastSetFrequency 处理多播组的频率设置请求
func (h *Handler) handleMulticastSetFrequency(c *gin.Context) {
	var cmd MulticastSetFrequencyCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Invalid request: " + err.Error()})
		return
	}

	multicastGroupID, found := h.config.MulticastGroups[cmd.GroupID]
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Unknown groupId: " + cmd.GroupID})
		return
	}

	freqMap := map[int]byte{30: 0x1E, 60: 0x3C, 120: 0x78}
	data := []byte{freqMap[cmd.Frequency]}

	_, err := h.csClient.EnqueueMulticast(multicastGroupID, 10, data)
	if err != nil {
		log.Error().Err(err).Str("multicastGroupID", multicastGroupID).Msg("发送多播频率设置失败")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to enqueue multicast downlink."})
		return
	}

	// log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("frequency", cmd.Frequency).Str("downlinkID", id).Msg("多播频率设置已入队")
	log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("frequency", cmd.Frequency).Msg("多播频率设置已入队")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Multicast frequency setting enqueued successfully."})
}

// handleMulticastSetLevel 处理多播组的亮度设置请求
func (h *Handler) handleMulticastSetLevel(c *gin.Context) {
	var cmd MulticastSetLevelCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Invalid request: " + err.Error()})
		return
	}

	multicastGroupID, found := h.config.MulticastGroups[cmd.GroupID]
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Unknown groupId: " + cmd.GroupID})
		return
	}

	highByte := byte(cmd.Level >> 8 & 0xFF)
	lowByte := byte(cmd.Level & 0xFF)
	data := []byte{highByte, lowByte}

	_, err := h.csClient.EnqueueMulticast(multicastGroupID, 13, data)
	if err != nil {
		log.Error().Err(err).Str("multicastGroupID", multicastGroupID).Msg("发送多播亮度设置失败")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to enqueue multicast downlink."})
		return
	}

	// log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("level", cmd.Level).Str("downlinkID", id).Msg("多播亮度设置已入队")
	log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("level", cmd.Level).Msg("多播亮度设置已入队")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Multicast level setting enqueued successfully."})
}

// handleMulticastSetManner 处理多播组的亮灯方式设置请求
func (h *Handler) handleMulticastSetManner(c *gin.Context) {
	var cmd MulticastSetMannerCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Invalid request: " + err.Error()})
		return
	}

	multicastGroupID, found := h.config.MulticastGroups[cmd.GroupID]
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Unknown groupId: " + cmd.GroupID})
		return
	}

	data := []byte{byte(cmd.Manner)}

	_, err := h.csClient.EnqueueMulticast(multicastGroupID, 12, data)
	if err != nil {
		log.Error().Err(err).Str("multicastGroupID", multicastGroupID).Msg("发送多播亮灯方式设置失败")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to enqueue multicast downlink."})
		return
	}

	// log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("Manner", cmd.Manner).Str("downlinkID", id).Msg("多播亮度设置已入队")
	log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("Manner", cmd.Manner).Msg("多播亮灯方式设置已入队")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Multicast level setting enqueued successfully."})
}

// handleMulticastSetSwitch 处理多播组开关设置请求
func (h *Handler) handleMulticastSetSwitch(c *gin.Context) {
	var cmd MulticastSetSwitchCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Invalid request: " + err.Error()})
		return
	}

	multicastGroupID, found := h.config.MulticastGroups[cmd.GroupID]
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Unknown groupId: " + cmd.GroupID})
		return
	}

	data := []byte{byte(cmd.Switch)}

	_, err := h.csClient.EnqueueMulticast(multicastGroupID, 14, data)
	if err != nil {
		log.Error().Err(err).Str("multicastGroupID", multicastGroupID).Msg("发送多播开关设置失败")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to enqueue multicast downlink."})
		return
	}

	// log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("Switch", cmd.Switch).Str("downlinkID", id).Msg("多播亮度设置已入队")
	log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Int("Switch", cmd.Switch).Msg("多播开关设置已入队")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Multicast level setting enqueued successfully."})
}

// handleMulticastSetOverall 处理多播组开关设置请求
func (h *Handler) handleMulticastSetOverall(c *gin.Context) {
	var cmd MulticastOverallSettingCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Invalid request: " + err.Error()})
		return
	}

	multicastGroupID, found := h.config.MulticastGroups[cmd.GroupID]
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Unknown groupId: " + cmd.GroupID})
		return
	}

	// data := []byte{byte(cmd.Switch)}
	freqMap := map[int]byte{30: 0x1E, 60: 0x3C, 120: 0x78}
	payload := []byte{
		byte(cmd.Color),
		freqMap[cmd.Frequency],
		byte(cmd.Level >> 8 & 0xFF),
		byte(cmd.Level & 0xFF),
		byte(cmd.Manner),
	}
	_, err := h.csClient.EnqueueMulticast(multicastGroupID, 15, payload)
	if err != nil {
		log.Error().Err(err).Str("multicastGroupID", multicastGroupID).Msg("发送多播总体设置失败")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to enqueue multicast downlink."})
		return
	}

	// log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Str("downlinkID", id).Msg("多播亮度设置已入队")
	log.Info().Str("groupId", cmd.GroupID).Str("multicastUUID", multicastGroupID).Msg("多播总体设置已入队")
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Multicast level setting enqueued successfully."})
}
