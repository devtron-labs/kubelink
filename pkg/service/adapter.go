package service

import (
	"github.com/devtron-labs/common-lib/helmLib/registry"
	"github.com/devtron-labs/common-lib/utils/remoteConnection/bean"
	client "github.com/devtron-labs/kubelink/grpc"
)

func ConvertToRegistryConfig(credential *client.RegistryCredential) *registry.Configuration {
	var registryConfig *registry.Configuration
	if credential != nil {

		registryConfig := &registry.Configuration{
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

		connectionConfig := credential.RemoteConnectionConfig
		if connectionConfig != nil {
			registryConfig.RemoteConnectionConfig = &bean.RemoteConnectionConfigBean{}
			switch connectionConfig.RemoteConnectionMethod {
			case client.RemoteConnectionMethod_PROXY:
				registryConfig.RemoteConnectionConfig.ConnectionMethod = bean.RemoteConnectionMethod(bean.ConnectionMethod_Proxy)
				registryConfig.RemoteConnectionConfig.ProxyConfig = ConvertConfigToProxyConfig(connectionConfig)
			case client.RemoteConnectionMethod_SSH:
				registryConfig.RemoteConnectionConfig.ConnectionMethod = bean.RemoteConnectionMethod(bean.ConnectionMethod_SSH)
				registryConfig.RemoteConnectionConfig.SSHTunnelConfig = ConvertConfigToSSHTunnelConfig(connectionConfig)
			}
		}
	}
	return registryConfig
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
