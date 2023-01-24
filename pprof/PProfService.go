package pprof

import (
	"net/http"
	"net/http/pprof"
)

func Index(w http.ResponseWriter, r *http.Request) {
	pprof.Index(w, r)
}

func Cmdline(w http.ResponseWriter, r *http.Request) {
	pprof.Cmdline(w, r)
}

func Profile(w http.ResponseWriter, r *http.Request) {
	pprof.Profile(w, r)
}

func Symbol(w http.ResponseWriter, r *http.Request) {
	pprof.Symbol(w, r)
}

func Trace(w http.ResponseWriter, r *http.Request) {
	pprof.Trace(w, r)
}

func Goroutine(w http.ResponseWriter, r *http.Request) {
	pprof.Handler("goroutine").ServeHTTP(w, r)
}

func Threadcreate(w http.ResponseWriter, r *http.Request) {
	pprof.Handler("threadcreate").ServeHTTP(w, r)
}

func Heap(w http.ResponseWriter, r *http.Request) {
	pprof.Handler("heap").ServeHTTP(w, r)
}

func Block(w http.ResponseWriter, r *http.Request) {
	pprof.Handler("block").ServeHTTP(w, r)
}

func Mutex(w http.ResponseWriter, r *http.Request) {
	pprof.Handler("mutex").ServeHTTP(w, r)
}

func Allocs(w http.ResponseWriter, r *http.Request) {
	pprof.Handler("allocs").ServeHTTP(w, r)
}
