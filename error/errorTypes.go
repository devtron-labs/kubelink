package error

import "strings"

func IsValidationError(err error) bool {
	//validation errors from k8s
	ok := strings.Contains(err.Error(), "error converting YAML to JSON")
	return ok
}

func IsClusterUnreachableErr(err error) bool {
	if strings.Contains(err.Error(), "cluster unreachable") {
		return true
	}
	return false
}
