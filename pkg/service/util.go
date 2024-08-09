package service

import (
	"errors"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
)

func IsReleaseNotFoundInCacheError(err error) bool {
	return errors.Is(err, k8sInformer.ErrorCacheMissReleaseNotFound)
}
