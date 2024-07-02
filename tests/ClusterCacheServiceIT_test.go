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

package tests

import (
	"compress/gzip"
	"context"
	client2 "github.com/devtron-labs/authenticator/client"
	"github.com/devtron-labs/common-lib/utils"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/cache"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	k8sInformer2 "github.com/devtron-labs/kubelink/pkg/k8sInformer"
	service2 "github.com/devtron-labs/kubelink/pkg/service/commonHelmService"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/go-pg/pg"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"io/ioutil"
	"k8s.io/helm/pkg/chartutil"
	chart2 "k8s.io/helm/pkg/proto/hapi/chart"
	"os"
	"path/filepath"
	"reflect"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"testing"
)

type chartReleaseRequest struct {
	ClusterId          int32
	ChartType          string
	ReleaseName        string
	ReleaseNamespace   string
	ChartPath          string
	ChartTypeYamlValue string
}

func TestGetAppDetailsByComparingWithCacheData(t *testing.T) {

	logger, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, k8sServiceImpl := getHelmAppServiceDependencies(t)
	clusterCacheConfig := &cache.ClusterCacheConfig{
		ClusterIdList:                 nil,
		ClusterCacheListSemaphoreSize: 20,
		ClusterCacheListPageSize:      50,
		ClusterSyncBatchSize:          3,
	}
	runTimeConfig := &client2.RuntimeConfig{LocalDevMode: true}
	k8sUtilLocal := k8sUtils.NewK8sUtil(logger, runTimeConfig)

	clusterCacheImpl := cache.NewClusterCacheImpl(logger, clusterCacheConfig, clusterRepository, k8sUtilLocal, k8sInformer)
	helmAppServiceImpl := service.NewHelmAppServiceImpl(logger, k8sServiceImpl, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, clusterCacheImpl)

	tests := []chartReleaseRequest{{
		ClusterId:          clusterId,
		ChartType:          commonBean.DeploymentKind,
		ReleaseName:        deploymentReleaseName,
		ReleaseNamespace:   deploymentReleaseNamespace,
		ChartPath:          deploymentReferenceTemplateDir,
		ChartTypeYamlValue: DeploymentYamlValue,
	}, {
		ClusterId:          clusterId,
		ChartType:          commonBean.StatefulSetKind,
		ReleaseName:        statefulSetReleaseName,
		ReleaseNamespace:   statefulSetReleaseNamespace,
		ChartPath:          statefulSetReferenceTemplateDir,
		ChartTypeYamlValue: StatefulSetYamlValue,
	}, {
		ClusterId:          clusterId,
		ChartType:          commonBean.K8sClusterResourceRolloutKind,
		ReleaseName:        rolloutReleaseName,
		ReleaseNamespace:   rolloutReleaseNamespace,
		ChartPath:          rolloutReferenceTemplateDir,
		ChartTypeYamlValue: RollOutYamlValue,
	}, {
		ClusterId:          clusterId,
		ChartType:          commonBean.K8sClusterResourceCronJobKind,
		ReleaseName:        jobCronjobReleaseName,
		ReleaseNamespace:   jobCronjobReleaseNamespace,
		ChartPath:          cronjobReferenceTemplateDir,
		ChartTypeYamlValue: CronJobYamlValue,
	}}

	for _, tt := range tests {
		devtronAppInstallRequest := getInstallRequest(t, tt)
		installReleaseResp, err := helmAppServiceImpl.InstallReleaseWithCustomChart(context.Background(), devtronAppInstallRequest)
		assert.Nil(t, err)
		assert.True(t, installReleaseResp)
		releaseIdentifier := devtronAppInstallRequest.ReleaseIdentifier
		appDetailBeforeSync := getAppDetails(t, releaseIdentifier, helmAppServiceImpl)

		//cluster cache sync started
		model, err := clusterRepository.FindById(int(releaseIdentifier.ClusterConfig.ClusterId))
		assert.Nil(t, err)
		clusterInfo := k8sInformer2.GetClusterInfo(model)
		clusterCacheImpl.SyncClusterCache(clusterInfo)
		//cluster cache sync ended

		// after cache sync
		appDetailAfterSync := getAppDetails(t, releaseIdentifier, helmAppServiceImpl)
		compareAppDetails(t, tt.ChartType, appDetailBeforeSync, appDetailAfterSync)
	}
}

