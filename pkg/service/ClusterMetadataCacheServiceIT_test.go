package service

import (
	"context"
	"fmt"
	client2 "github.com/devtron-labs/authenticator/client"
	"github.com/devtron-labs/common-lib/utils"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/cache"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	k8sInformer2 "github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/go-pg/pg"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"os"
	"testing"
)

var clusterConfig = &client.ClusterConfig{
	ApiServerUrl:          "https://4.246.166.113:16443",
	Token:                 "d2JMZUwzOHZZOUlaMzdRb2tqT2liK3RNSVVManYyTXpvU0YvRXZoRHRGRT0K",
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
		ReleaseNamespace: "",
	},
	ValuesYaml: statefullSetYamlValue,
	ChartContent: &client.ChartContent{
		Content: statefullsetsChartContent,
	},
	RunInCtx: false,
}

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

	appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
	fmt.Println("App Details ", appDetail)
	assert.Nil(t, err)
	model, err := clusterRepository.FindById(int(installReleaseReq.ReleaseIdentifier.ClusterConfig.ClusterId))
	assert.Nil(t, err)
	clusterInfo := k8sInformer2.GetClusterInfo(model)
	clusterCacheImpl.SyncClusterCache(clusterInfo)
	clusterCacheAppDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
	assert.Nil(t, err)
	fmt.Println("Cluster cache App Details ", clusterCacheAppDetail)
	// Deployment kind test cases
	t.Run("Test for Deployment Type ", func(t *testing.T) {
		appDetailReq := &client.AppDetailRequest{
			ClusterConfig: installReleaseReqDeployment.ReleaseIdentifier.ClusterConfig,
			Namespace:     installReleaseReqDeployment.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   installReleaseReqDeployment.ReleaseIdentifier.ReleaseName,
		}
		appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
		assert.Nil(t, err)
		fmt.Println("App details for Deployment ", appDetail)
	})

	// RollOut kind test cases
	t.Run("Test for RollOut type", func(t *testing.T) {
		appDetailReq := &client.AppDetailRequest{
			ClusterConfig: installReleaseReqRollout.ReleaseIdentifier.ClusterConfig,
			Namespace:     installReleaseReqRollout.ReleaseIdentifier.ReleaseNamespace,
			ReleaseName:   installReleaseReqRollout.ReleaseIdentifier.ReleaseName,
		}
		appDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
		assert.Nil(t, err)
		fmt.Println("App details for Deployment ", appDetail)
	})

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

	// Common test cases for all kind

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
	//installedReleaseResp, err := helmAppServiceImpl.InstallRelease(context.Background(), installReleaseReq)
	//if err != nil {
	//	logger.Errorw("chart not installed successfully")
	//}
	installedReleaseResp, err := helmAppServiceImpl.InstallReleaseWithCustomChart(context.Background(), installReleaseReqJobAndCronJob)
	if err != nil {
		logger.Errorw("chart not installed successfully")
	}
	fmt.Println(installedReleaseResp)
	//if installedReleaseResp != nil && installedReleaseResp.Success == true {
	//	logger.Infow("chart installed successfully")
	//}
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
