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
	helmErrorInternalErrorMap := map[string]map[string]codes.Code{
		ClusterUnreachableErrorMsg:  {InternalClusterUnreachableErrorMsg: codes.DeadlineExceeded},
		CrdPreconditionErrorMsg:     {InternalCrdPreconditionErrorMsg: codes.FailedPrecondition},
		NamespaceNotFoundErrorMsg:   {InternalNamespaceNotFoundErrorMsg: codes.Unknown},
		ArrayStringMismatchErrorMsg: {InternalArrayStringMismatchErrorMsg: codes.Unknown},
		InvalidValueErrorMsg:        {InternalInvalidValueErrorMsg: codes.Unknown},
		OperationInProgressErrorMsg: {InternalOperationInProgressErrorMsg: codes.FailedPrecondition},
	}
	for helmErrMsg, internalErr := range helmErrorInternalErrorMap {
		if strings.Contains(err.Error(), helmErrMsg) {
			for internalErrMsg, internalErrCode := range internalErr {
				return status.New(internalErrCode, internalErrMsg).Err()
			}
		}
	}
	return nil
}
