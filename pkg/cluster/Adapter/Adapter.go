package Adapter

import (
	remoteConnectionBean "github.com/devtron-labs/common-lib/utils/remoteConnection/bean"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	clusterBean "github.com/devtron-labs/kubelink/pkg/cluster/bean"
	"github.com/devtron-labs/kubelink/pkg/remoteConnection"
)

func GetClusterBean(model repository.Cluster) clusterBean.ClusterBean {
	model = *ConvertClusterToNewCluster(&model) // repo model is converted according to new struct
	bean := clusterBean.ClusterBean{}
	bean.Id = model.Id
	bean.ClusterName = model.ClusterName
	bean.ServerUrl = model.ServerUrl
	bean.PrometheusUrl = model.PrometheusEndpoint
	bean.AgentInstallationStage = model.AgentInstallationStage
	bean.Active = model.Active
	bean.Config = model.Config
	bean.K8sVersion = model.K8sVersion
	bean.InsecureSkipTLSVerify = model.InsecureSkipTlsVerify
	bean.IsVirtualCluster = model.IsVirtualCluster
	bean.ErrorInConnecting = model.ErrorInConnecting
	if model.RemoteConnectionConfig != nil && model.RemoteConnectionConfig.ConnectionMethod != remoteConnectionBean.RemoteConnectionMethodDirect {
		bean.RemoteConnectionConfig = &remoteConnectionBean.RemoteConnectionConfigBean{
			RemoteConnectionConfigId: model.RemoteConnectionConfigId,
			ConnectionMethod:         model.RemoteConnectionConfig.ConnectionMethod,
			ProxyConfig: &remoteConnectionBean.ProxyConfig{
				ProxyUrl: model.RemoteConnectionConfig.ProxyUrl,
			},
			SSHTunnelConfig: &remoteConnectionBean.SSHTunnelConfig{
				SSHServerAddress: model.RemoteConnectionConfig.SSHServerAddress,
				SSHUsername:      model.RemoteConnectionConfig.SSHUsername,
				SSHPassword:      model.RemoteConnectionConfig.SSHPassword,
				SSHAuthKey:       model.RemoteConnectionConfig.SSHAuthKey,
			},
		}
	}
	return bean
}

func ConvertClusterToNewCluster(model *repository.Cluster) *repository.Cluster {
	if len(model.ProxyUrl) > 0 || model.ToConnectWithSSHTunnel {
		// converting old to new
		connectionConfig := &remoteConnection.RemoteConnectionConfig{
			Id:               model.RemoteConnectionConfigId,
			ProxyUrl:         model.ProxyUrl,
			SSHServerAddress: model.SSHTunnelServerAddress,
			SSHUsername:      model.SSHTunnelUser,
			SSHPassword:      model.SSHTunnelPassword,
			SSHAuthKey:       model.SSHTunnelAuthKey,
			Deleted:          false,
		}
		if len(model.ProxyUrl) > 0 {
			connectionConfig.ConnectionMethod = remoteConnectionBean.RemoteConnectionMethodProxy
		} else if model.ToConnectWithSSHTunnel {
			connectionConfig.ConnectionMethod = remoteConnectionBean.RemoteConnectionMethodSSH
		}
		model.RemoteConnectionConfig = connectionConfig
		// reset old config
		model.ProxyUrl = ""
		model.SSHTunnelUser = ""
		model.SSHTunnelPassword = ""
		model.SSHTunnelServerAddress = ""
		model.SSHTunnelAuthKey = ""
		model.ToConnectWithSSHTunnel = false
	}
	return model
}
