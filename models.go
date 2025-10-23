package main

// --- 单播 API 模型  ---

// SetColorCommand 对应设置颜色的请求体
type SetColorCommand struct {
	StakeNo string `json:"stakeNo" binding:"required"`
	Color   int    `json:"color" binding:"oneof=0 1"`
}

// SetFrequencyCommand 对应设置频率的请求体
type SetFrequencyCommand struct {
	StakeNo   string `json:"stakeNo" binding:"required"`
	Frequency int    `json:"frequency" binding:"oneof=30 60 120"`
}

// SetLevelCommand 对应设置亮度的请求体
type SetLevelCommand struct {
	StakeNo string `json:"stakeNo" binding:"required"`
	Level   int    `json:"level" binding:"oneof=500 1000 2000 4000 7000"`
}

// SetMannerCommand 对应设置亮灯方式的请求体
type SetMannerCommand struct {
	StakeNo string `json:"stakeNo" binding:"required"`
	Manner  int    `json:"manner" binding:"oneof=0 1"`
}

// SetSwitchCommand 对应设置开关的请求体
type SetSwitchCommand struct {
	StakeNo string `json:"stakeNo" binding:"required"`
	Switch  int    `json:"switch" binding:"oneof=0 1"`
}

// OverallSettingCommand 对应整体设置的请求体
type OverallSettingCommand struct {
	StakeNo   string `json:"stakeNo" binding:"required"`
	Color     int    `json:"color" binding:"oneof=0 1"`
	Frequency int    `json:"frequency" binding:"oneof=30 60 120"`
	Level     int    `json:"level" binding:"oneof=500 1000 2000 4000 7000"`
	Manner    int    `json:"manner" binding:"oneof=0 1"`
}

// UplinkEvent 对应 ChirpStack 上行事件的 JSON 结构
type UplinkEvent struct {
	DeviceInfo struct {
		DevEui string `json:"devEui"`
	} `json:"deviceInfo"`
	Data string `json:"data"`
}

// --- 新增：多播 API 模型 ---

type MulticastSetColorCommand struct {
	GroupID string `json:"groupId" binding:"required"`
	Color   int    `json:"color" binding:"oneof=0 1"`
}

type MulticastSetFrequencyCommand struct {
	GroupID   string `json:"groupId" binding:"required"`
	Frequency int    `json:"frequency" binding:"required,oneof=30 60 120"`
}

type MulticastSetLevelCommand struct {
	GroupID string `json:"groupId" binding:"required"`
	Level   int    `json:"level" binding:"required,oneof=500 1000 2000 4000 7000"`
}

type MulticastSetMannerCommand struct {
	GroupID string `json:"groupId" binding:"required"`
	Manner  int    `json:"manner" binding:"oneof=0 1"`
}

type MulticastSetSwitchCommand struct {
	GroupID string `json:"groupId" binding:"required"`
	Switch  int    `json:"switch" binding:"oneof=0 1"`
}

type MulticastCharacterCommand struct {
	GroupID string `json:"groupId" binding:"required"`
	Switch  int    `json:"switch" binding:"oneof=0 1"`
}
type MulticastOverallSettingCommand struct {
	GroupID   string `json:"groupId" binding:"required"`
	Color     int    `json:"color" binding:"oneof=0 1"`
	Frequency int    `json:"frequency" binding:"required,oneof=30 60 120"`
	Level     int    `json:"level" binding:"required,oneof=500 1000 2000 4000 7000"`
	Manner    int    `json:"manner" binding:"oneof=0 1"`
}

// 新增，传递多播组参数给单个设备
type SetMulticastGroupCommand struct {
	StakeNo string `json:"stakeNo" binding:"required"`
	DevAddr string `json:"devAddr" binding:"required,len=8"`
	AppSKey string `json:"appSKey" binding:"required,len=32"`
	NwkSKey string `json:"nwkSKey" binding:"required,len=32"`
}

// 设置加速度检测模式的请求体
// devEUI: 设备唯一标识，enable: 1=打开，0=关闭
// 用于POST /api/device/set-acceleration-mode
type SetAccelerationModeCommand struct {
	DevEUI string `json:"devEUI" binding:"required"`
	Enable int    `json:"enable" binding:"oneof=0 1"`
}
