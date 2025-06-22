package main

import (
	"chirpstack-httpserver/config"
	"chirpstack-httpserver/services"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {

	// 打开日志文件
	logFile, err := os.OpenFile(
		"log.txt",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0664,
	)

	if err != nil {
		log.Fatal().Err(err).Msg("无法打开日志文件")
	}

	defer logFile.Close()

	// 初始化结构化日志库 Zerolog，输出到文件 同时保留控制台输出（可选）
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr}
	fileWriter := zerolog.ConsoleWriter{
		Out:        logFile,
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
	handler := NewHandler(csClient, statusClient)
	handler.RegisterRoutes(router)
	log.Info().Msg("API 路由注册成功")

	// 启动 HTTP 服务
	log.Info().Str("address", cfg.ListenAddress).Msg("HTTP 服务即将启动")
	if err := router.Run(cfg.ListenAddress); err != nil {
		log.Fatal().Err(err).Msg("HTTP 服务启动失败")
	}
}
