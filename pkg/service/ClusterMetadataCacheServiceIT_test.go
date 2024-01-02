package service

import (
	"context"
	"fmt"
	client2 "github.com/devtron-labs/authenticator/client"
	"github.com/devtron-labs/common-lib/utils"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/cache"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	k8sInformer2 "github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/sql"
	test_data "github.com/devtron-labs/kubelink/test-data"
	"github.com/go-pg/pg"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"os"
	"reflect"
	"testing"
)

const (
	mongoOperatorChartName = "mongo-operator"
)

var clusterName = "default_cluster"
var clusterId int32 = 1

// helm chart variables
var helmAppReleaseName = "mongo-operator"
var helmAppReleaseNamespace = "ns1"
var helmChartName = "community-operator"
var helmChartVersion = "0.8.3"
var helmChartRepoName = "mongodb"
var helmChartUrl = "https://mongodb.github.io/helm-charts"

// cronjob and job variables
var jobCronjobReleaseName = "cache-test-cronjob-devtron-demo"
var jobCronjobReleaseNamespace = "devtron-demo"

// deployment variables
var deploymentReleaseName = "testing-deployment-devtron-demo"
var deploymentReleaseNamespace = "devtron-demo"

// rollout variables
var rolloutReleaseName = "cache-test-01-default-cluster--devtroncd"
var rolloutReleaseNamespace = "devtron-demo"

// statefulSet variables
var statefulSetReleaseName = "cache-test-stateful-devtron-demo"
var statefulSetReleaseNamespace = "devtron-demo"

var clusterConfig = &client.ClusterConfig{
	ApiServerUrl:          "",
	Token:                 "",
	ClusterId:             clusterId,
	ClusterName:           clusterName,
	InsecureSkipTLSVerify: true,
}

var installReleaseReq = &client.InstallReleaseRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      helmAppReleaseName,
		ReleaseNamespace: helmAppReleaseNamespace,
	},
	ChartName:    helmChartName,
	ChartVersion: helmChartVersion,
	ValuesYaml:   test_data.InstallReleaseReqYamlValue,
	ChartRepository: &client.ChartRepository{
		Name: helmChartRepoName,
		Url:  helmChartUrl,
	},
}

var installReleaseReqJobAndCronJob = &client.HelmInstallCustomRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      jobCronjobReleaseName,
		ReleaseNamespace: jobCronjobReleaseNamespace,
	},
	ValuesYaml: test_data.CronJobYamlValue,
	ChartContent: &client.ChartContent{
		Content: cronJobRefChart,
	},
}

var installReleaseReqDeployment = &client.HelmInstallCustomRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      deploymentReleaseName,
		ReleaseNamespace: deploymentReleaseNamespace,
	},
	ValuesYaml: test_data.DeploymentYamlValue,
	ChartContent: &client.ChartContent{
		Content: deploymentRefChart,
	},
}

var installReleaseReqRollout = &client.HelmInstallCustomRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      rolloutReleaseName,
		ReleaseNamespace: rolloutReleaseNamespace,
	},
	ValuesYaml: test_data.RollOutYamlValue,
	ChartContent: &client.ChartContent{
		Content: rolloutDeploymentCharContent,
	},
}

var installReleaseReqStatefullset = &client.HelmInstallCustomRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      statefulSetReleaseName,
		ReleaseNamespace: statefulSetReleaseNamespace,
	},
	ValuesYaml: test_data.StatefulSetYamlValue,
	ChartContent: &client.ChartContent{
		Content: statefullsetsChartContent,
	},
}

var devtronPayloadArray = [4]*client.HelmInstallCustomRequest{installReleaseReqRollout, installReleaseReqStatefullset, installReleaseReqDeployment, installReleaseReqJobAndCronJob}

var helmPayloadArray = [1]*client.InstallReleaseRequest{installReleaseReq}

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

