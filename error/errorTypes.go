package error

import (
	"errors"
	"fmt"
	"k8s.io/kube-openapi/pkg/util/proto/validation"
	"strings"
)

// Externally visible error codes
const (
	CodeBadRequest         = "ERR_BAD_REQUEST"
	CodeForbidden          = "ERR_FORBIDDEN"
	CodeNotFound           = "ERR_NOT_FOUND"
	CodeNotImplemented     = "ERR_NOT_IMPLEMENTED"
	CodeTimeout            = "ERR_TIMEOUT"
	CodeInternal           = "ERR_INTERNAL"
	CodeClusterUnreachable = "ERR_CLUSTER_UNREACHABLE"
	CodeNamespaceNotFound  = "ERR_NAMESPACE_NOT_FOUND"
	CodeUnmarshallingError = "ERR_UNMARSHALLING_ERROR"
	CodeValidating         = "ERR_VALIDATING_K8S_OBJECT"
)

// kubelinkerr is the internal implementation of a kubelink error which wraps the error
type Kubelinkerr struct {
	code    string
	message string
	err     error
}

// Wrap returns an error annotating err with a code and supplied message,
// If code supplied is empty it tries to extract code from err
// If err is nil, Wrap returns nil.
func Wrap(err error, code string, message string) error {
	if err == nil {
		return nil
	}
	if len(message) == 0 {
		message = err.Error()
	}
	if len(code) == 0 {
		// typecast error using errors.As and understand code here and assign a proper code in kubelinkerr struct
		if IsValidationError(err) {
			code = CodeValidating
		} else if IsClusterUnreachableErr(err) {
			code = CodeClusterUnreachable
		}
	}
	err = fmt.Errorf(message+": %w", err)
	return Kubelinkerr{code, message, err}
}

func (e Kubelinkerr) Error() string {
	return errors.New(fmt.Sprintf(e.message+": %w", e.err)).Error()
}

func (e Kubelinkerr) Code() string {
	return e.code
}

func IsValidationError(err error) bool {
	//validation errors from k8s
	ok := strings.Contains(err.Error(), "error converting YAML to JSON") || errors.As(err, &validation.ValidationError{}) || errors.As(err, &validation.InvalidTypeError{}) ||
		errors.As(err, &validation.MissingRequiredFieldError{}) ||
		errors.As(err, &validation.UnknownFieldError{}) || errors.As(err, &validation.InvalidObjectTypeError{})
	return ok
}

func IsClusterUnreachableErr(err error) bool {
	if strings.Contains(err.Error(), "cluster unreachable") {
		return true
	}
	return false
}
