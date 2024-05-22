package error

import (
	"strings"
)

func IsValidationError(err error) bool {
	//validation errors from k8s
	ok := strings.Contains(err.Error(), YAMLToJSONConversionError)
	return ok
}