func TestHelmAppService_BuildAppDetail(t *testing.T) {
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
	helmAppServiceImpl := NewHelmAppServiceImpl(logger, k8sServiceImpl, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, clusterCacheImpl)
	var resourceTreeMap = map[string]*bean.AppDetail{}
	var helmAppResourceTreeMap = map[string]*bean.AppDetail{}
	var cacheResourceTreeMap = map[string]*bean.AppDetail{}
	var cacheHelmResourceTreeMap = map[string]*bean.AppDetail{}

	prepareResourceTreeMapBeforeClusterSyncForDevtronApp(t, &resourceTreeMap, &helmAppResourceTreeMap, helmAppServiceImpl)
	resourceDataBeforeSync := buildResourceInfoBeforeCacheSync(resourceTreeMap)
	helmAppResourceTreeSize := len(helmAppResourceTreeMap[mongoOperatorChartName].ResourceTreeResponse.Nodes)
	resourceTreeSize := len(resourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.Nodes)
	resourceTreeSizeStatefulSets := len(resourceTreeMap[commonBean.StatefulSetKind].ResourceTreeResponse.Nodes)

	//cluster cache sync started
	model, err := clusterRepository.FindById(int(installReleaseReq.ReleaseIdentifier.ClusterConfig.ClusterId))
	assert.Nil(t, err)
	clusterInfo := k8sInformer2.GetClusterInfo(model)

	clusterCacheImpl.SyncClusterCache(clusterInfo)
	//cluster cache sync ended

	prepareResourceTreeMapAfterClusterSyncForDevtronApp(t, &cacheResourceTreeMap, &cacheHelmResourceTreeMap, helmAppServiceImpl)
	resourceDataAfterSync := buildResourceInfoAfterCacheSync(cacheResourceTreeMap)
	cacheAppResourceTreeSize := len(cacheHelmResourceTreeMap[mongoOperatorChartName].ResourceTreeResponse.Nodes)
	//Health Status for Pod and other resources
	t.Run("Status_of_pod and other resources", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for uid, _ := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				if !reflect.DeepEqual(resourceDataBeforeSync[uid].health, nodeInfoAfterSync.health) {
					t.Errorf("Status of pod health before cluster sync = %v , Status of pod health After cluster sync = %v ", resourceDataBeforeSync[uid].health, nodeInfoAfterSync.health)
					break
				}
			}
		}
	})

	//Port number comparison for Service, Endpoints and EndpointSlice
	t.Run("Service, Endpoints and EndpointSlice with port numbers", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for uid, resourceInfo := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				if (resourceInfo.kind == "Service" || resourceInfo.kind == "EndpointSlice" || resourceInfo.kind == "Endpoints") && resourceInfo.kind == nodeInfoAfterSync.kind {
					if !reflect.DeepEqual(resourceInfo.port, nodeInfoAfterSync.port) {
						t.Errorf("port number before cluster cache = %v, port number after cluster cache = %v", resourceDataBeforeSync[uid].port, nodeInfoAfterSync.port)
						break
					}
				}
			}
		}
	})

	// Validation for NetworkingInfo
	t.Run("Comparing labels for NetworkingInfo", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for uid, resourceInfo := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				deploymentNetworkingInfo := nodeInfoAfterSync.NetworkingInfo
				cacheNetworkingInfo := resourceInfo.NetworkingInfo
				if !reflect.DeepEqual(deploymentNetworkingInfo.Labels, cacheNetworkingInfo.Labels) {
					t.Errorf("Networking Info Before cluster sync = %v, Networking Info After cluster sync = %v", deploymentNetworkingInfo.Labels, cacheNetworkingInfo.Labels)
					break
				}
			}
		}
	})

	// Pod age validation
	t.Run("Check age of pod after changes", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for uid, resourceInfo := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				deploymentKind := resourceInfo.kind
				cacheKind := nodeInfoAfterSync.kind
				if deploymentKind == "pod" && deploymentKind == cacheKind {
					if !reflect.DeepEqual(resourceInfo.CreatedAt, nodeInfoAfterSync.CreatedAt) {
						t.Errorf("Pod Created at before cluster cache = %v, Pod Created at after cluster cache = %v", resourceDataBeforeSync[uid].CreatedAt, nodeInfoAfterSync.CreatedAt)
						break
					}
				}
			}
		}
	})

	// CanBeHibernated
	t.Run("Check hibernation", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for uid, resourceInfo := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				if resourceInfo.IsHibernated != nodeInfoAfterSync.IsHibernated {
					t.Errorf("Is Hibernated before cluster cache = %v, Hibernate status after cluster cache = %v", resourceDataBeforeSync[uid].IsHibernated, nodeInfoAfterSync.IsHibernated)
				}
			}
		}
	})

	// Pod Meta Data
	t.Run("Pod Meta data", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for uid, resourceInfo := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				deploymentPodMetaData := resourceInfo.PodMetaData
				cachePodMetaData := nodeInfoAfterSync.PodMetaData
				if !reflect.DeepEqual(deploymentPodMetaData, cachePodMetaData) {
					t.Errorf("Pod Meta data before Cluster sync = %v, Pod Meta data After cluster sync = %v", deploymentPodMetaData, cachePodMetaData)
				}
			}
		}

	})

	// Release status
	t.Run("Release status", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for i := 0; i < resourceTreeSize; i++ {
			deploymentReleaseStatus := resourceTreeMap[commonBean.DeploymentKind].ReleaseStatus
			cacheReleaseStatus := cacheResourceTreeMap[commonBean.DeploymentKind].ReleaseStatus
			if !reflect.DeepEqual(deploymentReleaseStatus, cacheReleaseStatus) {
				t.Errorf("Release status Before cluster sync = %v, Release Status After cluster sync = %v", *deploymentReleaseStatus, *cacheReleaseStatus)
			}
		}
	})

	//Application Status
	t.Run("Application status", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		deploymentAppStatus := *resourceTreeMap[commonBean.DeploymentKind].ApplicationStatus
		cacheAppStatus := *cacheResourceTreeMap[commonBean.DeploymentKind].ApplicationStatus
		if deploymentAppStatus != cacheAppStatus {
			t.Errorf("Application status Before Cluster sync = %v, Application Status After Cluster Sync = %v", deploymentAppStatus, cacheAppStatus)
		}
	})

	// Count ReplicaSets
	t.Run("Replica count for pod", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		deploymentReplicaCount, cacheReplicaCount := 0, 0

		// For deployment type chart
		//since size check has passed so iterating over resourceTreeSize since cache nodes size will also be same
		for i := 0; i < resourceTreeSize; i++ {
			deploymentKind := resourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := cacheResourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				deploymentReplicaCount++
			}
			if cacheReplicaKind == "pod" {
				cacheReplicaCount++
			}
		}
		if deploymentReplicaCount != cacheReplicaCount {
			t.Errorf("Replica count of Deployment Before cluster sync = %v, Replica count of deployment After Cluster sync = %v", deploymentReplicaCount, cacheReplicaCount)
		}

		// For StatefulSet deployment chart
		statefulSetReplicaCount, cacheStatefulSetReplicaCount := 0, 0
		for i := 0; i < len(resourceTreeMap[commonBean.StatefulSetKind].ResourceTreeResponse.Nodes); i++ {
			deploymentKind := resourceTreeMap[commonBean.StatefulSetKind].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := cacheResourceTreeMap[commonBean.StatefulSetKind].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				statefulSetReplicaCount++
			}
			if cacheReplicaKind == "pod" {
				cacheStatefulSetReplicaCount++
			}
		}
		if statefulSetReplicaCount != cacheStatefulSetReplicaCount {
			t.Errorf("Replica count of StatefullSets Before cluster sync = %v, Replica count of StatefullSets After Cluster sync = %v", statefulSetReplicaCount, cacheStatefulSetReplicaCount)
		}

		// For RollOut type chart
		rollOutReplicaCount, cacheRollOutCount := 0, 0
		for i := 0; i < len(resourceTreeMap[commonBean.K8sClusterResourceRolloutKind].ResourceTreeResponse.Nodes); i++ {
			deploymentKind := resourceTreeMap[commonBean.K8sClusterResourceRolloutKind].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := cacheResourceTreeMap[commonBean.K8sClusterResourceRolloutKind].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				rollOutReplicaCount++
			}
			if cacheReplicaKind == "pod" {
				cacheRollOutCount++
			}
		}
		if rollOutReplicaCount != cacheRollOutCount {
			t.Errorf("Replica Count of RollOut Before Cluster sync = %v, Replica count After Cluster Sync = %v", rollOutReplicaCount, cacheRollOutCount)
		}

		// For Job and Cronjob
		cronJobReplicaCount, cacheCronJobCount := 0, 0
		for i := 0; i < len(resourceTreeMap[commonBean.K8sClusterResourceCronJobKind].ResourceTreeResponse.Nodes); i++ {
			deploymentKind := resourceTreeMap[commonBean.K8sClusterResourceCronJobKind].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := cacheResourceTreeMap[commonBean.K8sClusterResourceCronJobKind].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				cronJobReplicaCount++
			}
			if cacheReplicaKind == "pod" {
				cacheCronJobCount++
			}
		}
		if cronJobReplicaCount != cacheCronJobCount {
			t.Errorf("Replica count of Job And CronJob Before Cluster sync = %v, Replica count of Job And CronJob After Cluster sync = %v", cronJobReplicaCount, cacheCronJobCount)
		}
	})

	// Restart Count
	t.Run("Restart count for a pod", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for uid, resourceInfo := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				if resourceDataBeforeSync[uid].kind == "pod" {
					deploymentRestart := resourceInfo.info
					cacheRestart := nodeInfoAfterSync.info
					for i := 0; i < len(deploymentRestart); i++ {
						if deploymentRestart[i].Name == "Restart Count" {
							if deploymentRestart[i].Value != cacheRestart[i].Value {
								t.Errorf("Restart count for pod Before Cluster sync = %v, Restart count for pod After CLuster sync = %v", deploymentRestart[i].Value, cacheRestart[i].Value)
							}
						}
					}
				}
			}
		}
	})

	// Count of pods
	t.Run("ReplicaCount as > 1, which is the Count of pods ready.", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		deploymentPodCount, cachePodCount := 0, 0
		for i := 0; i < resourceTreeSize; i++ {
			deploymentKind := resourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.Nodes[i].Kind
			cacheKind := cacheResourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				deploymentPodCount++
			}
			if cacheKind == "pod" {
				cachePodCount++
			}
		}
		if cachePodCount != deploymentPodCount {
			t.Errorf("Ready pod count Before Cluster sync = %v, Ready pod count After Cluster sync = %v", deploymentPodCount, cachePodCount)
		}
	})

	// PersistentVolumeClaim for StatefulSets Deployment
	t.Run("Persistence volume", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		isPVCDeployment, isPVCCache := false, false
		for i := 0; i < resourceTreeSizeStatefulSets; i++ {
			deploymentKind := resourceTreeMap[commonBean.StatefulSetKind].ResourceTreeResponse.Nodes[i].Kind
			cacheKind := cacheResourceTreeMap[commonBean.StatefulSetKind].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "PersistentVolumeClaim" {
				isPVCDeployment = true
			}
			if cacheKind == "PersistentVolumeClaim" {
				isPVCCache = true
			}
		}
		if !isPVCDeployment || !isPVCCache {
			t.Errorf("PVC is missing")
		}
	})

	// Init and Ephemeral container
	t.Run("Init and Ephemeral container validation", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		deploymentEphemeralContainer := resourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.PodMetadata
		cacheEphemeralContainer := cacheResourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.PodMetadata
		for i := 0; i < len(deploymentEphemeralContainer); i++ {
			if !reflect.DeepEqual(deploymentEphemeralContainer[i].EphemeralContainers, cacheEphemeralContainer[i].EphemeralContainers) {
				t.Errorf("Ephemeral Containers does not exist")
			}
			if !reflect.DeepEqual(deploymentEphemeralContainer[i].InitContainers, cacheEphemeralContainer[i].InitContainers) {
				t.Errorf("Init Containers does not exist")
			}
		}
	})

	// newPod vs oldPod
	t.Run("NewPod Vs OldPod", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		var newPodData []string
		var cacheNewPodData []string
		// empty the impl.clustersCache
		//install release with cust chart
		//build app detail se resource tree with new image jisme new pod rhenge jisme isNew ka tag bhi rhega
		//sync cluster cache
		//now get resoure tree after cache sync
		// then compare
		model, err := clusterRepository.FindById(int(installReleaseReq.ReleaseIdentifier.ClusterConfig.ClusterId))
		assert.Nil(t, err)
		clusterInfo := k8sInformer2.GetClusterInfo(model)
		clusterCacheImpl.SyncClusterCache(clusterInfo)

		appDetailReqDev := &client.AppDetailRequest{
			ClusterConfig: installReleaseReqDeployment.ReleaseIdentifier.ClusterConfig,
			Namespace:     installReleaseReqDeployment.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   installReleaseReqDeployment.ReleaseIdentifier.ReleaseName,
		}
		cacheAppDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReqDev)
		assert.Nil(t, err)

		deploymentNewPod := resourceTreeMap["Deployment"].ResourceTreeResponse.PodMetadata
		cacheNewPod := cacheAppDetail.ResourceTreeResponse.PodMetadata
		for j := 0; j < len(deploymentNewPod); j++ {
			if deploymentNewPod[j].IsNew {
				newPodData = append(newPodData, deploymentNewPod[j].Name)
			}
		}

		for k := 0; k < len(cacheNewPod); k++ {
			if cacheNewPod[k].IsNew {
				cacheNewPodData = append(cacheNewPodData, cacheNewPod[k].Name)
			}
		}
		if !reflect.DeepEqual(newPodData, cacheNewPodData) {
			t.Errorf("Pod data is different for new and old pod")
		}
	})

	// Test Cases for helm App
	t.Run("Release_status for helm app", func(t *testing.T) {
		if helmAppResourceTreeSize != cacheAppResourceTreeSize {
			t.Errorf("Different Node length")
			return
		}
		for i := 0; i < helmAppResourceTreeSize; i++ {
			helmReleaseStatus := helmAppResourceTreeMap[mongoOperatorChartName].ReleaseStatus
			cacheHelmReleaseStatus := cacheHelmResourceTreeMap[mongoOperatorChartName].ReleaseStatus
			if !reflect.DeepEqual(helmReleaseStatus, cacheHelmReleaseStatus) {
				t.Errorf("Release status Before cluster sync = %v, Release status After cluster sync = %v", helmReleaseStatus, cacheHelmReleaseStatus)
			}
		}
	})

	t.Run("App_status for helm app", func(t *testing.T) {
		if helmAppResourceTreeSize != cacheAppResourceTreeSize {
			t.Errorf("Different Node length")
			return
		}
		for i := 0; i < helmAppResourceTreeSize; i++ {
			helmAppStatus := helmAppResourceTreeMap[mongoOperatorChartName].ApplicationStatus
			cacheHelmAppStatus := cacheHelmResourceTreeMap[mongoOperatorChartName].ApplicationStatus
			if !reflect.DeepEqual(helmAppStatus, cacheHelmAppStatus) {
				t.Errorf("Application status Before cluster sync = %v, Application status After cluster sync = %v", helmAppStatus, cacheHelmAppStatus)
			}
		}
	})

	t.Run("Replica_count_pods for helm app", func(t *testing.T) {
		if helmAppResourceTreeSize != cacheAppResourceTreeSize {
			t.Errorf("Different Node length")
			return
		}
		helmAppPodCount, cacheHelmAppPodCount := 0, 0
		for i := 0; i < helmAppResourceTreeSize; i++ {
			helmAppKind := helmAppResourceTreeMap[mongoOperatorChartName].ResourceTreeResponse.Nodes[i].Kind
			cacheHelmAppKind := cacheHelmResourceTreeMap[mongoOperatorChartName].ResourceTreeResponse.Nodes[i].Kind
			if helmAppKind == "pod" {
				helmAppPodCount++
			}
			if cacheHelmAppKind == "pod" {
				cacheHelmAppPodCount++
			}
		}
		if helmAppPodCount != cacheHelmAppPodCount {
			t.Errorf("Replica pod count Before cluster sync = %v, Replica pod count After cluster sync = %v", helmAppPodCount, cacheHelmAppPodCount)
		}
	})
}

