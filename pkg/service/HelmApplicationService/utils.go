package HelmApplicationService

import (
	client "github.com/devtron-labs/kubelink/grpc"
	"strconv"
)

func getUniqueReleaseIdentifierName(releaseIdentifier *client.ReleaseIdentifier) string {
	return releaseIdentifier.ReleaseNamespace + "_" + releaseIdentifier.ReleaseName + "_" + strconv.Itoa(int(releaseIdentifier.ClusterConfig.ClusterId))
}
