package pprof

import (
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type PProfRouterImpl struct {
	logger *zap.SugaredLogger
	Router *mux.Router
}

func NewPProfRouter(logger *zap.SugaredLogger) *PProfRouterImpl {
	return &PProfRouterImpl{
		logger: logger,
		Router: mux.NewRouter(),
	}
}

func (r *PProfRouterImpl) InitPProfRouter() {
	r.Router.HandleFunc("/", Index)
	r.Router.HandleFunc("/cmdline", Cmdline)
	r.Router.HandleFunc("/profile", Profile)
	r.Router.HandleFunc("/symbol", Symbol)
	r.Router.HandleFunc("/trace", Trace)
	r.Router.HandleFunc("/goroutine", Goroutine)
	r.Router.HandleFunc("/threadcreate", Threadcreate)
	r.Router.HandleFunc("/heap", Heap)
	r.Router.HandleFunc("/block", Block)
	r.Router.HandleFunc("/mutex", Mutex)
	r.Router.HandleFunc("/allocs", Allocs)
}