func GetDbConnAndLoggerService(t *testing.T) (*zap.SugaredLogger, *pg.DB) {
	cfg, _ := sql.GetConfig()
	logger, err := utils.NewSugardLogger()
	assert.Nil(t, err)
	dbConnection, err := sql.NewDbConnection(cfg, logger)
	assert.Nil(t, err)

	return logger, dbConnection
}

func getHelmAppServiceDependencies(t *testing.T) (*zap.SugaredLogger, *k8sInformer2.K8sInformerImpl, *HelmReleaseConfig,
	*k8sUtils.K8sUtil, *repository.ClusterRepositoryImpl, *K8sServiceImpl) {
	logger, dbConnection := GetDbConnAndLoggerService(t)
	helmReleaseConfig := &HelmReleaseConfig{
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
	k8sServiceImpl := NewK8sServiceImpl(logger)
	return logger, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, k8sServiceImpl
}

func prepareResourceTreeMapBeforeClusterSyncForDevtronApp(t *testing.T, resourceTreeMap *map[string]*bean.AppDetail,
	helmAppResourceTreeMap *map[string]*bean.AppDetail, helmAppServiceImpl *HelmAppServiceImpl) {
	devtronAppResp := *resourceTreeMap
	for _, payload := range devtronPayloadArray {
		appDetailReqDev := &client.AppDetailRequest{
			ClusterConfig: payload.ReleaseIdentifier.ClusterConfig,
			Namespace:     payload.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   payload.ReleaseIdentifier.ReleaseName,
		}
		appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReqDev)
		assert.Nil(t, err)

		//store appDetail in the map for corresponding key eg "deployment":appDetail for deployment kind
		if payload == installReleaseReqJobAndCronJob {
			devtronAppResp[commonBean.K8sClusterResourceCronJobKind] = appDetail
		} else if payload == installReleaseReqStatefullset {
			devtronAppResp[commonBean.StatefulSetKind] = appDetail
		} else if payload == installReleaseReqRollout {
			devtronAppResp[commonBean.K8sClusterResourceRolloutKind] = appDetail
		} else {
			devtronAppResp[commonBean.DeploymentKind] = appDetail
		}
	}
	resourceTreeMap = &devtronAppResp

	helmChartResp := *helmAppResourceTreeMap
	// Store appDetails in the map for corresponding chart type
	for _, payload := range helmPayloadArray {
		appDetailHelmReq := &client.AppDetailRequest{
			ClusterConfig: payload.ReleaseIdentifier.ClusterConfig,
			Namespace:     payload.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   payload.ReleaseIdentifier.ReleaseName,
		}
		appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailHelmReq)
		if err != nil {
			logrus.Error("Error in fetching app details")
		}

		//Store App details for particular chart
		helmChartResp[payload.ReleaseIdentifier.ReleaseName] = appDetail
	}
	helmAppResourceTreeMap = &helmChartResp
}

