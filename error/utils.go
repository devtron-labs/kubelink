package error

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
)

// ConvertHelmErrorToInternalError converts known error message from helm to internal error and also maps it with proper grpc code
func ConvertHelmErrorToInternalError(err error) error {
	genericError := getInternalErrorForGenericErrorTypes(err)
	if genericError != nil {
		return genericError
	}
	var internalError error
	for helmErrMsg, internalErr := range helmErrorInternalErrorMap {
		if strings.Contains(err.Error(), helmErrMsg) {
			for internalErrMsg, internalErrCode := range internalErr {
				internalError = status.New(internalErrCode, internalErrMsg).Err()
			}
		}
	}
	return internalError
}

// getInternalErrorForGenericErrorTypes returns all those kinds of errors which are generic in nature and also dynamic, make sure to return all generic and dynamic errors from this func. instead of putting them in helmErrorInternalErrorMap
func getInternalErrorForGenericErrorTypes(err error) error {
	/*
		for example:-
			1. if namespace is not found err is:- namespace "ns1" not found,
			2. in case ingress class not found error is of type ingress class: IngressClass.networking.k8s.io "ingress1" not found,
			3. when some resource is forbidden then err can be of many formats one of which is:- Unable to continue with install: could not get information about the resource Ingress "prakash-1-prakash-env3-ingress" in namespace "prakash-ns3": ingresses.networking.k8s.io "prakash-1-prakash-env3-ingress" is forbidden...
	*/
	var internalError error
	if strings.Compare(strings.ToLower(err.Error()), NotFoundErrorMsg) == 0 {
		internalError = status.New(codes.NotFound, err.Error()).Err()
	} else if strings.Contains(strings.ToLower(err.Error()), ForbiddenErrorMsg) {
		internalError = status.New(codes.PermissionDenied, err.Error()).Err()
	}
	return internalError
}
