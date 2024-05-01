package error

import "google.golang.org/grpc/codes"

// list of errors from helm
const (
	YAMLToJSONConversionError   = "error converting YAML to JSON"
	ClusterUnreachableErrorMsg  = "cluster unreachable"
	CrdPreconditionErrorMsg     = "ensure CRDs are installed first"
	ArrayStringMismatchErrorMsg = "got array expected string"
	//NamespaceNotFoundErrorMsg   = "not found" // todo - add more acurate error message, this is very generic
	InvalidValueErrorMsg        = "Invalid value"
	OperationInProgressErrorMsg = "another operation (install/upgrade/rollback) is in progress"
)

// list of internal errors
const (
	InternalClusterUnreachableErrorMsg  = "cluster unreachable"
	InternalCrdPreconditionErrorMsg     = "ensure CRDs are installed first"
	InternalArrayStringMismatchErrorMsg = "got array expected string"
	InternalNamespaceNotFoundErrorMsg   = "namespace not found"
	InternalInvalidValueErrorMsg        = "invalid value in manifest"
	InternalOperationInProgressErrorMsg = "another operation (install/upgrade/rollback) is in progress"
)

var helmErrorInternalErrorMap = map[string]map[string]codes.Code{
	ClusterUnreachableErrorMsg: {InternalClusterUnreachableErrorMsg: codes.DeadlineExceeded},
	CrdPreconditionErrorMsg:    {InternalCrdPreconditionErrorMsg: codes.FailedPrecondition},
	//NamespaceNotFoundErrorMsg:   {InternalNamespaceNotFoundErrorMsg: codes.Unknown},
	ArrayStringMismatchErrorMsg: {InternalArrayStringMismatchErrorMsg: codes.Unknown},
	InvalidValueErrorMsg:        {InternalInvalidValueErrorMsg: codes.Unknown},
	OperationInProgressErrorMsg: {InternalOperationInProgressErrorMsg: codes.FailedPrecondition},
}