func prepareResourceTreeMapAfterClusterSyncForDevtronApp(t *testing.T, cacheResourceTreeMap *map[string]*bean.AppDetail,
	cacheHelmResourceTreeMap *map[string]*bean.AppDetail, helmAppServiceImpl *HelmAppServiceImpl) {

	cacheDevtronAppResp := *cacheResourceTreeMap
	for _, payload := range devtronPayloadArray {
		appDetailReqDev := &client.AppDetailRequest{
			ClusterConfig: payload.ReleaseIdentifier.ClusterConfig,
			Namespace:     payload.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   payload.ReleaseIdentifier.ReleaseName,
		}
		cacheAppDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReqDev)
		assert.Nil(t, err)
		// Storing cache App Details
		if payload == installReleaseReqJobAndCronJob {
			cacheDevtronAppResp[commonBean.K8sClusterResourceCronJobKind] = cacheAppDetail
		} else if payload == installReleaseReqStatefullset {
			cacheDevtronAppResp[commonBean.StatefulSetKind] = cacheAppDetail
		} else if payload == installReleaseReqRollout {
			cacheDevtronAppResp[commonBean.K8sClusterResourceRolloutKind] = cacheAppDetail
		} else {
			cacheDevtronAppResp[commonBean.DeploymentKind] = cacheAppDetail
		}
	}
	cacheResourceTreeMap = &cacheDevtronAppResp

	cacheHelmChartResp := *cacheHelmResourceTreeMap
	for _, payload := range helmPayloadArray {
		appDetailHelmReq := &client.AppDetailRequest{
			ClusterConfig: payload.ReleaseIdentifier.ClusterConfig,
			Namespace:     payload.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   payload.ReleaseIdentifier.ReleaseName,
		}
		appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailHelmReq)
		if err != nil {
			logrus.Error("Error in fetching app details")
		}
		//Store Cache App details for particular chart
		cacheHelmChartResp[payload.ReleaseIdentifier.ReleaseName] = appDetail
	}
	cacheHelmResourceTreeMap = &cacheHelmChartResp
}

