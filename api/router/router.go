package router

import (
	"encoding/json"

	"github.com/devtron-labs/common-lib/monitoring"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"net/http"
)

type RouterImpl struct {
	logger           *zap.SugaredLogger
	Router           *mux.Router
	monitoringRouter *monitoring.MonitoringRouter
}

func NewRouter(logger *zap.SugaredLogger, monitoringRouter *monitoring.MonitoringRouter) *RouterImpl {
	return &RouterImpl{
		logger:           logger,
		Router:           mux.NewRouter(),
		monitoringRouter: monitoringRouter,
	}
}

type Response struct {
	Code   int         `json:"code,omitempty"`
	Status string      `json:"status,omitempty"`
	Result interface{} `json:"result,omitempty"`
}

func (r *RouterImpl) InitRouter() {
	pProfListenerRouter := r.Router.PathPrefix("/kubelink/debug/pprof/").Subrouter()
	statsVizRouter := r.Router.PathPrefix("/kubelink").Subrouter()

	r.monitoringRouter.InitMonitoringRouter(pProfListenerRouter, statsVizRouter, "/kubelink")

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
