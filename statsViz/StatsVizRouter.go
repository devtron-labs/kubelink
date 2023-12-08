package statsViz

import (
	"github.com/arl/statsviz"
	"github.com/caarlos0/env"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type StatsVizRouter interface {
	InitStatsVizRouter(router *mux.Router)
}

type StatVizConfig struct {
	EnableStatsviz bool `env:"ENABLE_STATSVIZ" envDefault:"false"`
}

func GetStatsVizConfig() (*StatVizConfig, error) {
	cfg := &StatVizConfig{}
	err := env.Parse(cfg)
	return cfg, err
}

type StatsVizRouterImpl struct {
	logger *zap.SugaredLogger
	config *StatVizConfig
}

func NewStatsVizRouter(logger *zap.SugaredLogger, config *StatVizConfig) *StatsVizRouterImpl {
	return &StatsVizRouterImpl{
		logger: logger,
		config: config,
	}
}

func (r *StatsVizRouterImpl) InitStatsVizRouter(router *mux.Router) {
	if !r.config.EnableStatsviz {
		return
	}
	stvServer, _ := statsviz.NewServer()
	router.Path("/debug/statsviz/ws").Name("GET /debug/statsviz/ws").HandlerFunc(stvServer.Ws())
	router.PathPrefix("/debug/statsviz/").Name("GET /debug/statsviz/").Handler(stvServer.Index())
}