func buildResourceInfoBeforeCacheSync(resourceTreeMap map[string]*bean.AppDetail) map[string]NodeInfo {
	resourceTreeSize := len(resourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.Nodes)
	// Storing Resource data before Cluster sync
	resourceDataBeforeSync := map[string]NodeInfo{}
	for i := 0; i < resourceTreeSize; i++ {
		node := resourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.Nodes[i]
		uid := node.UID
		nodeInfo := NodeInfo{
			kind:              node.Kind,
			health:            node.Health,
			port:              node.Port,
			CreatedAt:         node.CreatedAt,
			IsHibernated:      node.IsHibernated,
			PodMetaData:       resourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.PodMetadata,
			ReleaseStatus:     resourceTreeMap[commonBean.DeploymentKind].ReleaseStatus,
			ApplicationStatus: resourceTreeMap[commonBean.DeploymentKind].ApplicationStatus,
			info:              node.Info,
			NetworkingInfo:    node.NetworkingInfo,
		}
		resourceDataBeforeSync[uid] = nodeInfo
	}
	return resourceDataBeforeSync
}

func buildResourceInfoAfterCacheSync(cacheResourceTreeMap map[string]*bean.AppDetail) map[string]NodeInfo {
	resourceCacheTreeSize := len(cacheResourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.Nodes)

	// Storing Resource data After Cluster Sync
	resourceDataAfterSync := map[string]NodeInfo{}
	for i := 0; i < resourceCacheTreeSize; i++ {
		node := cacheResourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.Nodes[i]
		uid := node.UID
		nodeInfo := NodeInfo{
			kind:              node.Kind,
			health:            node.Health,
			port:              node.Port,
			CreatedAt:         node.CreatedAt,
			IsHibernated:      node.IsHibernated,
			PodMetaData:       cacheResourceTreeMap[commonBean.DeploymentKind].ResourceTreeResponse.PodMetadata,
			ReleaseStatus:     cacheResourceTreeMap[commonBean.DeploymentKind].ReleaseStatus,
			ApplicationStatus: cacheResourceTreeMap[commonBean.DeploymentKind].ApplicationStatus,
			info:              node.Info,
			NetworkingInfo:    node.NetworkingInfo,
		}
		resourceDataAfterSync[uid] = nodeInfo
	}
	return resourceDataAfterSync
}

