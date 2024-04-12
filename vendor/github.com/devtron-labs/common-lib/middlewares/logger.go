package middlewares

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
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
		finalReq := extractedFields(fields)
		index := slices.Index(fields, MethodFieldKey)
		message := fmt.Sprintf("AUDIT_LOG: requestMethod: %s, requestPayload: %s", fields[index+1], finalReq)
		lg.Info(message)
		fmt.Println("hello")
	})
}
func extractedFields(fields []any) []byte {
	index := slices.Index(fields, RequestFieldKey)
	req := make(map[string]interface{})
	marshal, _ := json.Marshal(fields[index+1])
	err := json.Unmarshal(marshal, &req)
	if err != nil {
		return nil
	}
	removedFields := []string{"RunInCtx", "chartContent", "valuesYaml"}
	for _, field := range removedFields {
		delete(req, field)
	}
	finalReq, _ := json.Marshal(req)
	return finalReq
}
func GenerateLogFields(ctx context.Context, meta interceptors.CallMeta) logging.Fields {
	fields := logging.Fields{logging.MethodFieldKey, meta.Method}
	ctx = logging.InjectFields(ctx, fields)
	return fields
}
