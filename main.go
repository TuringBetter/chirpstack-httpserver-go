package main

import (
	"chirpstack-httpserver/config"
	"chirpstack-httpserver/services"
	"os"

	"time"

	"github.com/gin-gonic/gin"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {

	// 日志轮转：每天零点轮转，文件名为 httpserver.log.2025-07-04，主日志为 httpserver.log
	// location, err := time.LoadLocation("Asia/Shanghai")
	// if err != nil {
	// 	log.Fatal().Err(err).Msg("无法加载上海时区")
	// }
	rotator, err := rotatelogs.New(
		"httpserver.log.%Y-%m-%d",
		rotatelogs.WithLinkName("httpserver.log"), // 始终指向当前日志
		rotatelogs.WithRotationTime(24*time.Hour), // 每天轮转
		rotatelogs.WithClock(rotatelogs.Local),    // 本地时区
	)
	if err != nil {
		log.Fatal().Err(err).Msg("无法创建日志轮转器")
	}

	// 初始化结构化日志库 Zerolog，输出到文件 同时保留控制台输出（可选）
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr}
	fileWriter := zerolog.ConsoleWriter{
		Out:        rotator,
		NoColor:    true,
		TimeFormat: "2006-01-02 15:04:05",
	}

	// 多输出，同时输出到控制台和文件
	multiWriter := zerolog.MultiLevelWriter(consoleWriter, fileWriter)
	log.Logger = zerolog.New(multiWriter).With().Timestamp().Logger()

	// 加载配置
	cfg := config.LoadConfig()
	log.Info().Msg("配置加载成功")

	// 初始化 ChirpStack 客户端
	csClient, err := services.NewChirpStackClient(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("无法连接到 ChirpStack gRPC 服务器")
	}
	log.Info().Str("server", cfg.ChirpStackServer).Msg("ChirpStack gRPC 客户端初始化成功")

	// 初始化状态服务器客户端
	statusClient := services.NewStatusServerClient(cfg)
	log.Info().Str("url", cfg.StatusServerURL).Msg("状态服务器客户端初始化成功")

	// 初始化 Gin 引擎
	router := gin.Default()

	// 创建并注册路由
	handler := NewHandler(csClient, statusClient, cfg)
	handler.RegisterRoutes(router)
	log.Info().Msg("API 路由注册成功")

	// 启动 HTTP 服务
	log.Info().Str("address", cfg.ListenAddress).Msg("HTTP 服务即将启动")
	if err := router.Run(cfg.ListenAddress); err != nil {
		log.Fatal().Err(err).Msg("HTTP 服务启动失败")
	}
}
