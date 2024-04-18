package middlewares

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

const (
	RequestFieldKey = "grpc.request.content"
	MethodFieldKey  = "grpc.method"
)

// InterceptorLogger adapts go-kit logger to interceptor logger.
func InterceptorLogger(enableLogger bool, lg *zap.SugaredLogger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		if !enableLogger {
			return
		}
		reqPayload := extractFromFields(fields, RequestFieldKey)
		reqMethodName := extractFromFields(fields, MethodFieldKey)
		message := fmt.Sprintf("AUDIT_LOG: requestMethod: %s, requestPayload: %s", reqMethodName, reqPayload)
		lg.Info(message)
	})
}

func getIndex(fields []any, fieldKey any) int {
	return slices.Index(fields, fieldKey)
}

func extractFromFields(fields []any, key string) any {
	index := getIndex(fields, key)
	if index == -1 || index == len(fields) {
		return ""
	}
	return fields[index+1]
}
