/*
 * Copyright (c) 2024. Devtron Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package middleware

import (
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"net/http"
	"strconv"
	"time"
)

// metrics name constant
const (
	KUBELINK_HTTP_DURATION_SECONDS = "kubelink_http_duration_seconds"
	KUBELINK_HTTP_REQUESTS_TOTAL   = "kubelink_http_requests_total"
	KUBELINK_HTTP_REQUESTS_CURRENT = "kubelink_http_requests_current"
)

// metrics labels constants
const (
	PATH   = "path"
	METHOD = "method"
	STATUS = "status"
)

var (
	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: KUBELINK_HTTP_DURATION_SECONDS,
		Help: "Duration of HTTP requests.",
	}, []string{PATH, METHOD, STATUS})
)
var requestCounter = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: KUBELINK_HTTP_REQUESTS_TOTAL,
		Help: "How many HTTP requests processed, partitioned by status code, method and HTTP path.",
	},
	[]string{PATH, METHOD, STATUS})

var currentRequestGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: KUBELINK_HTTP_REQUESTS_CURRENT,
	Help: "no of request being served currently",
}, []string{PATH, METHOD})

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
