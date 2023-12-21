package service

import (
	"context"
	"fmt"
	client2 "github.com/devtron-labs/authenticator/client"
	"github.com/devtron-labs/common-lib/utils"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/cache"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	k8sInformer2 "github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/go-pg/pg"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"os"
	"reflect"
	"testing"
)

var clusterConfig = &client.ClusterConfig{
	ApiServerUrl:          "https://20.232.141.127:16443",
	Token:                 "dmVlcHI2NkpOckQrSVBnQWduSHlqRENjWHFIVXkrckdtQStVajZtaXBhMD0K",
	ClusterId:             1,
	ClusterName:           "default_cluster",
	InsecureSkipTLSVerify: true,
}

var installReleaseReq = &client.InstallReleaseRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      "mongo-operator",
		ReleaseNamespace: "ns1",
	},
	ChartName:    "community-operator",
	ChartVersion: "0.8.3",
	ValuesYaml:   installReleaseReqYamlValue,
	ChartRepository: &client.ChartRepository{
		Name:     "mongodb",
		Url:      "https://mongodb.github.io/helm-charts",
		Username: "",
		Password: "",
	},
}

var installReleaseReqJobAndCronJob = &client.HelmInstallCustomRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      "cache-test-cronjob-devtron-demo",
		ReleaseNamespace: "devtron-demo",
	},
	ValuesYaml: cronJobYamlValue,
	ChartContent: &client.ChartContent{
		Content: cronJobRefChart,
	},
	RunInCtx: false,
}

var installReleaseReqDeployment = &client.HelmInstallCustomRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      "testing-deployment-devtron-demo",
		ReleaseNamespace: "devtron-demo",
	},
	ValuesYaml: deploymentYamlvalue,
	ChartContent: &client.ChartContent{
		Content: deploymentRefChart,
	},
	RunInCtx: false,
}

var installReleaseReqRollout = &client.HelmInstallCustomRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      "cache-test-01-default-cluster--devtroncd",
		ReleaseNamespace: "devtron-demo",
	},
	ValuesYaml: rollOutYamlValue,
	ChartContent: &client.ChartContent{
		Content: rolloutDeploymentCharContent,
	},
	RunInCtx: true,
}

