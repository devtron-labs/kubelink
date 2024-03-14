package error

import (
	"google.golang.org/grpc/status"
	"strings"
)

func ConvertHelmErrorToInternalError(err error) error {
	for helmErrMsg, internalErr := range helmErrorInternalErrorMap {
		if strings.Contains(err.Error(), helmErrMsg) {
			for internalErrMsg, internalErrCode := range internalErr {
				return status.New(internalErrCode, internalErrMsg).Err()
			}
		}
	}
	return nil
}
