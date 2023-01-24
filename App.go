package main

import (
	"fmt"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/service"
	"github.com/devtron-labs/kubelink/pprof"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"log"
	"net"
	"time"
)

type App struct {
	Logger     *zap.SugaredLogger
	ServerImpl *service.ApplicationServiceServerImpl
	router     *pprof.PProfRouterImpl
}

func NewApp(Logger *zap.SugaredLogger, ServerImpl *service.ApplicationServiceServerImpl,
	router *pprof.PProfRouterImpl) *App {
	return &App{
		Logger:     Logger,
		ServerImpl: ServerImpl,
		router:     router,
	}
}

func (app *App) Start() {

	port := 50051 //TODO: extract from environment variable

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Panic(err)
	}

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge: 10 * time.Second,
		}),
	}
	app.router.InitPProfRouter()
	grpcServer := grpc.NewServer(opts...)

	client.RegisterApplicationServiceServer(grpcServer, app.ServerImpl)
	app.Logger.Infow("starting server on ", "port", port)

	err = grpcServer.Serve(listener)
	if err != nil {
		app.Logger.Fatalw("failed to listen: %v", "err", err)
	}

}
