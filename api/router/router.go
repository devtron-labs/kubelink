package router

import (
	"github.com/devtron-labs/kubelink/pprof"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

type RouterImpl struct {
	logger      *zap.SugaredLogger
	Router      *mux.Router
	pprofRouter pprof.PProfRouter
}

func NewRouter(logger *zap.SugaredLogger,
	pprofRouter pprof.PProfRouter) *RouterImpl {
	return &RouterImpl{
		logger:      logger,
		Router:      mux.NewRouter(),
		pprofRouter: pprofRouter,
	}
}

func (r *RouterImpl) InitRouter() {
	pProfListenerRouter := r.Router.PathPrefix("/kubelink/debug/pprof/").Subrouter()
	r.pprofRouter.InitPProfRouter(pProfListenerRouter)
	r.Router.PathPrefix("/kubelink/metrics").Handler(promhttp.Handler())
}
