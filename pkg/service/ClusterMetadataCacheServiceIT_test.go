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
	KeyData:               "",
	CertData:              "",
	CaData:                "",
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

func TestHelmAppService_BuildAppDetail(t *testing.T) {
	logger, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, k8sServiceImpl := getHelmAppServiceDependencies(t)
	clusterCacheConfig := &cache.ClusterCacheConfig{}
	clusterCacheImpl := cache.NewClusterCacheImpl(logger, clusterCacheConfig, clusterRepository, k8sUtil, k8sInformer)
	helmAppServiceImpl := NewHelmAppServiceImpl(logger, k8sServiceImpl, k8sInformer, helmReleaseConfig, k8sUtil, clusterRepository, clusterCacheImpl)
	appDetailReq := &client.AppDetailRequest{
		ClusterConfig: installReleaseReq.ReleaseIdentifier.ClusterConfig,
		Namespace:     installReleaseReq.ReleaseIdentifier.ReleaseNamespace,
		ReleaseName:   installReleaseReq.ReleaseIdentifier.ReleaseName,
	}
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
		} else if payload == installReleaseReqDeployment {
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
		helmAppResourceTreeMap["mongodb"] = appDetail
	}

	helmAppDetailMongo, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
	helmAppResourceTreeMap["helmChartResource"] = helmAppDetailMongo
	if err != nil {
		logger.Errorw("App details for chart Mongo not fetched successfully", err)
	}
	assert.Nil(t, err)
	model, err := clusterRepository.FindById(int(installReleaseReq.ReleaseIdentifier.ClusterConfig.ClusterId))
	assert.Nil(t, err)
	clusterInfo := k8sInformer2.GetClusterInfo(model)
	clusterInfo = &bean.ClusterInfo{
		ClusterId:             1,
		ClusterName:           "default_cluster",
		BearerToken:           "dmVlcHI2NkpOckQrSVBnQWduSHlqRENjWHFIVXkrckdtQStVajZtaXBhMD0K",
		ServerUrl:             "https://20.232.141.127:16443",
		InsecureSkipTLSVerify: true,
		KeyData:               "",
		CertData:              "",
		CAData:                "",
	}
	clusterCacheImpl.SyncClusterCache(clusterInfo)
	clusterCacheAppDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
	for _, payload := range devtronPayloadArray {
		appDetailReqDev := &client.AppDetailRequest{
			ClusterConfig: payload.ReleaseIdentifier.ClusterConfig,
			Namespace:     payload.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   payload.ReleaseIdentifier.ReleaseName,
		}
		cacheAppDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReqDev)
		if err != nil {
			logger.Errorw("App Details not build successfully", err)
		}
		// Storing cache App Details
		if payload == installReleaseReqJobAndCronJob {
			cacheResourceTreeMap["JobAndCronJob"] = cacheAppDetail
		} else if payload == installReleaseReqStatefullset {
			cacheResourceTreeMap["StatefulSets"] = cacheAppDetail
		} else if payload == installReleaseReqDeployment {
			cacheResourceTreeMap["RollOut"] = cacheAppDetail
		} else {
			cacheResourceTreeMap["Deployment"] = cacheAppDetail
		}
	}
	assert.Nil(t, err)
	fmt.Println("Cluster cache App Details ", clusterCacheAppDetail)
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
		cacheHelmResourceTreeMap["mongodb"] = appDetail
	}

	resourceTreeSize := len(clusterCacheAppDetail.ResourceTreeResponse.Nodes)

	// Health Status for Pod and other resources
	t.Run("Status of pod and other resources", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			healthStatus := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Health
			cacheHealthStatus := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Health
			if healthStatus != cacheHealthStatus {
				t.Errorf("Health status for pod and resources are not valid")
			}
		}
	})

	//Port number comparison for Service, Endpoints and EndpointSlice
	t.Run("Service, Endpoints and EndpointSlice with port numbers", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			kindTypeDeployment := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			kindTypeCache := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			if (kindTypeDeployment == "Service" || kindTypeDeployment == "EndpointSlice" || kindTypeDeployment == "Endpoints") && kindTypeDeployment == kindTypeCache {
				portDeployment := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Port
				portCache := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Port
				if !reflect.DeepEqual(portDeployment, portCache) {
					t.Errorf("Response body does not contain the respective ports")
				}
			}
		}
	})

	// Validation for NetworkingInfo
	t.Run("Comparing labels for NetworkingInfo", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentNetworkingInfo := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].NetworkingInfo
			cacheNetworkingInfo := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].NetworkingInfo
			if !reflect.DeepEqual(deploymentNetworkingInfo, cacheNetworkingInfo) {
				t.Errorf("Networking Info for deployment and cluster cache are different")
			}
		}
	})

	// Pod age validation
	t.Run("Check age of pod after changes", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentPodAge := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].CreatedAt
			cachePodAge := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].CreatedAt
			if deploymentPodAge != cachePodAge {
				t.Errorf("Pod age are different")
			}
		}
	})

	// CanBeHibernated
	t.Run("Check hibernation ", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentHibernated := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].CanBeHibernated
			cacheHibernated := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].CanBeHibernated
			if deploymentHibernated != cacheHibernated {
				t.Errorf("")
			}
		}
	})

	// Pod Meta Data
	t.Run("PodMeta data", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentPodMetaData := resourceTreeMap["Deployment"].ResourceTreeResponse.PodMetadata
			cachePodMetaData := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.PodMetadata
			if !reflect.DeepEqual(deploymentPodMetaData, cachePodMetaData) {
				t.Errorf("Pod Meta data are different")
			}
		}
	})

	// Release status
	t.Run("Release status", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentReleaseStatus := resourceTreeMap["Deployment"].ReleaseStatus
			cacheReleaseStatus := cacheResourceTreeMap["Deployment"].ReleaseStatus
			if !reflect.DeepEqual(deploymentReleaseStatus, cacheReleaseStatus) {
				t.Errorf("Release status is different for ")
			}
		}
	})

	//Application Status
	t.Run("Application status", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentAppStatus := resourceTreeMap["Deployment"].ApplicationStatus
			cacheAppStatus := cacheResourceTreeMap["Deployment"].ApplicationStatus
			if deploymentAppStatus != cacheAppStatus {
				t.Errorf("Application status are not same as in cache")
			}
		}
	})

	// Count ReplicaSets
	t.Run("ReplicaSet count", func(t *testing.T) {
		deploymentReplicaCount, cacheReplicaCount := 0, 0
		for i := 0; i < resourceTreeSize; i++ {
			deploymentKind := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "ReplicaSet" {
				deploymentReplicaCount++
			}
			if cacheReplicaKind == "ReplicaSet" {
				cacheReplicaCount++
			}
		}
		if deploymentReplicaCount != cacheReplicaCount {
			t.Errorf("Different Replica count")
		}
	})

	// Restart Count
	t.Run("Restart count for a pod", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentKind := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			cacheKind := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			if deploymentKind == "pod" && cacheKind == deploymentKind {
				deploymentRestart := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Info
				cacheRestart := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Info
				for j := 0; j < len(deploymentRestart); i++ {
					if deploymentRestart[j].Name == "Restart Count" {
						if deploymentRestart[j].Value != cacheRestart[j].Value {
							t.Errorf("Restart count is different")
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
		for i := 0; i < resourceTreeSize; i++ {
			deploymentKind := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			cacheKind := cacheResourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
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
