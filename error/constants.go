package error

import (
	"google.golang.org/grpc/codes"
)

// list of error strings from Helm. These are part of the errors we check for presence in Helm's error messages.
const (
	YAMLToJSONConversionError   = "error converting YAML to JSON"
	ClusterUnreachableErrorMsg  = "cluster unreachable"
	CrdPreconditionErrorMsg     = "ensure CRDs are installed first"
	ArrayStringMismatchErrorMsg = "got array expected string"
	NotFoundErrorMsg            = "not found" //this is a generic type constant, an error could be namespace "ns1" not found or service "ser1" not found.
	InvalidValueErrorMsg        = "Invalid value"
	OperationInProgressErrorMsg = "another operation (install/upgrade/rollback) is in progress"
	ForbiddenErrorMsg           = "forbidden"
)

// list of internal errors, these errors are easy for the users to understand
const (
	InternalClusterUnreachableErrorMsg  = "cluster unreachable"
	InternalOperationInProgressErrorMsg = "another operation (install/upgrade/rollback) is in progress"
)

type errorGrpcCodeTuple struct {
	errorMsg string
	grpcCode codes.Code
}

var helmErrorInternalErrorMap = map[string]errorGrpcCodeTuple{
	ClusterUnreachableErrorMsg:  {errorMsg: InternalClusterUnreachableErrorMsg, grpcCode: codes.DeadlineExceeded},
	OperationInProgressErrorMsg: {errorMsg: InternalOperationInProgressErrorMsg, grpcCode: codes.FailedPrecondition},
}

var DynamicErrorMapping = map[string]codes.Code{
	NotFoundErrorMsg:            codes.NotFound,
	ForbiddenErrorMsg:           codes.PermissionDenied,
	InvalidValueErrorMsg:        codes.InvalidArgument,
	ArrayStringMismatchErrorMsg: codes.InvalidArgument,
	CrdPreconditionErrorMsg:     codes.FailedPrecondition,
}