func setupSuite(t *testing.T) func(t *testing.T) {
	logger, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, k8sServiceImpl := getHelmAppServiceDependencies(t)
	clusterCacheConfig := &cache.ClusterCacheConfig{}
	clusterCacheImpl := cache.NewClusterCacheImpl(logger, clusterCacheConfig, clusterRepository, k8sUtil, k8sInformer)
	helmAppServiceImpl := NewHelmAppServiceImpl(logger, k8sServiceImpl, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, clusterCacheImpl)

	// App creation with different payload for helm app chart
	for _, payload := range helmPayloadArray {
		installReleaseResp, err := helmAppServiceImpl.InstallRelease(context.Background(), payload)
		assert.Nil(t, err)
		fmt.Println(installReleaseResp)
	}

	// App Creation with different payload for devtron apps
	for _, payload := range devtronPayloadArray {
		installReleaseResp, err := helmAppServiceImpl.InstallReleaseWithCustomChart(context.Background(), payload)
		assert.Nil(t, err)
		fmt.Println(installReleaseResp)
	}

	// Return a function to teardown the test
	return func(t *testing.T) {
		for _, payload := range helmPayloadArray {
			releaseIdentifier := &client.ReleaseIdentifier{
				ClusterConfig:    payload.ReleaseIdentifier.ClusterConfig,
				ReleaseNamespace: payload.ReleaseIdentifier.ReleaseNamespace,
				ReleaseName:      payload.ReleaseIdentifier.ReleaseName,
			}
			_, err := helmAppServiceImpl.UninstallRelease(releaseIdentifier)
			assert.Nil(t, err)
		}

		for _, payload := range devtronPayloadArray {
			releaseIdentifier := &client.ReleaseIdentifier{
				ClusterConfig:    payload.ReleaseIdentifier.ClusterConfig,
				ReleaseNamespace: payload.ReleaseIdentifier.ReleaseNamespace,
				ReleaseName:      payload.ReleaseIdentifier.ReleaseName,
			}
			_, err := helmAppServiceImpl.UninstallRelease(releaseIdentifier)
			assert.Nil(t, err)
		}
	}
}

func TestMain(m *testing.M) {
	var t *testing.T
	tearDownSuite := setupSuite(t)
	code := m.Run()
	tearDownSuite(t)
	os.Exit(code)
}
