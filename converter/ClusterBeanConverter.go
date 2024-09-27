/*
 * Copyright (c) 2024. Devtron Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package converter

import (
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
)

type ClusterBeanConverter interface {
	GetClusterConfigFromClientBean(config *client.ClusterConfig) *k8sUtils.ClusterConfig
	GetClusterConfig(cluster *bean.ClusterInfo) *k8sUtils.ClusterConfig
	GetClusterInfo(c *repository.Cluster) *bean.ClusterInfo
	GetAllClusterInfo(clusters ...*repository.Cluster) []*bean.ClusterInfo
}

type ClusterBeanConverterImpl struct {
}

func NewConverterImpl() *ClusterBeanConverterImpl {
	return &ClusterBeanConverterImpl{}
}

func (impl *ClusterBeanConverterImpl) GetClusterConfigFromClientBean(config *client.ClusterConfig) *k8sUtils.ClusterConfig {
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

func (impl *ClusterBeanConverterImpl) GetClusterConfig(cluster *bean.ClusterInfo) *k8sUtils.ClusterConfig {
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

func (impl *ClusterBeanConverterImpl) GetClusterInfo(c *repository.Cluster) *bean.ClusterInfo {
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
			clusterInfo.KeyData = config[commonBean.TlsKey]
			clusterInfo.CertData = config[commonBean.CertData]
			clusterInfo.CAData = config[commonBean.CertificateAuthorityData]
		}
	}
	return clusterInfo
}

func (impl *ClusterBeanConverterImpl) GetAllClusterInfo(clusters ...*repository.Cluster) []*bean.ClusterInfo {
	clusterInfos := make([]*bean.ClusterInfo, 0, len(clusters))
	for _, c := range clusters {
		clusterInfos = append(clusterInfos, impl.GetClusterInfo(c))
	}
	return clusterInfos
}
