package router

import (
	"encoding/json"
	"github.com/devtron-labs/kubelink/pprof"
	"github.com/devtron-labs/kubelink/statsViz"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"net/http"
)

type RouterImpl struct {
	logger         *zap.SugaredLogger
	Router         *mux.Router
	pprofRouter    pprof.PProfRouter
	statsVizRouter statsViz.StatsVizRouter
}

func NewRouter(logger *zap.SugaredLogger,
	pprofRouter pprof.PProfRouter,
	statsVizRouter statsViz.StatsVizRouter,
) *RouterImpl {
	return &RouterImpl{
		logger:         logger,
		Router:         mux.NewRouter(),
		pprofRouter:    pprofRouter,
		statsVizRouter: statsVizRouter,
	}
}

type Response struct {
	Code   int         `json:"code,omitempty"`
	Status string      `json:"status,omitempty"`
	Result interface{} `json:"result,omitempty"`
}

func (r *RouterImpl) InitRouter() {
	pProfListenerRouter := r.Router.PathPrefix("/kubelink/debug/pprof/").Subrouter()
	r.pprofRouter.InitPProfRouter(pProfListenerRouter)

	statsVizRouter := r.Router.Methods("GET").Subrouter()
	r.statsVizRouter.InitStatsVizRouter(statsVizRouter)

	r.Router.PathPrefix("/metrics").Handler(promhttp.Handler())
	r.Router.Path("/health").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(200)
		response := Response{}
		response.Code = 200
		response.Result = "OK"
		b, err := json.Marshal(response)
		if err != nil {
			b = []byte("OK")
			r.logger.Errorw("Unexpected error in apiError", "err", err)
		}
		_, _ = writer.Write(b)
	})
}
