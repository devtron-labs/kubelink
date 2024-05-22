package error

import (
	"fmt"
	"github.com/devtron-labs/common-lib/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
)

// ConvertHelmErrorToInternalError converts known error message from helm to internal error and also maps it with proper grpc code
func ConvertHelmErrorToInternalError(err error) error {
	err = GetGrpcErrorWithCustomHelmErrorCode(err)
	genericError := getInternalErrorForGenericErrorTypes(err)
	if genericError != nil {
		return genericError
	}
	for helmErrMsg, internalErr := range helmErrorInternalErrorMap {
		if strings.Contains(err.Error(), helmErrMsg) {
			err = status.New(internalErr.grpcCode, internalErr.errorMsg).Err()
		}
	}
	return err
}

// GetGrpcErrorWithCustomHelmErrorCode returns error type with error initialized via grpc's status.New constructor with HelmErrorGrpcCode custom grpc error code
func GetGrpcErrorWithCustomHelmErrorCode(err error) error {
	return status.New(codes.Code(HelmErrorGrpcCode), err.Error()).Err()
}

// GetGrpcErrorWithCustomInternalErrorCode returns error type with error initialized via grpc's status.New constructor with InternalErrorGrpcCode custom grpc error code
func GetGrpcErrorWithCustomInternalErrorCode(err error) error {
	return status.New(codes.Code(InternalErrorGrpcCode), err.Error()).Err()
}

// getInternalErrorForGenericErrorTypes returns all those kinds of errors which are generic in nature and also dynamic, make sure to return all generic and dynamic errors from this func. instead of putting them in helmErrorInternalErrorMap
func getInternalErrorForGenericErrorTypes(err error) error {
	/*
		for example:-
			1. if namespace is not found err is:- namespace "ns1" not found,
			2. in case ingress class not found error is of type ingress class: IngressClass.networking.k8s.io "ingress1" not found,
			3. when some resource is forbidden then err can be of many formats one of which is:- Unable to continue with install: could not get information about the resource Ingress "prakash-1-prakash-env3-ingress" in namespace "prakash-ns3": ingresses.networking.k8s.io "prakash-1-prakash-env3-ingress" is forbidden...
			etc..
	*/
	for errorMsg, code := range DynamicErrorMapping {
		if strings.Contains(strings.ToLower(err.Error()), errorMsg) {
			return status.New(code, err.Error()).Err()
		}
	}

	return nil
}

func HandleError(err error) {
	switch e := err.(type) {
	//make this default
	case *utils.InternalError:
		fmt.Printf("Handled internal error: %s\n", e.Err.Error())
		// Further handling based on gRPC status
		//grpcError := e.GRPCError()

	case *utils.HelmError:
		fmt.Printf("Handled Helm error: %s\n", e.Err.Error())
		// Further handling based on gRPC status
		//grpcError := e.GRPCError()

	default:
		st, ok := status.FromError(err)
		if ok {
			fmt.Printf("Unhandled gRPC error: %s\n", st.Message())
		} else {
			fmt.Printf("Unhandled error: %s\n", err.Error())
		}
	}
}

// CustomCode type that includes standard gRPC codes and custom codes
//type CustomCode codes.Code

const (
	InternalErrorGrpcCode = 100 + iota // Start custom codes from 100 to avoid conflicts
	HelmErrorGrpcCode
)

// InternalError represents an internal error with a gRPC status code.
type InternalError struct {
	err  error
	Code codes.Code
}

func (e *InternalError) Error() string {
	return fmt.Sprintf("Internal Error: %s", e.err.Error())
}

func (e *InternalError) GRPCError() error {
	return status.New(e.Code, e.err.Error()).Err()
}

// HelmError represents an error from Helm with a gRPC status code.
type HelmError struct {
	err  error
	Code codes.Code
}

func (e *HelmError) Error() string {
	return fmt.Sprintf("Helm Error: %s", e.err.Error())
}

func (e *HelmError) GRPCError() error {
	return status.New(e.Code, e.err.Error()).Err()
}
