package middleware

import (
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"net/http"
	"strconv"
	"time"
)

var (
	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "kubelink_http_duration_seconds",
		Help: "Duration of HTTP requests.",
	}, []string{"path", "method", "status"})
)
var requestCounter = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "kubelink_http_requests_total",
		Help: "How many HTTP requests processed, partitioned by status code, method and HTTP path.",
	},
	[]string{"path", "method", "status"})

var currentRequestGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "kubelink_http_requests_current",
	Help: "no of request being served currently",
}, []string{"path", "method"})

// prometheusMiddleware implements mux.MiddlewareFunc.
func PrometheusMiddleware(next http.Handler) http.Handler {
	//	prometheus.MustRegister(requestCounter)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		route := mux.CurrentRoute(r)
		path, _ := route.GetPathTemplate()
		method := r.Method
		g := currentRequestGauge.WithLabelValues(path, method)
		g.Inc()
		defer g.Dec()
		d := NewDelegator(w, nil)
		next.ServeHTTP(d, r)
		httpDuration.WithLabelValues(path, method, strconv.Itoa(d.Status())).Observe(time.Since(start).Seconds())
		requestCounter.WithLabelValues(path, method, strconv.Itoa(d.Status())).Inc()
	})
}
