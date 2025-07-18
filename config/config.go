package config

import (
	"time"

	"github.com/spf13/viper"
)

// Config 结构体用于存放所有应用配置
type Config struct {
	ChirpStackServer string            `mapstructure:"chirpstack_server"`
	APIToken         string            `mapstructure:"api_token"`
	StatusServerURL  string            `mapstructure:"status_server_url"`
	ListenAddress    string            `mapstructure:"listen_address"`
	GRPCTimeout      time.Duration     `mapstructure:"grpc_timeout"`
	HTTPTimeout      time.Duration     `mapstructure:"http_timeout"`
	MulticastGroups  map[string]string `mapstructure:"multicast_groups"`
}

// LoadConfig 加载并返回配置
func LoadConfig() Config {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")

	// 设置默认值
	viper.SetDefault("grpc_timeout", "5s")
	viper.SetDefault("http_timeout", "5s")

	err := viper.ReadInConfig()
	if err != nil {
		panic("无法读取配置文件: " + err.Error())
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		panic("配置文件解析失败: " + err.Error())
	}

	// 处理超时时间（字符串转time.Duration）
	if d, err := time.ParseDuration(viper.GetString("grpc_timeout")); err == nil {
		cfg.GRPCTimeout = d
	}
	if d, err := time.ParseDuration(viper.GetString("http_timeout")); err == nil {
		cfg.HTTPTimeout = d
	}

	return cfg
}
