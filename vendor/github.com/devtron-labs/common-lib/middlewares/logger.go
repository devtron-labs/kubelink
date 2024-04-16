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

// InterceptorLogger adapts go-kit logger to interceptor logger.
func InterceptorLogger(enableLogger bool, removeFields []string, lg *zap.SugaredLogger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		if !enableLogger {
			return
		}
		req := extractFromFields(fields, RequestFieldKey)
		finalReq := extractRequestAfterRemovingFields(req, removeFields)
		methodName := extractFromFields(fields, MethodFieldKey)
		message := fmt.Sprintf("AUDIT_LOG: requestMethod: %s, requestPayload: %s", methodName, finalReq)
		lg.Info(message)
	})
}

func getIndex(fields []any, fieldKey any) int {
	return slices.Index(fields, fieldKey)
}
func extractFromFields(fields []any, key string) []byte {
	index := getIndex(fields, key)
	if index == -1 || index == len(fields) {
		return []byte{}
	}
	marshalField, _ := json.Marshal(fields[index+1])
	return marshalField
}
func extractRequestAfterRemovingFields(marshalReq []byte, removeFields []string) []byte {
	if len(removeFields) == 0 {
		return marshalReq
	}
	req := make(map[string]interface{})
	json.Unmarshal(marshalReq, &req)
	for _, field := range removeFields {
		delete(req, field)
	}
	finalReq, _ := json.Marshal(req)
	return finalReq
}
