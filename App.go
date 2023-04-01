package main

import (
	"fmt"
	"github.com/devtron-labs/kubelink/api/router"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/service"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"log"
	"net"
	"net/http"
	"time"
)

type App struct {
	Logger      *zap.SugaredLogger
	ServerImpl  *service.ApplicationServiceServerImpl
	router      *router.RouterImpl
	k8sInformer k8sInformer.K8sInformer
}

func NewApp(Logger *zap.SugaredLogger, ServerImpl *service.ApplicationServiceServerImpl,
	router *router.RouterImpl, k8sInformer k8sInformer.K8sInformer) *App {
	return &App{
		Logger:      Logger,
		ServerImpl:  ServerImpl,
		router:      router,
		k8sInformer: k8sInformer,
	}
}

func (app *App) Start() {

	port := 50051 //TODO: extract from environment variable

	httpPort := 50052

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Panic(err)
	}

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge: 10 * time.Second,
		}),
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	}
	app.router.InitRouter()
	grpcServer := grpc.NewServer(opts...)

	client.RegisterApplicationServiceServer(grpcServer, app.ServerImpl)
	grpc_prometheus.EnableHandlingTimeHistogram()
	grpc_prometheus.Register(grpcServer)
	go func() {
		server := &http.Server{Addr: fmt.Sprintf(":%d", httpPort), Handler: app.router.Router}
		err := server.ListenAndServe()
		if err != nil {
			log.Fatal("error in starting http server", err)
		}
	}()
	app.Logger.Infow("starting server on ", "port", port)

	err = grpcServer.Serve(listener)
	if err != nil {
		app.Logger.Fatalw("failed to listen: %v", "err", err)
	}

}
