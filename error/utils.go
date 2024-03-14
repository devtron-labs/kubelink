package error

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
)

// list of errors from helm
const (
	YAMLToJSONConversionError   = "error converting YAML to JSON"
	ClusterUnreachableErrorMsg  = "cluster unreachable"
	CrdPreconditionErrorMsg     = "ensure CRDs are installed first"
	ArrayStringMismatchErrorMsg = "got array expected string"
	NamespaceNotFoundErrorMsg   = "not found"
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

func ConvertHelmErrorToInternalError(err error) error {
	helmErrorInternalErrorMap := map[string]string{
		ClusterUnreachableErrorMsg:  InternalClusterUnreachableErrorMsg,
		CrdPreconditionErrorMsg:     InternalCrdPreconditionErrorMsg,
		NamespaceNotFoundErrorMsg:   InternalNamespaceNotFoundErrorMsg,
		ArrayStringMismatchErrorMsg: InternalArrayStringMismatchErrorMsg,
		InvalidValueErrorMsg:        InternalInvalidValueErrorMsg,
		OperationInProgressErrorMsg: InternalOperationInProgressErrorMsg,
	}
	for helmErrMsg, internalErrMsg := range helmErrorInternalErrorMap {
		if strings.Contains(err.Error(), helmErrMsg) {
			return status.New(codes.Unknown, internalErrMsg).Err()
		}
	}
	return nil
}