func compareAppDetails(t *testing.T, deployType string, appDetailBeforeSync *bean.AppDetail, appDetailAfterSync *bean.AppDetail) {
	// check Application status
	deploymentAppStatus := appDetailBeforeSync.ApplicationStatus
	cacheAppStatus := appDetailAfterSync.ApplicationStatus
	if deploymentAppStatus != cacheAppStatus {
		t.Errorf("Application status Before Cluster sync = %v, Application Status After Cluster Sync = %v", deploymentAppStatus, cacheAppStatus)
		return
	}
	// check Release status
	releaseStatus := appDetailBeforeSync.ReleaseStatus
	cacheReleaseStatus := appDetailAfterSync.ReleaseStatus
	if !reflect.DeepEqual(releaseStatus, cacheReleaseStatus) {
		t.Errorf("Release status Before cluster sync = %v, Release Status After cluster sync = %v", *releaseStatus, *cacheReleaseStatus)
	}

	resourceInfoMapBeforeSync := buildResourceInfo(appDetailBeforeSync)
	resourceInfoMapAfterSync := buildResourceInfo(appDetailAfterSync)
	if len(resourceInfoMapAfterSync) != len(resourceInfoMapBeforeSync) {
		t.Errorf("Different Node length")
		return
	}
	replicaCount, cacheReplicaCount := 0, 0
	var isPVCDeployment, isPVCCache bool
	for uid, resourceInfo := range resourceInfoMapBeforeSync {
		if nodeInfoAfterSync, ok := resourceInfoMapAfterSync[uid]; ok {
			// Status_of_pod and other resources
			if !reflect.DeepEqual(resourceInfoMapBeforeSync[uid].health, nodeInfoAfterSync.health) {
				t.Errorf("Status of pod health before cluster sync = %v , Status of pod health After cluster sync = %v ", resourceInfoMapBeforeSync[uid].health, nodeInfoAfterSync.health)
				break
			}
			// Port number comparison for Service, Endpoints and EndpointSlice
			if (resourceInfo.kind == "Service" || resourceInfo.kind == "EndpointSlice" || resourceInfo.kind == "Endpoints") && resourceInfo.kind == nodeInfoAfterSync.kind {
				if !reflect.DeepEqual(resourceInfo.port, nodeInfoAfterSync.port) {
					t.Errorf("port number before cluster cache = %v, port number after cluster cache = %v", resourceInfoMapBeforeSync[uid].port, nodeInfoAfterSync.port)
					break
				}
			}
			// Comparing labels for NetworkingInfo
			networkingInfo := nodeInfoAfterSync.NetworkingInfo
			cacheNetworkingInfo := resourceInfo.NetworkingInfo
			if !reflect.DeepEqual(networkingInfo.Labels, cacheNetworkingInfo.Labels) {
				t.Errorf("Networking Info Before cluster sync = %v, Networking Info After cluster sync = %v", networkingInfo.Labels, cacheNetworkingInfo.Labels)
				break
			}
			// Check age of pod after changes
			resourceKind := resourceInfo.kind
			cacheResourceKind := nodeInfoAfterSync.kind
			if resourceKind == "pod" && resourceKind == cacheResourceKind {
				replicaCount++
				cacheReplicaCount++
				if !reflect.DeepEqual(resourceInfo.CreatedAt, nodeInfoAfterSync.CreatedAt) {
					t.Errorf("Pod Created at before cluster cache = %v, Pod Created at after cluster cache = %v", resourceInfoMapBeforeSync[uid].CreatedAt, nodeInfoAfterSync.CreatedAt)
					break
				}
				// Restart count for a pod
				podRestart := resourceInfo.info
				cachePodRestart := nodeInfoAfterSync.info
				for i := 0; i < len(podRestart); i++ {
					if podRestart[i].Name == "Restart Count" {
						if podRestart[i].Value != cachePodRestart[i].Value {
							t.Errorf("Restart count for pod Before Cluster sync = %v, Restart count for pod After CLuster sync = %v", podRestart[i].Value, cachePodRestart[i].Value)
						}
					}
				}
			}
			if resourceKind == commonBean.PersistentVolumeClaimKind && cacheResourceKind == resourceKind {
				isPVCDeployment = true
				isPVCCache = true
			}
			// check Hibernation
			if resourceInfo.IsHibernated != nodeInfoAfterSync.IsHibernated {
				t.Errorf("Is Hibernated before cluster cache = %v, Hibernate status after cluster cache = %v", resourceInfoMapBeforeSync[uid].IsHibernated, nodeInfoAfterSync.IsHibernated)
				break
			}
			// check for pod metadata
			podMetaData := resourceInfo.PodMetaData
			cachePodMetaData := nodeInfoAfterSync.PodMetaData
			if isErr := checkForPodMetadata(t, podMetaData, cachePodMetaData); isErr {
				return
			}

		}
	}
	if deployType == commonBean.StatefulSetKind {
		if !isPVCDeployment || !isPVCCache {
			t.Errorf("PVC is missing")
			return
		}
	}
	// check for Replica count for pod
	if replicaCount != cacheReplicaCount {
		t.Errorf("Replica count Before cluster sync = %v, Replica count After Cluster sync = %v", replicaCount, cacheReplicaCount)
		return
	}

}

