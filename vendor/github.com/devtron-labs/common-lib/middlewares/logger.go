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
		finalReq := extractRequestFromFields(fields, removeFields)
		index := getIndex(fields, MethodFieldKey)
		if index == -1 {
			return
		}
		message := fmt.Sprintf("AUDIT_LOG: requestMethod: %s, requestPayload: %s", fields[index+1], finalReq)
		lg.Info(message)
	})
}

func getIndex(fields []any, fieldKey any) int {
	index := slices.Index(fields, fieldKey)
	if index == len(fields) {
		return -1
	}
	return index
}
func extractRequestFromFields(fields []any, removeFields []string) []byte {
	index := getIndex(fields, RequestFieldKey)
	if index == -1 {
		return []byte{}
	}
	req := make(map[string]interface{})
	marshalReq, _ := json.Marshal(fields[index+1])
	if len(removeFields) == 0 {
		return marshalReq
	}
	json.Unmarshal(marshalReq, &req)
	for _, field := range removeFields {
		delete(req, field)
	}
	finalReq, _ := json.Marshal(req)
	return finalReq
}
