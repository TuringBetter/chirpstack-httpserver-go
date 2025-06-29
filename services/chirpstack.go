package services

import (
	"chirpstack-httpserver/config"
	"context"

	"github.com/chirpstack/chirpstack/api/go/v4/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ChirpStackClient 封装了与 ChirpStack 的交互
type ChirpStackClient struct {
	client          api.DeviceServiceClient
	multicastClient api.MulticastGroupServiceClient
	authToken       []grpc.CallOption
	config          config.Config
}

// APIToken 实现了 gRPC 的 PerRPCCredentials 接口
type APIToken string

func (a APIToken) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + string(a),
	}, nil
}

func (a APIToken) RequireTransportSecurity() bool {
	return false
}

// NewChirpStackClient 创建一个新的 ChirpStack 客户端
func NewChirpStackClient(cfg config.Config) (*ChirpStackClient, error) {
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(APIToken(cfg.APIToken)),
	}

	conn, err := grpc.Dial(cfg.ChirpStackServer, dialOpts...)
	if err != nil {
		return nil, err
	}

	// client := api.NewDeviceServiceClient(conn)
	return &ChirpStackClient{
		client:          api.NewDeviceServiceClient(conn),
		multicastClient: api.NewMulticastGroupServiceClient(conn),
		config:          cfg,
	}, nil
}

// SendDownlink 发送下行消息
func (c *ChirpStackClient) SendDownlink(devEUI string, fPort uint32, confirmed bool, data []byte) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.GRPCTimeout)
	defer cancel()

	req := &api.EnqueueDeviceQueueItemRequest{
		QueueItem: &api.DeviceQueueItem{
			DevEui:    devEUI,
			FPort:     fPort,
			Confirmed: confirmed,
			Data:      data,
		},
	}

	resp, err := c.client.Enqueue(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Id, nil
}

// EnqueueMulticast 发送多播下行消息
func (c *ChirpStackClient) EnqueueMulticast(multicastGroupID string, fPort uint32, data []byte) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.GRPCTimeout)
	defer cancel()

	req := &api.EnqueueMulticastGroupQueueItemRequest{
		QueueItem: &api.MulticastGroupQueueItem{
			MulticastGroupId: multicastGroupID,
			FPort:            fPort,
			Data:             data,
		},
	}
	_, err := c.multicastClient.Enqueue(ctx, req)
	if err != nil {
		return "", err
	}

	return "muticast enqueue succeed", nil
}
