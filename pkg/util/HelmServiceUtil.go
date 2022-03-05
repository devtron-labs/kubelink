package util

import (
	"fmt"
	"helm.sh/helm/v3/pkg/release"
)

// GetAppId returns AppID by logic  cluster_id|namespace|release_name
func GetAppId(clusterId int32, release *release.Release) string {
	return fmt.Sprintf("%d|%s|%s", clusterId, release.Namespace, release.Name)
}