func checkForPodMetadata(t *testing.T, podMetaData []*bean.PodMetadata, cachePodMetaData []*bean.PodMetadata) bool {
	if !reflect.DeepEqual(podMetaData, cachePodMetaData) {
		t.Errorf("Pod Meta data before Cluster sync = %v, Pod Meta data After cluster sync = %v", podMetaData, cachePodMetaData)
		return true
	}
	// check for New Pods
	var newPodData, oldPodData []string
	var cacheNewPodData, cacheOldPodData []string
	for j := 0; j < len(podMetaData); j++ {
		podName := podMetaData[j].Name
		if podMetaData[j].IsNew {
			newPodData = append(newPodData, podName)
		} else {
			oldPodData = append(oldPodData, podName)
		}
	}

	for k := 0; k < len(cachePodMetaData); k++ {
		podName := cachePodMetaData[k].Name
		if cachePodMetaData[k].IsNew {
			cacheNewPodData = append(cacheNewPodData, podName)
		} else {
			cacheOldPodData = append(cacheOldPodData, podName)
		}
	}
	if !reflect.DeepEqual(newPodData, cacheNewPodData) || !reflect.DeepEqual(oldPodData, cacheOldPodData) {
		t.Errorf("Pod data is different for new and old pod")
		return true
	}
	// check for Ephemeral containers and Init Containers
	for i := 0; i < len(podMetaData); i++ {
		if !reflect.DeepEqual(podMetaData[i].EphemeralContainers, cachePodMetaData[i].EphemeralContainers) {
			t.Errorf("Ephemeral Containers does not exist")
			return true
		}
		if !reflect.DeepEqual(podMetaData[i].InitContainers, cachePodMetaData[i].InitContainers) {
			t.Errorf("Init Containers does not exist")
			return true
		}
	}
	return false
}

func getAppDetails(t *testing.T, releaseIdentifier *client.ReleaseIdentifier, helmAppServiceImpl *service.HelmAppServiceImpl) *bean.AppDetail {
	appDetailReqDev := &client.AppDetailRequest{
		ClusterConfig: releaseIdentifier.ClusterConfig,
		Namespace:     releaseIdentifier.ReleaseNamespace,
		ReleaseName:   releaseIdentifier.ReleaseName,
	}
	appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReqDev)
	assert.Nil(t, err)
	return appDetail
}

func buildResourceInfo(appDetail *bean.AppDetail) map[string]NodeInfo {
	resourceTreeSize := len(appDetail.ResourceTreeResponse.Nodes)
	// Storing Resource data before Cluster sync
	resourceDataMap := map[string]NodeInfo{}
	for i := 0; i < resourceTreeSize; i++ {
		node := appDetail.ResourceTreeResponse.Nodes[i]
		uid := node.UID
		nodeInfo := NodeInfo{
			kind:              node.Kind,
			health:            node.Health,
			port:              node.Port,
			CreatedAt:         node.CreatedAt,
			IsHibernated:      node.IsHibernated,
			PodMetaData:       appDetail.ResourceTreeResponse.PodMetadata,
			ReleaseStatus:     appDetail.ReleaseStatus,
			ApplicationStatus: appDetail.ApplicationStatus,
			info:              node.Info,
			NetworkingInfo:    node.NetworkingInfo,
		}
		resourceDataMap[uid] = nodeInfo
	}
	return resourceDataMap
}

