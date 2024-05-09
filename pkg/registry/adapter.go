package registry

import (
	"github.com/devtron-labs/common-lib/utils/remoteConnection/bean"
	client "github.com/devtron-labs/kubelink/grpc"
)

func ConvertToRegistryConfig(credential *client.RegistryCredential) *bean.RegistryConfig {
	var registryConfig *bean.RegistryConfig
	if credential != nil {
		registryConfig = &bean.RegistryConfig{
			RegistryId:                credential.RegistryName,
			RegistryUrl:               credential.RegistryUrl,
			RegistryUsername:          credential.Username,
			RegistryPassword:          credential.Password,
			RegistryConnectionType:    credential.Connection,
			RegistryCertificateString: credential.RegistryCertificate,
		}
		connectionConfig := credential.RemoteConnectionConfig
		if connectionConfig != nil {
			registryConfig.ConnectionMethod = bean.ConnectionMethod(connectionConfig.RemoteConnectionMethod)
			switch connectionConfig.RemoteConnectionMethod {
			case client.RemoteConnectionMethod_PROXY:
				registryConfig.ConnectionMethod = bean.ConnectionMethod_Proxy
				registryConfig.ProxyConfig = ConvertConfigToProxyConfig(connectionConfig)
			case client.RemoteConnectionMethod_SSH:
				registryConfig.ConnectionMethod = bean.ConnectionMethod_SSH
				registryConfig.SSHConfig = ConvertConfigToSSHTunnelConfig(connectionConfig)
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
