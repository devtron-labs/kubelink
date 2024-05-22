package utils

import (
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ApiError struct {
	HttpStatusCode    int         `json:"-"`
	Code              string      `json:"code,omitempty"`
	InternalMessage   string      `json:"internalMessage,omitempty"`
	UserMessage       interface{} `json:"userMessage,omitempty"`
	UserDetailMessage string      `json:"userDetailMessage,omitempty"`
}

func (e *ApiError) Error() string {
	return e.InternalMessage
}

// default internal will be set
func (e *ApiError) ErrorfInternal(format string, a ...interface{}) error {
	return &ApiError{InternalMessage: fmt.Sprintf(format, a...)}
}

// default user message will be set
func (e ApiError) ErrorfUser(format string, a ...interface{}) error {
	return &ApiError{InternalMessage: fmt.Sprintf(format, a...)}
}

// InternalError represents an internal error with a gRPC status code.
type InternalError struct {
	Err  error
	Code codes.Code
}

func (e *InternalError) Error() string {
	return fmt.Sprintf("Internal Error: %s", e.Err.Error())
}

func (e *InternalError) GRPCError() error {
	return status.New(e.Code, e.Err.Error()).Err()
}

// HelmError represents an error from Helm with a gRPC status code.
type HelmError struct {
	Err  error
	Code codes.Code
}

func (e *HelmError) Error() string {
	return fmt.Sprintf("Helm Error: %s", e.Err.Error())
}

func (e *HelmError) GRPCError() error {
	return status.New(e.Code, e.Err.Error()).Err()
}
