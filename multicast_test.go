package main

import (
	"chirpstack-httpserver/config"
	"context"
	"fmt"
	"testing"

	"github.com/chirpstack/chirpstack/api/go/v4/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	multicastGroupID = "e81cd77b-f1e9-40fc-87ba-10e1fc935596"
)

// APIToken 结构体用于实现 gRPC 的 PerRPCCredentials 接口，从而在每个API请求中附加认证信息
type APIToken string

func (a APIToken) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": fmt.Sprintf("Bearer %s", a),
	}, nil
}

func (a APIToken) RequireTransportSecurity() bool {
	// 在生产环境中如果启用了TLS，这里应该返回 true
	return false
}

func TestMultiCast(t *testing.T) {

	// 加载配置
	cfg := config.LoadConfig()

	dialOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(APIToken(cfg.APIToken)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	conn, err := grpc.Dial(cfg.ChirpStackServer, dialOpts...)
	if err != nil {
		fmt.Println("gRPC dial error: ", err)
	}

	defer conn.Close()

	fmt.Println("Successfully connected to ChirpStack API at ", cfg.ChirpStackServer)

	// ===================================================================================
	// 关键部分：使用 MulticastGroupServiceClient
	// ===================================================================================
	// 1 创建多播组服务的客户端
	multicastClient := api.NewMulticastGroupServiceClient(conn)

	// 2 构造发送请求
	req := &api.EnqueueMulticastGroupQueueItemRequest{
		QueueItem: &api.MulticastGroupQueueItem{
			MulticastGroupId: multicastGroupID,
			FPort:            10,
			Data:             []byte{0x01, 0x02, 0x03},
		},
	}
	// 3 发送请求
	_, err = multicastClient.Enqueue(context.Background(), req)
	if err != nil {
		fmt.Println("Failed to enqueue multicast downlink:", err)
	}

}
