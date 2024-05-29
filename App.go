package main

import (
	"context"
	"fmt"
	"github.com/devtron-labs/common-lib/constants"
	"github.com/devtron-labs/common-lib/middlewares"
	"github.com/devtron-labs/common-lib/pubsub-lib/metrics"
	"github.com/devtron-labs/kubelink/api/router"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/internals/middleware"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/service"
	"github.com/go-pg/pg"
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
	Logger      *zap.SugaredLogger
	ServerImpl  *service.ApplicationServiceServerImpl
	router      *router.RouterImpl
	k8sInformer k8sInformer.K8sInformer
	db          *pg.DB
	server      *http.Server
	grpcServer  *grpc.Server
	fluxApp     *service.FluxApplicationServiceImpl
}

func NewApp(Logger *zap.SugaredLogger, ServerImpl *service.ApplicationServiceServerImpl,
	router *router.RouterImpl, k8sInformer k8sInformer.K8sInformer, db *pg.DB, fluxApp *service.FluxApplicationServiceImpl) *App {
	return &App{
		Logger:      Logger,
		ServerImpl:  ServerImpl,
		router:      router,
		k8sInformer: k8sInformer,
		db:          db,
		fluxApp:     fluxApp,
	}
}

func (app *App) Start() {

	port := 50051 //TODO: extract from environment variable

	httpPort := 50052

	numbers := []int{1}
	res2, _ := app.fluxApp.ListApplications(numbers)
	fmt.Println(res2)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Panic(err)
	}

	grpcPanicRecoveryHandler := func(p any) (err error) {
		metrics.IncPanicRecoveryCount("grpc", "", "", "")
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
			recovery.StreamServerInterceptor(recoveryOption)), // panic interceptor, should be at last
		grpc.ChainUnaryInterceptor(
			grpc_prometheus.UnaryServerInterceptor,
			recovery.UnaryServerInterceptor(recoveryOption)), // panic interceptor, should be at last
	}
	app.router.Router.Use(middleware.PrometheusMiddleware)
	app.router.InitRouter()
	app.grpcServer = grpc.NewServer(opts...)
	client.RegisterApplicationServiceServer(app.grpcServer, app.ServerImpl)
	grpc_prometheus.EnableHandlingTimeHistogram()
	grpc_prometheus.Register(app.grpcServer)
	go func() {
		app.server = &http.Server{Addr: fmt.Sprintf(":%d", httpPort), Handler: app.router.Router}
		app.router.Router.Use(middlewares.Recovery)
		err := app.server.ListenAndServe()
		if err != nil {
			log.Fatal("error in starting http server", err)
		}
	}()
	app.Logger.Infow("starting server on ", "port", port)

	err = app.grpcServer.Serve(listener)
	if err != nil {
		app.Logger.Fatalw("failed to listen: %v", "err", err)
	}

}

func (app *App) Stop() {

	app.Logger.Infow("kubelink shutdown initiating")

	timeoutContext, _ := context.WithTimeout(context.Background(), 5*time.Second)
	app.Logger.Infow("closing router")
	err := app.server.Shutdown(timeoutContext)
	if err != nil {
		app.Logger.Errorw("error in mux router shutdown", "err", err)
	}

	// Gracefully stop the gRPC server
	app.Logger.Info("Stopping gRPC server...")
	app.grpcServer.GracefulStop()

	app.Logger.Infow("closing db connection")
	err = app.db.Close()
	if err != nil {
		app.Logger.Errorw("error in closing db connection", "err", err)
	}

	app.Logger.Infow("housekeeping done. exiting now")
}