var installReleaseReqStatefullset = &client.HelmInstallCustomRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig:    clusterConfig,
		ReleaseName:      "cache-test-stateful-devtron-demo",
		ReleaseNamespace: "devtron-demo",
	},
	ValuesYaml: statefullSetYamlValue,
	ChartContent: &client.ChartContent{
		Content: statefullsetsChartContent,
	},
	RunInCtx: false,
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
	for _, payload := range devtronPayloadArray {
		appDetailReqDev := &client.AppDetailRequest{
			ClusterConfig: payload.ReleaseIdentifier.ClusterConfig,
			Namespace:     payload.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   payload.ReleaseIdentifier.ReleaseName,
		}
		appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReqDev)
		if err != nil {
			logger.Errorw("App Details not build successfully", err)
		}

		//store appDetail in the map for corresponding key eg "deployment":appDetail for deployment kind
		if payload == installReleaseReqJobAndCronJob {
			resourceTreeMap["JobAndCronJob"] = appDetail
		} else if payload == installReleaseReqStatefullset {
			resourceTreeMap["StatefulSets"] = appDetail
		} else if payload == installReleaseReqRollout {
			resourceTreeMap["RollOut"] = appDetail
		} else {
			resourceTreeMap["Deployment"] = appDetail
		}
	}

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
		helmAppResourceTreeMap[payload.ReleaseIdentifier.ReleaseName] = appDetail
	}

	helmAppResourceTreeSize := len(helmAppResourceTreeMap["mongo-operator"].ResourceTreeResponse.Nodes)

	resourceTreeSize := len(resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes)
	resourceTreeSizeStatefulSets := len(resourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes)

	// Storing Resource data before Cluster sync
	resourceDataBeforeSync := map[string]NodeInfo{}
	for i := 0; i < resourceTreeSize; i++ {
		uid := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].UID
		nodeInfo := NodeInfo{
			kind:              resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind,
			health:            resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Health,
			port:              resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Port,
			CreatedAt:         resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].CreatedAt,
			IsHibernated:      resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].IsHibernated,
			PodMetaData:       resourceTreeMap["Deployment"].ResourceTreeResponse.PodMetadata,
			ReleaseStatus:     resourceTreeMap["Deployment"].ReleaseStatus,
			ApplicationStatus: resourceTreeMap["Deployment"].ApplicationStatus,
			info:              resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Info,
			NetworkingInfo:    resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].NetworkingInfo,
		}
		resourceDataBeforeSync[uid] = nodeInfo
	}

	model, err := clusterRepository.FindById(int(installReleaseReq.ReleaseIdentifier.ClusterConfig.ClusterId))
	assert.Nil(t, err)
	clusterInfo := k8sInformer2.GetClusterInfo(model)

	clusterCacheImpl.SyncClusterCache(clusterInfo)

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
			cacheResourceTreeMap["JobAndCronJob"] = cacheAppDetail
		} else if payload == installReleaseReqStatefullset {
			cacheResourceTreeMap["StatefulSets"] = cacheAppDetail
		} else if payload == installReleaseReqRollout {
			cacheResourceTreeMap["RollOut"] = cacheAppDetail
		} else {
			cacheResourceTreeMap["Deployment"] = cacheAppDetail
		}
	}
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
		cacheHelmResourceTreeMap[payload.ReleaseIdentifier.ReleaseName] = appDetail
	}

	resourceCacheTreeSize := len(cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes)
	resourceCacheTreeSizeStatefulSets := len(cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes)

	// Storing Resource data After Cluster Sync
	resourceDataAfterSync := map[string]NodeInfo{}
	for i := 0; i < resourceCacheTreeSize; i++ {
		uid := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].UID
		nodeInfo := NodeInfo{
			kind:              cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind,
			health:            cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Health,
			port:              cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Port,
			CreatedAt:         cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].CreatedAt,
			IsHibernated:      cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].IsHibernated,
			PodMetaData:       cacheResourceTreeMap["Deployment"].ResourceTreeResponse.PodMetadata,
			ReleaseStatus:     cacheResourceTreeMap["Deployment"].ReleaseStatus,
			ApplicationStatus: cacheResourceTreeMap["Deployment"].ApplicationStatus,
			info:              cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Info,
			NetworkingInfo:    cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].NetworkingInfo,
		}
		resourceDataAfterSync[uid] = nodeInfo
	}

	for i := 0; i < resourceCacheTreeSizeStatefulSets; i++ {
		uid := cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].UID
		nodeInfo := NodeInfo{
			kind:              cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].Kind,
			health:            cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].Health,
			port:              cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].Port,
			CreatedAt:         cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].CreatedAt,
			IsHibernated:      cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].IsHibernated,
			PodMetaData:       cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.PodMetadata,
			ReleaseStatus:     cacheResourceTreeMap["StatefulSets"].ReleaseStatus,
			ApplicationStatus: cacheResourceTreeMap["StatefulSets"].ApplicationStatus,
			info:              cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].Info,
			NetworkingInfo:    cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].NetworkingInfo,
		}
		resourceDataAfterSync[uid] = nodeInfo
	}

	//Health Status for Pod and other resources
	t.Run("Status_of_pod and other resources", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for uid, _ := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				if !reflect.DeepEqual(resourceDataBeforeSync[uid].health, nodeInfoAfterSync.health) {
					t.Errorf("Status_of_pod , status_b:= Node status is different")
				}
			} else {
				// this is tests fail case hence throw error here
			}
		}
	})

	//Port number comparison for Service, Endpoints and EndpointSlice
	t.Run("Service, Endpoints and EndpointSlice with port numbers", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		for uid, _ := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				if (resourceDataBeforeSync[uid].kind == "Service" || resourceDataBeforeSync[uid].kind == "EndpointSlice" || resourceDataBeforeSync[uid].kind == "Endpoints") && resourceDataBeforeSync[uid].kind == nodeInfoAfterSync.kind {
					if !reflect.DeepEqual(resourceDataBeforeSync[uid].port, nodeInfoAfterSync.port) {
						t.Errorf("Port list is different")
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
		for uid := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				deploymentNetworkingInfo := nodeInfoAfterSync.NetworkingInfo
				cacheNetworkingInfo := resourceDataBeforeSync[uid].NetworkingInfo
				if !reflect.DeepEqual(deploymentNetworkingInfo.Labels, cacheNetworkingInfo.Labels) {
					t.Errorf("Networking Info are different")
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
		for uid := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				deploymentKind := resourceDataBeforeSync[uid].kind
				cacheKind := nodeInfoAfterSync.kind
				if deploymentKind == "pod" && deploymentKind == cacheKind {
					if !reflect.DeepEqual(resourceDataBeforeSync[uid].CreatedAt, nodeInfoAfterSync.CreatedAt) {
						t.Errorf("Pod Age is Different")
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
		for uid := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				if resourceDataBeforeSync[uid].IsHibernated != nodeInfoAfterSync.IsHibernated {
					t.Errorf("Hibernation status is different")
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
		for uid := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				deploymentPodMetaData := resourceDataBeforeSync[uid].PodMetaData
				cachePodMetaData := nodeInfoAfterSync.PodMetaData
				if !reflect.DeepEqual(deploymentPodMetaData, cachePodMetaData) {
					t.Errorf("Pod Meta data is different")
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
			deploymentReleaseStatus := resourceTreeMap["Deployment"].ReleaseStatus
			cacheReleaseStatus := cacheResourceTreeMap["Deployment"].ReleaseStatus
			if !reflect.DeepEqual(deploymentReleaseStatus, cacheReleaseStatus) {
				t.Errorf("Release status is different for")
			}
		}
	})

	//Application Status
	t.Run("Application status", func(t *testing.T) {
		if resourceTreeSize != len(resourceDataBeforeSync) {
			t.Errorf("Different Node length")
			return
		}
		deploymentAppStatus := *resourceTreeMap["Deployment"].ApplicationStatus
		cacheAppStatus := *cacheResourceTreeMap["Deployment"].ApplicationStatus
		if deploymentAppStatus != cacheAppStatus {
			t.Errorf("Application status are not same as in cache")
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
		for i := 0; i < resourceTreeSize; i++ {
			deploymentKind := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				deploymentReplicaCount++
			}
			if cacheReplicaKind == "pod" {
				cacheReplicaCount++
			}
		}
		if deploymentReplicaCount != cacheReplicaCount {
			t.Errorf("Different Replica count pod")
		}

		// For StatefulSet deployment chart
		statefulSetReplicaCount, cacheStatefulSetReplicaCount := 0, 0
		for i := 0; i < len(resourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes); i++ {
			deploymentKind := resourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				statefulSetReplicaCount++
			}
			if cacheReplicaKind == "pod" {
				cacheStatefulSetReplicaCount++
			}
		}
		if statefulSetReplicaCount != cacheStatefulSetReplicaCount {
			t.Errorf("Different Replica count pod")
		}

		// For RollOut type chart
		rollOutReplicaCount, cacheRollOutCount := 0, 0
		for i := 0; i < len(resourceTreeMap["RollOut"].ResourceTreeResponse.Nodes); i++ {
			deploymentKind := resourceTreeMap["RollOut"].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := cacheResourceTreeMap["RollOut"].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				rollOutReplicaCount++
			}
			if cacheReplicaKind == "pod" {
				cacheRollOutCount++
			}
		}
		if rollOutReplicaCount != cacheRollOutCount {
			t.Errorf("Different Replica count pod")
		}

		// For Job and Cronjob
		cronJobReplicaCount, cacheCronJobCount := 0, 0
		for i := 0; i < len(resourceTreeMap["JobAndCronJob"].ResourceTreeResponse.Nodes); i++ {
			deploymentKind := resourceTreeMap["JobAndCronJob"].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := cacheResourceTreeMap["JobAndCronJob"].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				cronJobReplicaCount++
			}
			if cacheReplicaKind == "pod" {
				cacheCronJobCount++
			}
		}
		if cronJobReplicaCount != cacheCronJobCount {
			t.Errorf("Different Replica count pod")
		}

	})

	// Restart Count
	t.Run("Restart count for a pod", func(t *testing.T) {
		for uid := range resourceDataBeforeSync {
			if nodeInfoAfterSync, ok := resourceDataAfterSync[uid]; ok {
				if resourceDataBeforeSync[uid].kind == "pod" {
					deploymentRestart := resourceDataBeforeSync[uid].info
					cacheRestart := nodeInfoAfterSync.info
					for i := 0; i < len(deploymentRestart); i++ {
						if deploymentRestart[i].Name == "Restart Count" {
							if deploymentRestart[i].Value != cacheRestart[i].Value {
								t.Errorf("Restart count is different")
							}
						}
					}
				}
			}
		}
	})

	// Count of pods
	t.Run("ReplicaCount as > 1, which is the Count of pods ready.", func(t *testing.T) {
		deploymentPodCount, cachePodCount := 0, 0
		for i := 0; i < resourceTreeSize; i++ {
			deploymentKind := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			cacheKind := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" {
				deploymentPodCount++
			}
			if cacheKind == "pod" {
				cachePodCount++
			}
		}
		if cachePodCount != deploymentPodCount {
			t.Errorf("Ready pod count is different")
		}
	})

	// PersistentVolumeClaim for StatefulSets Deployment
	t.Run("Persistence volume", func(t *testing.T) {
		isPVCDeployment, isPVCCache := false, false
		for i := 0; i < resourceTreeSizeStatefulSets; i++ {
			deploymentKind := resourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].Kind
			cacheKind := cacheResourceTreeMap["StatefulSets"].ResourceTreeResponse.Nodes[i].Kind
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
		deploymentEphemeralContainer := resourceTreeMap["Deployment"].ResourceTreeResponse.PodMetadata
		cacheEphemeralContainer := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.PodMetadata
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
		var newPodData []string
		var cacheNewPodData []string

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
	t.Run("Number_of_Replica_Pod for helm Apps ", func(t *testing.T) {
		helmAppMap := map[string]*bean.ResourceNetworkingInfo{}
		cacheHelmMap := map[string]*bean.ResourceNetworkingInfo{}
		for i := 0; i < helmAppResourceTreeSize; i++ {
			uid := helmAppResourceTreeMap["mongo-operator"].ResourceTreeResponse.Nodes[i].UID
			helmAppMap[uid] = helmAppResourceTreeMap["mongo-operator"].ResourceTreeResponse.Nodes[i].NetworkingInfo
			cacheHelmMap[uid] = cacheHelmResourceTreeMap["mongo-operator"].ResourceTreeResponse.Nodes[i].NetworkingInfo
		}
		for uid := range helmAppMap {
			if cacheHelmData, ok := cacheHelmMap[uid]; ok {
				helmAppNetworkingInfo := helmAppMap[uid].Labels
				cacheHelmAppNetworkingInfo := cacheHelmData.Labels
				if !reflect.DeepEqual(helmAppNetworkingInfo, cacheHelmAppNetworkingInfo) {
					t.Errorf("Networking Info are different")
				}
			}
		}
	})

	t.Run("Release_status for helm app", func(t *testing.T) {
		for i := 0; i < helmAppResourceTreeSize; i++ {
			helmReleaseStatus := helmAppResourceTreeMap["mongo-operator"].ReleaseStatus
			cacheHelmReleaseStatus := cacheHelmResourceTreeMap["mongo-operator"].ReleaseStatus
			if !reflect.DeepEqual(helmReleaseStatus, cacheHelmReleaseStatus) {
				t.Errorf("Release status are different")
			}
		}
	})

	t.Run("App_status for helm app", func(t *testing.T) {
		for i := 0; i < helmAppResourceTreeSize; i++ {
			helmAppStatus := helmAppResourceTreeMap["mongo-operator"].ApplicationStatus
			cacheHelmAppStatus := cacheHelmResourceTreeMap["mongo-operator"].ApplicationStatus
			if !reflect.DeepEqual(helmAppStatus, cacheHelmAppStatus) {
				t.Errorf("Application status are different")
			}
		}
	})

	t.Run("Replica_count_pods for helm app", func(t *testing.T) {
		helmAppPodCount, cacheHelmAppPodCount := 0, 0
		for i := 0; i < helmAppResourceTreeSize; i++ {
			helmAppKind := helmAppResourceTreeMap["mongo-operator"].ResourceTreeResponse.Nodes[i].Kind
			cacheHelmAppKind := cacheResourceTreeMap["mongo-operator"].ResourceTreeResponse.Nodes[i].Kind
			if helmAppKind == "pod" {
				helmAppPodCount++
			}
			if cacheHelmAppKind == "pod" {
				cacheHelmAppPodCount++
			}
		}
		if helmAppPodCount != cacheHelmAppPodCount {
			t.Errorf("Replica pod count is different")
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

func setupSuite(t *testing.T) func(t *testing.T) {
	logger, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, k8sServiceImpl := getHelmAppServiceDependencies(t)
	clusterCacheConfig := &cache.ClusterCacheConfig{}
	clusterCacheImpl := cache.NewClusterCacheImpl(logger, clusterCacheConfig, clusterRepository, k8sUtil, k8sInformer)

	helmAppServiceImpl := NewHelmAppServiceImpl(logger, k8sServiceImpl, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, clusterCacheImpl)

	// App creation with different payload for helm app chart
	for _, payload := range helmPayloadArray {
		installReleaseReq, err := helmAppServiceImpl.InstallRelease(context.Background(), payload)
		if err != nil {
			logger.Errorw("Char not installed successfully", err)
		}
		fmt.Println(installReleaseReq)
	}

	// App Creation with different payload for devtron apps
	for _, payload := range devtronPayloadArray {
		installReleaseReq, err := helmAppServiceImpl.InstallReleaseWithCustomChart(context.Background(), payload)
		if err != nil {
			logger.Errorw("Chart not installed successfully", err)
		}
		fmt.Println(installReleaseReq)
	}

	// Return a function to teardown the test
	return func(t *testing.T) {
		releaseIdentfier := &client.ReleaseIdentifier{
			ClusterConfig:    installReleaseReq.ReleaseIdentifier.ClusterConfig,
			ReleaseNamespace: installReleaseReq.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:      installReleaseReq.ReleaseIdentifier.ReleaseName,
		}
		resp, err := helmAppServiceImpl.UninstallRelease(releaseIdentfier)
		if err != nil {
			logger.Errorw("error in uninstalling chart", "releaseName", releaseIdentfier.ReleaseName)
		}
		if resp.Success == true {
			logger.Infow("chart uninstalled successfully")
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
