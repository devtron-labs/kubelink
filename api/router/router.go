package router

import (
	"encoding/json"
	"github.com/arl/statsviz"
	"github.com/devtron-labs/kubelink/pprof"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"net/http"
)

type RouterImpl struct {
	logger         *zap.SugaredLogger
	Router         *mux.Router
	pprofRouter    pprof.PProfRouter
	statsVizServer *statsviz.Server
	//statsVizRouter *statsViz.StatsVizRouter
}

func NewRouter(logger *zap.SugaredLogger,
	pprofRouter pprof.PProfRouter,
	// statsVizRouter *statsViz.StatsVizRouter,
) *RouterImpl {
	stvServer, _ := statsviz.NewServer()
	return &RouterImpl{
		logger:      logger,
		Router:      mux.NewRouter(),
		pprofRouter: pprofRouter,
		//statsVizRouter: statsVizRouter,
		statsVizServer: stvServer,
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

	//statsVizRouter := r.Router.PathPrefix("/debug/statsViz").Subrouter()
	//r.statsVizRouter.Init
	r.Router.Methods("GET").Path("/debug/statsviz/ws").Name("GET /debug/statsviz/ws").HandlerFunc(r.statsVizServer.Ws())
	r.Router.Methods("GET").PathPrefix("/debug/statsviz/").Name("GET /debug/statsviz/").Handler(r.statsVizServer.Index())
	//r.Router.Methods("GET").Path().HandleFunc("/debug/statsviz", r.statsVizServer.Index())
	//r.Router.HandleFunc("/debug/statsviz/ws", r.statsVizServer.Ws())

	r.Router.PathPrefix("/kubelink/metrics").Handler(promhttp.Handler())
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
