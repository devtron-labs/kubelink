package service

import (
	"context"
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

var installReleaseReq = &client.InstallReleaseRequest{
	ReleaseIdentifier: &client.ReleaseIdentifier{
		ClusterConfig: &client.ClusterConfig{
			ApiServerUrl:          "https://4.246.166.113:16443",
			Token:                 "d2JMZUwzOHZZOUlaMzdRb2tqT2liK3RNSVVManYyTXpvU0YvRXZoRHRGRT0K",
			ClusterId:             1,
			ClusterName:           "default_cluster",
			InsecureSkipTLSVerify: true,
			KeyData:               "",
			CertData:              "",
			CaData:                "",
		},
		ReleaseName:      "mongo-operator",
		ReleaseNamespace: "ns1",
	},
	ChartName:    "community-operator",
	ChartVersion: "0.8.3",
	ValuesYaml:   "## Reference to one or more secrets to be used when pulling images\n## ref: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/\nimagePullSecrets: []\n# - name: \"image-pull-secret\"\n## Operator\noperator:\n  # Name that will be assigned to most of internal Kubernetes objects like\n  # Deployment, ServiceAccount, Role etc.\n  name: mongodb-kubernetes-operator\n\n  # Name of the operator image\n  operatorImageName: mongodb-kubernetes-operator\n\n  # Name of the deployment of the operator pod\n  deploymentName: mongodb-kubernetes-operator\n\n  # Version of mongodb-kubernetes-operator\n  version: 0.8.3\n\n  # Uncomment this line to watch all namespaces\n  # watchNamespace: \"*\"\n\n  # Resources allocated to Operator Pod\n  resources:\n    limits:\n      cpu: 1100m\n      memory: 1Gi\n    requests:\n      cpu: 500m\n      memory: 200Mi\n\n  # replicas deployed for the operator pod. Running 1 is optimal and suggested.\n  replicas: 1\n\n  # Additional environment variables\n  extraEnvs: []\n  # environment:\n  # - name: CLUSTER_DOMAIN\n  #   value: my-cluster.domain\n\n  podSecurityContext:\n    runAsNonRoot: true\n    runAsUser: 2000\n\n  securityContext: {}\n\n## Operator's database\ndatabase:\n  name: mongodb-database\n  # set this to the namespace where you would like\n  # to deploy the MongoDB database,\n  # Note if the database namespace is not same\n  # as the operator namespace,\n  # make sure to set \"watchNamespace\" to \"*\"\n  # to ensure that the operator has the\n  # permission to reconcile resources in other namespaces\n  # namespace: mongodb-database\n\nagent:\n  name: mongodb-agent\n  version: 12.0.25.7724-1\nversionUpgradeHook:\n  name: mongodb-kubernetes-operator-version-upgrade-post-start-hook\n  version: 1.0.8\nreadinessProbe:\n  name: mongodb-kubernetes-readinessprobe\n  version: 1.0.17\nmongodb:\n  name: mongo\n  repo: docker.io\n\nregistry:\n  agent: quay.io/mongodb\n  versionUpgradeHook: quay.io/mongodb\n  readinessProbe: quay.io/mongodb\n  operator: quay.io/mongodb\n  pullPolicy: Always\n\n# Set to false if CRDs have been installed already. The CRDs can be installed\n# manually from the code repo: github.com/mongodb/mongodb-kubernetes-operator or\n# using the `community-operator-crds` Helm chart.\ncommunity-operator-crds:\n  enabled: true\n\n# Deploys MongoDB with `resource` attributes.\ncreateResource: false\nresource:\n  name: mongodb-replica-set\n  version: 4.4.0\n  members: 3\n  tls:\n    enabled: false\n\n    # Installs Cert-Manager in this cluster.\n    useX509: false\n    sampleX509User: false\n    useCertManager: true\n    certificateKeySecretRef: tls-certificate\n    caCertificateSecretRef: tls-ca-key-pair\n    certManager:\n      certDuration: 8760h   # 365 days\n      renewCertBefore: 720h   # 30 days\n\n  users: []\n  # if using the MongoDBCommunity Resource, list any users to be added to the resource\n  # users:\n  # - name: my-user\n  #   db: admin\n  #   passwordSecretRef: # a reference to the secret that will be used to generate the user's password\n  #     name: <secretName>\n  #   roles:\n  #     - name: clusterAdmin\n  #       db: admin\n  #     - name: userAdminAnyDatabase\n  #       db: admin\n  #     - name: readWriteAnyDatabase\n  #       db: admin\n  #     - name: dbAdminAnyDatabase\n  #       db: admin\n  #   scramCredentialsSecretName: my-scram\n",
	ChartRepository: &client.ChartRepository{
		Name:     "mongodb",
		Url:      "https://mongodb.github.io/helm-charts",
		Username: "",
		Password: "",
	},
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
	assert.Nil(t, err)
	model, err := clusterRepository.FindById(int(installReleaseReq.ReleaseIdentifier.ClusterConfig.ClusterId))
	assert.Nil(t, err)
	clusterInfo := k8sInformer2.GetClusterInfo(model)
	clusterCacheImpl.SyncClusterCache(clusterInfo)
	clusterCacheAppDetail, err := helmAppServiceImpl.BuildAppDetail(appDetailReq)
	assert.Nil(t, err)
	t.Run("Test1 FetchBuildAppDetail without cluster cache", func(t *testing.T) {
		if appDetail.ReleaseStatus != nil && clusterCacheAppDetail.ReleaseStatus != nil {
			if appDetail.ReleaseStatus.Status != clusterCacheAppDetail.ReleaseStatus.Status {
				//some issue
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
	installedReleaseResp, err := helmAppServiceImpl.InstallRelease(context.Background(), installReleaseReq)
	if err != nil {
		logger.Errorw("chart not installed successfully")
	}
	if installedReleaseResp != nil && installedReleaseResp.Success == true {
		logger.Infow("chart installed successfully")
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
