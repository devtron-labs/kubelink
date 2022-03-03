package service

import (
	"fmt"
	"github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/helmClient"
	k8sUtils "github.com/devtron-labs/kubelink/pkg/util/k8s"
	"google.golang.org/protobuf/types/known/timestamppb"
	"helm.sh/helm/v3/pkg/release"
)

type AppService struct {
}

func (impl *AppService) GetApplicationListForCluster(config *client.ClusterConfig) *client.DeployedAppList {
	deployedApp := &client.DeployedAppList{ClusterId: config.GetClusterId()}
	restConfig, err := k8sUtils.GetRestConfig(config)
	if err != nil {
		deployedApp.Errored = true
		deployedApp.ErrorMsg = err.Error()
		return deployedApp
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
		},
		RestConfig: restConfig,
	}

	helmAppClient, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		deployedApp.Errored = true
		deployedApp.ErrorMsg = err.Error()
		return deployedApp
	}
	releases, err := helmAppClient.ListAllReleases()
	if err != nil {
		deployedApp.Errored = true
		deployedApp.ErrorMsg = err.Error()
		return deployedApp
	}

	var deployedApps []*client.DeployedAppDetail
	for _, items := range releases {
		appDetail := &client.DeployedAppDetail{
			AppId:        GatAppId(config.ClusterId, items),
			AppName:      items.Name,
			ChartName:    items.Chart.Name(),
			ChartAvatar:  items.Chart.Metadata.Icon,
			LastDeployed: timestamppb.New(items.Info.LastDeployed.Time),
			EnvironmentDetail: &client.EnvironmentDetails{
				ClusterName: config.ClusterName,
				ClusterId:   config.ClusterId,
				Namespace:   items.Namespace,
			},
		}
		deployedApps = append(deployedApps, appDetail)
	}
	deployedApp.DeployedAppDetail = deployedApps
	return deployedApp
}

// GatAppId returns AppID by logic  cluster_id|namespace|release_name
func GatAppId(clusterId int32, release *release.Release) string {
	return fmt.Sprintf("%d|%s|%s", clusterId, release.Namespace, release.Name)
}
