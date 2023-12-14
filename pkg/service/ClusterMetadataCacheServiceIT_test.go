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
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"os"
	"reflect"
	"testing"
)

var clusterConfig = &client.ClusterConfig{
	ApiServerUrl:          "https://shared-aks-shared-rg-d1e22f-9a9cb33h.hcp.eastus.azmk8s.io:443",
	Token:                 "",
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
		ReleaseNamespace: "devtroncd",
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
		ReleaseNamespace: "devtroncd",
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
		ReleaseNamespace: "devtroncd",
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
	var cacheResourceTreeMap = map[string]*bean.AppDetail{}
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

		//store appDetail in the map for corres. key eg "deployment":appDetail for deployment kind
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

	helmAppDetailMongo, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
	cacheResourceTreeMap["cacheResource"] = helmAppDetailMongo

	appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
	if err != nil {
		logger.Errorw("App details not fetched successfully", err)
	}
	fmt.Println("App Details ", appDetail)
	assert.Nil(t, err)
	model, err := clusterRepository.FindById(int(installReleaseReq.ReleaseIdentifier.ClusterConfig.ClusterId))
	assert.Nil(t, err)
	clusterInfo := k8sInformer2.GetClusterInfo(model)
	clusterCacheImpl.SyncClusterCache(clusterInfo)
	clusterCacheAppDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
	assert.Nil(t, err)
	fmt.Println("Cluster cache App Details ", clusterCacheAppDetail)
	resourceTreeSize := len(clusterCacheAppDetail.ResourceTreeResponse.Nodes)
	// Deployment kind test cases

	// Sts kind test cases
	t.Run("Test for RollOut type", func(t *testing.T) {
		appDetailReq := &client.AppDetailRequest{
			ClusterConfig: installReleaseReqStatefullset.ReleaseIdentifier.ClusterConfig,
			Namespace:     installReleaseReqStatefullset.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   installReleaseReqStatefullset.ReleaseIdentifier.ReleaseName,
		}
		appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
		assert.Nil(t, err)
		fmt.Println("App details for Deployment ", appDetail)
		RollOutResourceVals := resourceTreeMap["RollOut"].ResourceTreeResponse.Nodes
		CacheResourceVals := cacheResourceTreeMap["cacheResource"].ResourceTreeResponse.Nodes
		fmt.Println("RollOut Resource Value ", RollOutResourceVals)
		fmt.Println("Cache Resource Value", CacheResourceVals)
	})

	// Job-Cronjob kind test cases
	t.Run("Test for RollOut type", func(t *testing.T) {
		appDetailReq := &client.AppDetailRequest{
			ClusterConfig: installReleaseReqJobAndCronJob.ReleaseIdentifier.ClusterConfig,
			Namespace:     installReleaseReqJobAndCronJob.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   installReleaseReqJobAndCronJob.ReleaseIdentifier.ReleaseName,
		}
		appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
		assert.Nil(t, err)
		fmt.Println("App details for Deployment ", appDetail)
	})

	// Health Status for Pod and other resources
	t.Run("Status of pod and other resources", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			healthStatus := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Health
			cacheHealthStatus := clusterCacheAppDetail.ResourceTreeResponse.Nodes[i].Health
			if healthStatus != cacheHealthStatus {
				t.Errorf("Health status for pod and resources are not valid")
			}
		}
	})

	//Port number comparision for Service, Endpoints and Endpointslice
	t.Run("Service, Endpoints and EndpointSlice with port numbers", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			kindTypeDeployment := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			kindTypeCache := clusterCacheAppDetail.ResourceTreeResponse.Nodes[i].Kind
			if (kindTypeDeployment == "Service" || kindTypeDeployment == "EndpointSlice" || kindTypeDeployment == "Endpoints") && kindTypeDeployment == kindTypeCache {
				portDeployment := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Port
				portCache := clusterCacheAppDetail.ResourceTreeResponse.Nodes[i].Port
				if !reflect.DeepEqual(portDeployment, portCache) {
					t.Errorf("Response body does not contain the respective ports")
				}
			}
		}
	})

	// Validation for NetworkingInfo
	t.Run("labels for NetworkingInfo", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentNetworkingInfo := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].NetworkingInfo
			cacheNetworkingInfo := clusterCacheAppDetail.ResourceTreeResponse.Nodes[i].NetworkingInfo
			if !reflect.DeepEqual(deploymentNetworkingInfo, cacheNetworkingInfo) {
				t.Errorf("Networking Info for deployment and cluster cache are different")
			}
		}
	})

	// Pod age validation
	t.Run("Check age of pod after changes", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentPodAge := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].CreatedAt
			cachePodAge := clusterCacheAppDetail.ResourceTreeResponse.Nodes[i].CreatedAt
			if deploymentPodAge != cachePodAge {
				t.Errorf("Pod age are different")
			}
		}
	})

	// CanBeHibernated
	t.Run("Check hibernation ", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentHibernated := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].CanBeHibernated
			cacheHibernated := clusterCacheAppDetail.ResourceTreeResponse.Nodes[i].CanBeHibernated
			if deploymentHibernated != cacheHibernated {
				t.Errorf("")
			}
		}
	})

	//Pod Meta Data
	t.Run("PodMeta data", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentPodMetaData := resourceTreeMap["Deployment"].ResourceTreeResponse.PodMetadata
			cachePodMetaData := clusterCacheAppDetail.ResourceTreeResponse.PodMetadata
			if !reflect.DeepEqual(deploymentPodMetaData, cachePodMetaData) {
				t.Errorf("Pod Meta data are different")
			}
		}
	})

	// Release status
	t.Run("Release status", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentReleaseStatus := resourceTreeMap["Deployment"].ReleaseStatus
			cacheReleaseStatus := clusterCacheAppDetail.ReleaseStatus
			if !reflect.DeepEqual(deploymentReleaseStatus, cacheReleaseStatus) {
				t.Errorf("Release status is different for ")
			}
		}
	})

	//Application Status
	t.Run("Application status", func(t *testing.T) {
		for i := 0; i < resourceTreeSize; i++ {
			deploymentAppStatus := resourceTreeMap["Deployment"].ApplicationStatus
			cacheAppStatus := clusterCacheAppDetail.ApplicationStatus
			if deploymentAppStatus != cacheAppStatus {
				t.Errorf("Application status are not same as in cache")
			}
		}
	})

	// Count ReplicaSets
	t.Run("ReplicaSet count", func(t *testing.T) {
		deploymentReplicaCount := 0
		cacheReplicaCount := 0
		for i := 0; i < resourceTreeSize; i++ {
			deploymentKind := resourceTreeMap["Deployment"].ResourceTreeResponse.Nodes[i].Kind
			cacheReplicaKind := clusterCacheAppDetail.ResourceTreeResponse.Nodes[i].Kind
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
