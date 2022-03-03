package main

import (
	"flag"
	"fmt"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/internal"
	"github.com/devtron-labs/kubelink/pkg/service"
	"go.uber.org/zap"
	_ "go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"net"
	"time"
)

func main() {
	config := zap.NewProductionConfig()
	log, err := config.Build()
	if err != nil {
		panic("erorr in building logger")
	}
	logger := log.Sugar()
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 50051))
	if err != nil {
		logger.Fatalw("failed to listen: %v", "err", err)
	}
	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge: 10 * time.Second,
		}),
	}
	grpcServer := grpc.NewServer(opts...)

	// create repository locker singleton
	chartRepositoryLocker := internal.NewChartRepositoryLocker(logger)

	client.RegisterApplicationServiceServer(grpcServer, service.NewApplicationServiceServerImpl(logger, chartRepositoryLocker))
	logger.Info("starting server ...................")
	err = grpcServer.Serve(lis)
	if err != nil {
		logger.Fatalw("failed to listen: %v", "err", err)

	}
}