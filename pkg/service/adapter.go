package service

import (
	"github.com/devtron-labs/common-lib/helmLib/registry"
	"github.com/devtron-labs/common-lib/utils/remoteConnection/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/util"
	"google.golang.org/protobuf/types/known/timestamppb"
	"helm.sh/helm/v3/pkg/release"
)

func NewRegistryConfig(credential *client.RegistryCredential) (*registry.Configuration, error) {
	var registryConfig *registry.Configuration
	if credential != nil {
		registryConfig = &registry.Configuration{
			RegistryId:                credential.RegistryName,
			RegistryUrl:               credential.RegistryUrl,
			Username:                  credential.Username,
			Password:                  credential.Password,
			AwsAccessKey:              credential.AccessKey,
			AwsSecretKey:              credential.SecretKey,
			AwsRegion:                 credential.AwsRegion,
			RegistryConnectionType:    credential.Connection,
			RegistryCertificateString: credential.RegistryCertificate,
			RegistryType:              credential.RegistryType,
			IsPublicRegistry:          credential.IsPublic,
		}

		if credential.Connection == registry.SECURE_WITH_CERT {
			certificatePath, err := registry.CreateCertificateFile(credential.RegistryName, credential.RegistryCertificate)
			if err != nil {
				return nil, err
			}
			registryConfig.RegistryCAFilePath = certificatePath
		}

		connectionConfig := credential.RemoteConnectionConfig
		if connectionConfig != nil {
			registryConfig.RemoteConnectionConfig = &bean.RemoteConnectionConfigBean{}
			switch connectionConfig.RemoteConnectionMethod {
			case client.RemoteConnectionMethod_DIRECT:
				registryConfig.RemoteConnectionConfig.ConnectionMethod = bean.RemoteConnectionMethodDirect
			case client.RemoteConnectionMethod_PROXY:
				registryConfig.RemoteConnectionConfig.ConnectionMethod = bean.RemoteConnectionMethodProxy
				registryConfig.RemoteConnectionConfig.ProxyConfig = ConvertConfigToProxyConfig(connectionConfig)
			case client.RemoteConnectionMethod_SSH:
				registryConfig.RemoteConnectionConfig.ConnectionMethod = bean.RemoteConnectionMethodSSH
				registryConfig.RemoteConnectionConfig.SSHTunnelConfig = ConvertConfigToSSHTunnelConfig(connectionConfig)
			}
		}
	}
	return registryConfig, nil
}

func ConvertConfigToProxyConfig(config *client.RemoteConnectionConfig) *bean.ProxyConfig {
	var proxyConfig *bean.ProxyConfig
	if config.ProxyConfig != nil {
		proxyConfig = &bean.ProxyConfig{
			ProxyUrl: config.ProxyConfig.ProxyUrl,
		}
	}
	return proxyConfig
}

func ConvertConfigToSSHTunnelConfig(config *client.RemoteConnectionConfig) *bean.SSHTunnelConfig {
	var sshConfig *bean.SSHTunnelConfig
	if config.SSHTunnelConfig != nil {
		sshConfig = &bean.SSHTunnelConfig{
			SSHUsername:      config.SSHTunnelConfig.SSHUsername,
			SSHPassword:      config.SSHTunnelConfig.SSHPassword,
			SSHAuthKey:       config.SSHTunnelConfig.SSHAuthKey,
			SSHServerAddress: config.SSHTunnelConfig.SSHServerAddress,
		}
	}
	return sshConfig
}

func NewDeployedAppDetail(config *client.ClusterConfig, release *release.Release) *client.DeployedAppDetail {
	return &client.DeployedAppDetail{
		AppId:        util.GetAppId(config.ClusterId, release),
		AppName:      release.Name,
		ChartName:    release.Chart.Name(),
		ChartAvatar:  release.Chart.Metadata.Icon,
		LastDeployed: timestamppb.New(release.Info.LastDeployed.Time),
		EnvironmentDetail: &client.EnvironmentDetails{
			ClusterName: config.ClusterName,
			ClusterId:   config.ClusterId,
			Namespace:   release.Namespace,
		},
	}
}
