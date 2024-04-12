package middlewares

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

const (
	RequestFieldKey = "grpc.request.content"
	MethodFieldKey  = "grpc.method"
)

type Method string

const (
	RunInCtx     Method = "RunInCtx"
	ChartContent Method = "chartContent"
	ValuesYaml   Method = "valuesYaml"
)

// InterceptorLogger adapts go-kit logger to interceptor logger.
func InterceptorLogger(enableLogger bool, lg *zap.SugaredLogger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		if !enableLogger {
			return
		}
		finalReq := extractRequestFromFields(fields)
		index := getIndex(fields, MethodFieldKey)
		message := fmt.Sprintf("AUDIT_LOG: requestMethod: %s, requestPayload: %s", fields[index+1], finalReq)
		lg.Info(message)
	})
}

func getIndex(fields []any, fieldKey any) int {
	return slices.Index(fields, fieldKey)
}
func extractRequestFromFields(fields []any) []byte {
	index := getIndex(fields, RequestFieldKey)
	req := make(map[string]interface{})
	marshal, _ := json.Marshal(fields[index+1])
	json.Unmarshal(marshal, &req)
	fieldsToRemove := []Method{RunInCtx, ChartContent, ValuesYaml}
	for _, field := range fieldsToRemove {
		delete(req, string(field))
	}
	finalReq, _ := json.Marshal(req)
	return finalReq
}
