package converter

import (
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
)

type Converter interface {
	GetClusterConfigFromClientBean(config *client.ClusterConfig) *k8sUtils.ClusterConfig
	GetClusterConfig(cluster *bean.ClusterInfo) *k8sUtils.ClusterConfig
	GetClusterInfo(c *repository.Cluster) *bean.ClusterInfo
}

type ConverterImpl struct {
}

func NewConverterImpl() *ConverterImpl {
	return &ConverterImpl{}
}

func (impl *ConverterImpl) GetClusterConfigFromClientBean(config *client.ClusterConfig) *k8sUtils.ClusterConfig {
	clusterConfig := &k8sUtils.ClusterConfig{}
	if config != nil {
		clusterConfig = &k8sUtils.ClusterConfig{
			ClusterName:           config.ClusterName,
			Host:                  config.ApiServerUrl,
			BearerToken:           config.Token,
			InsecureSkipTLSVerify: config.InsecureSkipTLSVerify,
		}
		if config.InsecureSkipTLSVerify == false {
			clusterConfig.KeyData = config.GetKeyData()
			clusterConfig.CertData = config.GetCertData()
			clusterConfig.CAData = config.GetCaData()
		}
	}
	return clusterConfig
}

func (impl *ConverterImpl) GetClusterConfig(cluster *bean.ClusterInfo) *k8sUtils.ClusterConfig {
	clusterConfig := &k8sUtils.ClusterConfig{}
	if cluster != nil {
		clusterConfig = &k8sUtils.ClusterConfig{
			Host:                  cluster.ServerUrl,
			BearerToken:           cluster.BearerToken,
			ClusterName:           cluster.ClusterName,
			InsecureSkipTLSVerify: cluster.InsecureSkipTLSVerify,
			KeyData:               cluster.KeyData,
			CertData:              cluster.CertData,
			CAData:                cluster.CAData,
		}
	}
	return clusterConfig
}

func (impl *ConverterImpl) GetClusterInfo(c *repository.Cluster) *bean.ClusterInfo {
	clusterInfo := &bean.ClusterInfo{}
	if c != nil {
		config := c.Config
		bearerToken := config["bearer_token"]
		clusterInfo = &bean.ClusterInfo{
			ClusterId:             c.Id,
			ClusterName:           c.ClusterName,
			BearerToken:           bearerToken,
			ServerUrl:             c.ServerUrl,
			InsecureSkipTLSVerify: c.InsecureSkipTlsVerify,
		}
		if c.InsecureSkipTlsVerify == false {
			clusterInfo.KeyData = config[k8sUtils.TlsKey]
			clusterInfo.CertData = config[k8sUtils.CertData]
			clusterInfo.CAData = config[k8sUtils.CertificateAuthorityData]
		}
	}
	return clusterInfo
}