type NodeInfo struct {
	kind              string
	health            *bean.HealthStatus
	port              []int64
	CreatedAt         string
	IsHibernated      bool
	PodMetaData       []*bean.PodMetadata
	ReleaseStatus     *bean.ReleaseStatus
	ApplicationStatus *bean.HealthStatusCode
	info              []bean.InfoItem
	NetworkingInfo    *bean.ResourceNetworkingInfo
}

func getInstallRequest(t *testing.T, tt chartReleaseRequest) *client.HelmInstallCustomRequest {

	releaseName := tt.ReleaseName
	releaseNamespace := tt.ReleaseNamespace
	refChartPath := tt.ChartPath
	chartTypeYamlValue := tt.ChartTypeYamlValue
	chartMetadata := &chart2.Metadata{
		Name:       appName,
		Version:    chartVersion,
		ApiVersion: apiVersion,
	}
	refChartByte := packageChartAndGetByteArrayRefChart(t, refChartPath, chartMetadata)
	customReq := &client.HelmInstallCustomRequest{
		ReleaseIdentifier: &client.ReleaseIdentifier{
			ClusterConfig:    clusterConfig,
			ReleaseName:      releaseName,
			ReleaseNamespace: releaseNamespace,
		},
		ValuesYaml: chartTypeYamlValue,
		ChartContent: &client.ChartContent{
			Content: refChartByte,
		},
	}
	return customReq
}

func packageChartAndGetByteArrayRefChart(t *testing.T, refChartPath string, chartMetadata *chart2.Metadata) []byte {
	valid, err := chartutil.IsChartDir(refChartPath)
	assert.Nil(t, err)
	if !valid {
		return nil
	}
	b, err := yaml.Marshal(chartMetadata)
	assert.Nil(t, err)
	err = ioutil.WriteFile(filepath.Join(refChartPath, "Chart.yaml"), b, 0600)
	assert.Nil(t, err)

	chart, err := chartutil.LoadDir(refChartPath)
	assert.Nil(t, err)

	archivePath, err := chartutil.Save(chart, refChartPath)
	assert.Nil(t, err)

	file, err := os.Open(archivePath)
	reader, err := gzip.NewReader(file)
	assert.Nil(t, err)

	// read the complete content of the file h.Name into the bs []byte
	bs, err := ioutil.ReadAll(reader)
	assert.Nil(t, err)
	return bs
}

func GetDbConnAndLoggerService(t *testing.T) (*zap.SugaredLogger, *pg.DB) {
	cfg, _ := sql.GetConfig()
	logger, err := utils.NewSugardLogger()
	assert.Nil(t, err)
	dbConnection, err := sql.NewDbConnection(cfg, logger)
	assert.Nil(t, err)

	return logger, dbConnection
}

func getHelmAppServiceDependencies(t *testing.T) (*zap.SugaredLogger, *k8sInformer2.K8sInformerImpl, *service2.HelmReleaseConfig,
	*k8sUtils.K8sUtil, *repository.ClusterRepositoryImpl, *service2.K8sServiceImpl) {
	logger, dbConnection := GetDbConnAndLoggerService(t)
	helmReleaseConfig := &service2.HelmReleaseConfig{
		EnableHelmReleaseCache:    false,
		MaxCountForHelmRelease:    0,
		ManifestFetchBatchSize:    0,
		RunHelmInstallInAsyncMode: false,
	}
	helmReleaseConfig2 := &k8sInformer2.HelmReleaseConfig{EnableHelmReleaseCache: true}
	clusterRepository := repository.NewClusterRepositoryImpl(dbConnection, logger)
	runTimeConfig := &client2.RuntimeConfig{LocalDevMode: false}
	k8sUtil := k8sUtils.NewK8sUtil(logger, runTimeConfig)
	k8sInformer := k8sInformer2.Newk8sInformerImpl(logger, clusterRepository, helmReleaseConfig2, k8sUtil)
	k8sServiceImpl := service2.NewK8sServiceImpl(logger)
	return logger, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, k8sServiceImpl
}
