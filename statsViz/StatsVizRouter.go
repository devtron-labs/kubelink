package statsViz

import (
	"github.com/arl/statsviz"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type StatsVizRouter interface {
	InitStatsVizRouter(router *mux.Router)
}

type StatsVizRouterImpl struct {
	logger         *zap.SugaredLogger
	statsVizServer *statsviz.Server
}

func NewStatsVizRouter(logger *zap.SugaredLogger) *StatsVizRouterImpl {
	stvServer, _ := statsviz.NewServer()
	return &StatsVizRouterImpl{
		logger:         logger,
		statsVizServer: stvServer,
	}
}

func (r *StatsVizRouterImpl) InitStatsVizRouter(router *mux.Router) {
	router.Path("/debug/statsviz/ws").Name("GET /debug/statsviz/ws").HandlerFunc(r.statsVizServer.Ws())
	router.PathPrefix("/debug/statsviz/").Name("GET /debug/statsviz/").Handler(r.statsVizServer.Index())
}
