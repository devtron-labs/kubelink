package main

import (
	"fmt"
	"github.com/caarlos0/env"
	"github.com/devtron-labs/common-lib/constants"
	"github.com/devtron-labs/common-lib/middlewares"
	"github.com/devtron-labs/kubelink/api/router"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/service"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"time"
)

type App struct {
	Logger       *zap.SugaredLogger
	ServerImpl   *service.ApplicationServiceServerImpl
	router       *router.RouterImpl
	k8sInformer  k8sInformer.K8sInformer
	LoggerConfig *LoggerConfig
}

func NewApp(Logger *zap.SugaredLogger, ServerImpl *service.ApplicationServiceServerImpl,
	router *router.RouterImpl, k8sInformer k8sInformer.K8sInformer) (*App, error) {
	app := &App{
		Logger:      Logger,
		ServerImpl:  ServerImpl,
		router:      router,
		k8sInformer: k8sInformer,
	}
	cfg, err := GetLoggerConfig()
	if err != nil {
		return nil, err
	}
	app.LoggerConfig = cfg
	return app, err
}

func (app *App) Start() {

	port := 50051 //TODO: extract from environment variable

	httpPort := 50052

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Panic(err)
	}

	grpcPanicRecoveryHandler := func(p any) (err error) {
		app.Logger.Error(constants.PanicLogIdentifier, "recovered from panic", "panic", p, "stack", string(debug.Stack()))
		return status.Errorf(codes.Internal, "%s", p)
	}
	recoveryOption := recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)
	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge: 10 * time.Second,
		}),
		grpc.ChainStreamInterceptor(
			grpc_prometheus.StreamServerInterceptor,
			logging.StreamServerInterceptor(middlewares.InterceptorLogger(app.LoggerConfig.EnableLogger, app.Logger),
				logging.WithLogOnEvents(logging.PayloadReceived)),
			recovery.StreamServerInterceptor(recoveryOption)), // panic interceptor, should be at last
		grpc.ChainUnaryInterceptor(
			grpc_prometheus.UnaryServerInterceptor,
			logging.UnaryServerInterceptor(middlewares.InterceptorLogger(app.LoggerConfig.EnableLogger, app.Logger),
				logging.WithLogOnEvents(logging.PayloadReceived)),
			recovery.UnaryServerInterceptor(recoveryOption)), // panic interceptor, should be at last
	}
	app.router.InitRouter()
	grpcServer := grpc.NewServer(opts...)

	client.RegisterApplicationServiceServer(grpcServer, app.ServerImpl)
	grpc_prometheus.EnableHandlingTimeHistogram()
	grpc_prometheus.Register(grpcServer)
	go func() {
		server := &http.Server{Addr: fmt.Sprintf(":%d", httpPort), Handler: app.router.Router}
		app.router.Router.Use(middlewares.Recovery)
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

type LoggerConfig struct {
	EnableLogger bool `env:"FEATURE_LOGGER_MIDDLEWARE_ENABLE" envDefault:"false"`
}

func GetLoggerConfig() (*LoggerConfig, error) {
	cfg := &LoggerConfig{}
	err := env.Parse(cfg)
	return cfg, err
}
