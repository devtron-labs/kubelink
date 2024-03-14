package error

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
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
