package service

import client "github.com/devtron-labs/kubelink/grpc"

func getUniqueReleaseIdentifierName(releaseIdentifier *client.ReleaseIdentifier) string {
	return releaseIdentifier.ReleaseNamespace + "_" + releaseIdentifier.ReleaseName + "_" + string(releaseIdentifier.ClusterConfig.ClusterId)
}
