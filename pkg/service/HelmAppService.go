package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/common-lib/utils/k8s/health"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/clusterCache"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/storage/driver"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"sync"

	"github.com/caarlos0/env"
	"github.com/devtron-labs/common-lib/pubsub-lib"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/common-lib/utils/yaml"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/helmClient"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/util"
	"github.com/devtron-labs/kubelink/pkg/util/argo"
	jsonpatch "github.com/evanphx/json-patch"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"io/ioutil"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	hibernateReplicaAnnotation            = "hibernator.devtron.ai/replicas"
	hibernatePatch                        = `[{"op": "replace", "path": "/spec/replicas", "value":%d}, {"op": "add", "path": "/metadata/annotations", "value": {"%s":"%s"}}]`
	chartWorkingDirectory                 = "/home/devtron/devtroncd/charts/"
	ReadmeFileName                        = "README.md"
	REGISTRY_TYPE_ECR                     = "ecr"
	REGISTRYTYPE_GCR                      = "gcr"
	REGISTRYTYPE_ARTIFACT_REGISTRY        = "artifact-registry"
	JSON_KEY_USERNAME              string = "_json_key"
	HELM_CLIENT_ERROR                     = "Error in creating Helm client"
	RELEASE_INSTALLED                     = "Release Installed"
)

type HelmAppService interface {
	GetApplicationListForCluster(config *client.ClusterConfig) *client.DeployedAppList
	BuildAppDetail(req *client.AppDetailRequest) (*bean.AppDetail, error)
	FetchApplicationStatus(req *client.AppDetailRequest) (*bean.HealthStatusCode, error)
	GetHelmAppValues(req *client.AppDetailRequest) (*client.ReleaseInfo, error)
	ScaleObjects(ctx context.Context, clusterConfig *client.ClusterConfig, requests []*client.ObjectIdentifier, scaleDown bool) (*client.HibernateResponse, error)
	GetDeploymentHistory(req *client.AppDetailRequest) (*client.HelmAppDeploymentHistory, error)
	GetDesiredManifest(req *client.ObjectRequest) (*client.DesiredManifestResponse, error)
	UninstallRelease(releaseIdentifier *client.ReleaseIdentifier) (*client.UninstallReleaseResponse, error)
	UpgradeRelease(ctx context.Context, request *client.UpgradeReleaseRequest) (*client.UpgradeReleaseResponse, error)
	GetDeploymentDetail(request *client.DeploymentDetailRequest) (*client.DeploymentDetailResponse, error)
	InstallRelease(ctx context.Context, request *client.InstallReleaseRequest) (*client.InstallReleaseResponse, error)
	UpgradeReleaseWithChartInfo(ctx context.Context, request *client.InstallReleaseRequest) (*client.UpgradeReleaseResponse, error)
	IsReleaseInstalled(ctx context.Context, releaseIdentifier *client.ReleaseIdentifier) (bool, error)
	RollbackRelease(request *client.RollbackReleaseRequest) (bool, error)
	TemplateChart(ctx context.Context, request *client.InstallReleaseRequest) (string, error)
	InstallReleaseWithCustomChart(req *client.HelmInstallCustomRequest) (bool, error)
	GetNotes(ctx context.Context, installReleaseRequest *client.InstallReleaseRequest) (string, error)
	UpgradeReleaseWithCustomChart(ctx context.Context, request *client.UpgradeReleaseRequest) (bool, error)
	// ValidateOCIRegistryLogin Validates the OCI registry credentials by login
	ValidateOCIRegistryLogin(ctx context.Context, OCIRegistryRequest *client.RegistryCredential) (*client.OCIRegistryResponse, error)
	// ExtractCredentialsForRegistry Takes client.RegistryCredential and extracts credentials for the provided registry details
	ExtractCredentialsForRegistry(registryCredential *client.RegistryCredential) (string, string, error)
	// OCIRegistryLogin Takes client.OCIRegistryRequest and helm client, Performs registry login for the given client session and return err if fails
	OCIRegistryLogin(client *registry.Client, registryCredential *client.RegistryCredential) error
	// PushHelmChartToOCIRegistryRepo Pushes the helm chart to the OCI registry and returns the generated digest and pushedUrl
	PushHelmChartToOCIRegistryRepo(ctx context.Context, OCIRegistryRequest *client.OCIRegistryRequest) (*client.OCIRegistryResponse, error)
}

type HelmReleaseConfig struct {
	EnableHelmReleaseCache    bool `env:"ENABLE_HELM_RELEASE_CACHE" envDefault:"true"`
	MaxCountForHelmRelease    int  `env:"MAX_COUNT_FOR_HELM_RELEASE" envDefault:"20"`
	ManifestFetchBatchSize    int  `env:"MANIFEST_FETCH_BATCH_SIZE" envDefault:"2"`
	RunHelmInstallInAsyncMode bool `env:"RUN_HELM_INSTALL_IN_ASYNC_MODE" envDefault:"false"`
}

func GetHelmReleaseConfig() (*HelmReleaseConfig, error) {
	cfg := &HelmReleaseConfig{}
	err := env.Parse(cfg)
	return cfg, err
}

type HelmAppServiceImpl struct {
	logger            *zap.SugaredLogger
	k8sService        K8sService
	randSource        rand.Source
	K8sInformer       k8sInformer.K8sInformer
	helmReleaseConfig *HelmReleaseConfig
	k8sUtil           *k8sUtils.K8sUtil
	pubsubClient      *pubsub_lib.PubSubClientServiceImpl
	clusterRepository repository.ClusterRepository
	clusterCache      clusterCache.ClusterCache
}

func NewHelmAppServiceImpl(logger *zap.SugaredLogger, k8sService K8sService,
	k8sInformer k8sInformer.K8sInformer, helmReleaseConfig *HelmReleaseConfig,
	k8sUtil *k8sUtils.K8sUtil,
	clusterRepository repository.ClusterRepository,
	clusterCache clusterCache.ClusterCache) *HelmAppServiceImpl {

	var pubsubClient *pubsub_lib.PubSubClientServiceImpl
	if helmReleaseConfig.RunHelmInstallInAsyncMode {
		pubsubClient = pubsub_lib.NewPubSubClientServiceImpl(logger)
	}
	helmAppServiceImpl := &HelmAppServiceImpl{
		logger:            logger,
		k8sService:        k8sService,
		randSource:        rand.NewSource(time.Now().UnixNano()),
		K8sInformer:       k8sInformer,
		helmReleaseConfig: helmReleaseConfig,
		pubsubClient:      pubsubClient,
		k8sUtil:           k8sUtil,
		clusterRepository: clusterRepository,
		clusterCache:      clusterCache,
	}
	err := os.MkdirAll(chartWorkingDirectory, os.ModePerm)
	if err != nil {
		helmAppServiceImpl.logger.Errorw("err in creating dir", "err", err)
	}

	return helmAppServiceImpl
}
func (impl HelmAppServiceImpl) CleanDir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		impl.logger.Warnw("error in deleting dir ", "dir", dir)
	}
}
func (impl HelmAppServiceImpl) GetRandomString() string {
	/* #nosec */
	r1 := rand.New(impl.randSource).Int63()
	return strconv.FormatInt(r1, 10)
}

func (impl *HelmAppServiceImpl) GetApplicationListForCluster(config *client.ClusterConfig) *client.DeployedAppList {
	impl.logger.Debugw("Fetching application list ", "clusterId", config.ClusterId, "clusterName", config.ClusterName)

	deployedApp := &client.DeployedAppList{ClusterId: config.GetClusterId()}
	var deployedApps []*client.DeployedAppDetail

	if impl.helmReleaseConfig.EnableHelmReleaseCache {
		impl.logger.Infow("Fetching helm release using Cache")
		deployedApps = impl.K8sInformer.GetAllReleaseByClusterId(int(config.GetClusterId()))
	} else {
		k8sClusterConfig := GetClusterConfigFromClientBean(config)
		restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
		if err != nil {
			impl.logger.Errorw("Error in building rest config ", "clusterId", config.ClusterId, "err", err)
			deployedApp.Errored = true
			deployedApp.ErrorMsg = err.Error()
			return deployedApp
		}
		opt := &helmClient.RestConfClientOptions{
			Options:    &helmClient.Options{},
			RestConfig: restConfig,
		}

		helmAppClient, err := helmClient.NewClientFromRestConf(opt)
		if err != nil {
			impl.logger.Errorw("Error in building client from rest config ", "clusterId", config.ClusterId, "err", err)
			deployedApp.Errored = true
			deployedApp.ErrorMsg = err.Error()
			return deployedApp
		}

		impl.logger.Debug("Fetching application list from helm")
		releases, err := helmAppClient.ListAllReleases()
		if err != nil {
			impl.logger.Errorw("Error in getting releases list ", "clusterId", config.ClusterId, "err", err)
			deployedApp.Errored = true
			deployedApp.ErrorMsg = err.Error()
			return deployedApp
		}

		for _, items := range releases {
			appDetail := &client.DeployedAppDetail{
				AppId:        util.GetAppId(config.ClusterId, items),
				AppName:      items.Name,
				ChartName:    items.Chart.Name(),
				ChartAvatar:  items.Chart.Metadata.Icon,
				LastDeployed: timestamppb.New(items.Info.LastDeployed.Time),
				EnvironmentDetail: &client.EnvironmentDetails{
					ClusterName: config.ClusterName,
					ClusterId:   config.ClusterId,
					Namespace:   items.Namespace,
				},
			}
			deployedApps = append(deployedApps, appDetail)
		}
	}

	deployedApp.DeployedAppDetail = deployedApps
	return deployedApp
}

func (impl HelmAppServiceImpl) BuildAppDetail(req *client.AppDetailRequest) (*bean.AppDetail, error) {
	helmRelease, err := impl.getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return &bean.AppDetail{ReleaseExists: false}, err
		}
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		return nil, err
	}

	resourceTreeResponse, err := impl.buildResourceTree(req, helmRelease)
	if err != nil {
		impl.logger.Errorw("Error in building resource tree ", "err", err)
		return nil, err
	}

	appDetail := &bean.AppDetail{
		ResourceTreeResponse: resourceTreeResponse,
		ApplicationStatus:    util.BuildAppHealthStatus(resourceTreeResponse.Nodes),
		LastDeployed:         helmRelease.Info.LastDeployed.Time,
		ChartMetadata: &bean.ChartMetadata{
			ChartName:    helmRelease.Chart.Name(),
			ChartVersion: helmRelease.Chart.Metadata.Version,
			Notes:        helmRelease.Info.Notes,
		},
		ReleaseStatus: &bean.ReleaseStatus{
			Status:      string(helmRelease.Info.Status),
			Description: helmRelease.Info.Description,
			Message:     util.GetMessageFromReleaseStatus(helmRelease.Info.Status),
		},
		EnvironmentDetails: &client.EnvironmentDetails{
			ClusterName: req.ClusterConfig.ClusterName,
			ClusterId:   req.ClusterConfig.ClusterId,
			Namespace:   helmRelease.Namespace,
		},
		ReleaseExists: true,
	}

	return appDetail, nil
}

func (impl *HelmAppServiceImpl) FetchApplicationStatus(req *client.AppDetailRequest) (*bean.HealthStatusCode, error) {
	var appStatus *bean.HealthStatusCode
	helmRelease, err := impl.getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		return appStatus, err
	}
	_, healthStatusArray, err := impl.getNodes(req, helmRelease)
	if err != nil {
		impl.logger.Errorw("Error in getting nodes", "err", err, "req", req)
		return appStatus, err
	}
	//getting app status on basis of healthy/non-healthy as this api is used for deployment status
	//in orchestrator and not for app status
	appStatus = util.GetAppStatusOnBasisOfHealthyNonHealthy(healthStatusArray)
	return appStatus, nil
}

func (impl HelmAppServiceImpl) GetHelmAppValues(req *client.AppDetailRequest) (*client.ReleaseInfo, error) {

	helmRelease, err := impl.getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		return nil, err
	}

	if helmRelease == nil {
		err = errors.New("release not found")
		return nil, err
	}

	releaseInfo, err := buildReleaseInfoBasicData(helmRelease)
	if err != nil {
		impl.logger.Errorw("Error in building release info basic data ", "err", err)
		return nil, err
	}

	appDetail := &client.DeployedAppDetail{
		AppId:        util.GetAppId(req.ClusterConfig.ClusterId, helmRelease),
		AppName:      helmRelease.Name,
		ChartName:    helmRelease.Chart.Name(),
		ChartAvatar:  helmRelease.Chart.Metadata.Icon,
		LastDeployed: timestamppb.New(helmRelease.Info.LastDeployed.Time),
		ChartVersion: helmRelease.Chart.Metadata.Version,
		EnvironmentDetail: &client.EnvironmentDetails{
			ClusterName: req.ClusterConfig.ClusterName,
			ClusterId:   req.ClusterConfig.ClusterId,
			Namespace:   helmRelease.Namespace,
		},
	}
	releaseInfo.DeployedAppDetail = appDetail
	return releaseInfo, nil

}

func (impl HelmAppServiceImpl) ScaleObjects(ctx context.Context, clusterConfig *client.ClusterConfig, objects []*client.ObjectIdentifier, scaleDown bool) (*client.HibernateResponse, error) {
	response := &client.HibernateResponse{}
	k8sClusterConfig := GetClusterConfigFromClientBean(clusterConfig)
	conf, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("Error in getting rest config ", "err", err)
		return nil, err
	}

	// iterate through objects
	for _, object := range objects {

		// append object's status in response
		hibernateStatus := &client.HibernateStatus{
			TargetObject: object,
		}
		response.Status = append(response.Status, hibernateStatus)

		// STEP-1 - get live manifest
		gvk := &schema.GroupVersionKind{
			Group:   object.Group,
			Kind:    object.Kind,
			Version: object.Version,
		}
		liveManifest, _, err := impl.k8sService.GetLiveManifest(conf, object.Namespace, gvk, object.Name)
		if err != nil {
			impl.logger.Errorw("Error in getting live manifest ", "err", err)
			hibernateStatus.ErrorMsg = err.Error()
			continue
		}
		if liveManifest == nil {
			hibernateStatus.ErrorMsg = "manifest not found"
			continue
		}
		// STEP-1 ends

		// initialise patch request bean
		patchRequest := &bean.KubernetesResourcePatchRequest{
			Name:      object.Name,
			Namespace: object.Namespace,
			Gvk:       gvk,
			PatchType: string(types.JSONPatchType),
		}

		// STEP-2  for scaleDown - get replicas from live manifest, for scaleUp - get original count from annotation
		// and update patch in bean accordingly
		if scaleDown {
			replicas, found, err := unstructured.NestedInt64(liveManifest.UnstructuredContent(), "spec", "replicas")
			if err != nil {
				hibernateStatus.ErrorMsg = err.Error()
				continue
			}
			if !found {
				hibernateStatus.ErrorMsg = "replicas not found in manifest"
				continue
			}
			if replicas == 0 {
				hibernateStatus.ErrorMsg = "object is already scaled down"
				continue
			}
			patchRequest.Patch = fmt.Sprintf(hibernatePatch, 0, hibernateReplicaAnnotation, strconv.Itoa(int(replicas)))
		} else {
			originalReplicaCount, err := strconv.Atoi(liveManifest.GetAnnotations()[hibernateReplicaAnnotation])
			if err != nil {
				hibernateStatus.ErrorMsg = err.Error()
				continue
			}
			if originalReplicaCount == 0 {
				hibernateStatus.ErrorMsg = "object is already scaled up"
				continue
			}
			patchRequest.Patch = fmt.Sprintf(hibernatePatch, originalReplicaCount, hibernateReplicaAnnotation, "0")
		}
		// STEP-2 ends

		// STEP-3 patch resource
		err = impl.k8sService.PatchResource(context.Background(), conf, patchRequest)
		if err != nil {
			impl.logger.Errorw("Error in patching resource ", "err", err)
			hibernateStatus.ErrorMsg = err.Error()
			continue
		}
		// STEP-3 ends

		// otherwise success true
		hibernateStatus.Success = true
	}

	return response, nil
}

func (impl HelmAppServiceImpl) GetDeploymentHistory(req *client.AppDetailRequest) (*client.HelmAppDeploymentHistory, error) {
	helmReleases, err := impl.getHelmReleaseHistory(req.ClusterConfig, req.Namespace, req.ReleaseName, impl.helmReleaseConfig.MaxCountForHelmRelease)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release history ", "err", err)
		return nil, err
	}
	helmAppDeployments := make([]*client.HelmAppDeploymentDetail, 0, len(helmReleases))
	for _, helmRelease := range helmReleases {
		chartMetadata := helmRelease.Chart.Metadata
		manifests := helmRelease.Manifest
		parsedManifests, err := yamlUtil.SplitYAMLs([]byte(manifests))
		if err != nil {
			return nil, err
		}
		dockerImages, err := util.ExtractAllDockerImages(parsedManifests)
		if err != nil {
			return nil, err
		}
		deploymentDetail := &client.HelmAppDeploymentDetail{
			DeployedAt: timestamppb.New(helmRelease.Info.LastDeployed.Time),
			ChartMetadata: &client.ChartMetadata{
				ChartName:    chartMetadata.Name,
				ChartVersion: chartMetadata.Version,
				Home:         chartMetadata.Home,
				Sources:      chartMetadata.Sources,
				Description:  chartMetadata.Description,
			},
			DockerImages: dockerImages,
			Version:      int32(helmRelease.Version),
		}
		helmAppDeployments = append(helmAppDeployments, deploymentDetail)
	}
	return &client.HelmAppDeploymentHistory{DeploymentHistory: helmAppDeployments}, nil
}

func (impl HelmAppServiceImpl) GetDesiredManifest(req *client.ObjectRequest) (*client.DesiredManifestResponse, error) {
	objectIdentifier := req.ObjectIdentifier
	helmRelease, err := impl.getHelmRelease(req.ClusterConfig, req.ReleaseNamespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		return nil, err
	}

	manifests, err := yamlUtil.SplitYAMLs([]byte(helmRelease.Manifest))
	if err != nil {
		return nil, err
	}

	desiredManifest := ""
	for _, manifest := range manifests {
		gvk := manifest.GroupVersionKind()
		if gvk.Group == objectIdentifier.Group && gvk.Version == objectIdentifier.Version && gvk.Kind == objectIdentifier.Kind && manifest.GetName() == objectIdentifier.Name {
			dataByteArr, err := json.Marshal(manifest.UnstructuredContent())
			if err != nil {
				return nil, err
			}
			desiredManifest = string(dataByteArr)
			break
		}
	}

	desiredManifestResponse := &client.DesiredManifestResponse{
		Manifest: desiredManifest,
	}

	return desiredManifestResponse, nil
}

func (impl HelmAppServiceImpl) UninstallRelease(releaseIdentifier *client.ReleaseIdentifier) (*client.UninstallReleaseResponse, error) {
	helmClient, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		return nil, err
	}

	err = helmClient.UninstallReleaseByName(releaseIdentifier.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in uninstall release ", "err", err)
		return nil, err
	}

	uninstallReleaseResponse := &client.UninstallReleaseResponse{
		Success: true,
	}

	return uninstallReleaseResponse, nil
}

func (impl HelmAppServiceImpl) UpgradeRelease(ctx context.Context, request *client.UpgradeReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	upgradeReleaseResponse := &client.UpgradeReleaseResponse{
		Success: true,
	}
	if request.ChartContent == nil {
		releaseIdentifier := request.ReleaseIdentifier
		helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
		if err != nil {
			return nil, err
		}
		registryClient, err := registry.NewClient()
		if err != nil {
			impl.logger.Errorw(HELM_CLIENT_ERROR, "err", err)
			return nil, err
		}
		if request.IsOCIRepo && request.RegistryCredential != nil && request.RegistryCredential.IsPublic {
			err = impl.OCIRegistryLogin(registryClient, request.RegistryCredential)
			if err != nil {
				return nil, err
			}
		}

		helmRelease, err := impl.getHelmRelease(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace, releaseIdentifier.ReleaseName)
		if err != nil {
			impl.logger.Errorw("Error in getting helm release ", "err", err)
			return nil, err
		}

		updateChartSpec := &helmClient.ChartSpec{
			ReleaseName:    releaseIdentifier.ReleaseName,
			Namespace:      releaseIdentifier.ReleaseNamespace,
			ValuesYaml:     request.ValuesYaml,
			MaxHistory:     int(request.HistoryMax),
			RegistryClient: registryClient,
		}

		impl.logger.Debug("Upgrading release")
		_, err = helmClientObj.UpgradeRelease(context.Background(), helmRelease.Chart, updateChartSpec)
		if err != nil {
			impl.logger.Errorw("Error in upgrade release ", "err", err)
			return nil, err
		}

		return upgradeReleaseResponse, nil
	}
	res, err := impl.UpgradeReleaseWithCustomChart(context.Background(), request)
	if err != nil {
		impl.logger.Errorw("Error in upgrade release ", "err", err)
		return nil, err
	}
	upgradeReleaseResponse.Success = res
	return upgradeReleaseResponse, nil
}

func (impl HelmAppServiceImpl) GetDeploymentDetail(request *client.DeploymentDetailRequest) (*client.DeploymentDetailResponse, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmReleases, err := impl.getHelmReleaseHistory(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace, releaseIdentifier.ReleaseName, impl.helmReleaseConfig.MaxCountForHelmRelease)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release history ", "err", err)
		return nil, err
	}

	resp := &client.DeploymentDetailResponse{}
	for _, helmRelease := range helmReleases {
		if request.DeploymentVersion == int32(helmRelease.Version) {
			releaseInfo, err := buildReleaseInfoBasicData(helmRelease)
			if err != nil {
				impl.logger.Errorw("Error in building release info basic data ", "err", err)
				return nil, err
			}
			resp.Manifest = helmRelease.Manifest
			resp.ValuesYaml = releaseInfo.MergedValues
			break
		}
	}

	return resp, nil
}

func (impl HelmAppServiceImpl) InstallRelease(ctx context.Context, request *client.InstallReleaseRequest) (*client.InstallReleaseResponse, error) {
	// Install release starts
	_, err := impl.installRelease(ctx, request, false)
	if err != nil {
		return nil, err
	}
	// Install release ends
	installReleaseResponse := &client.InstallReleaseResponse{
		Success: true,
	}

	return installReleaseResponse, nil

}

func (impl HelmAppServiceImpl) GetOCIChartName(registryUrl, repoName string) string {
	// helm package expects chart name to be in this format
	chartName := fmt.Sprintf("%s://%s/%s", "oci", registryUrl, repoName)
	return chartName
}

func (impl HelmAppServiceImpl) installRelease(ctx context.Context, request *client.InstallReleaseRequest, dryRun bool) (*release.Release, error) {

	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		return nil, err
	}

	//oci registry client
	registryClient, err := registry.NewClient()
	if err != nil {
		impl.logger.Errorw(HELM_CLIENT_ERROR, "err", err)
		return nil, err
	}
	var chartName string
	switch request.IsOCIRepo {
	case true:
		chartName = impl.GetOCIChartName(request.RegistryCredential.RegistryUrl, request.RegistryCredential.RepoName)
		if request.RegistryCredential != nil && !request.RegistryCredential.IsPublic {
			err = impl.OCIRegistryLogin(registryClient, request.RegistryCredential)
			if err != nil {
				return nil, err
			}
		}
	case false:
		chartRepoRequest := request.ChartRepository
		chartRepoName := chartRepoRequest.Name
		// Add or update chart repo starts
		chartRepo := repo.Entry{
			Name:     chartRepoName,
			URL:      chartRepoRequest.Url,
			Username: chartRepoRequest.Username,
			Password: chartRepoRequest.Password,
			// Since helm 3.6.1 it is necessary to pass 'PassCredentialsAll = true'.
			PassCredentialsAll:    true,
			InsecureSkipTLSverify: true,
		}
		impl.logger.Debug("Adding/Updating Chart repo")
		err = helmClientObj.AddOrUpdateChartRepo(chartRepo)
		if err != nil {
			impl.logger.Errorw("Error in add/update chart repo ", "err", err)
			return nil, err
		}
		chartName = fmt.Sprintf("%s/%s", chartRepoName, request.ChartName)
		// Add or update chart repo ends
	}

	// Install release starts
	chartSpec := &helmClient.ChartSpec{
		ReleaseName:      releaseIdentifier.ReleaseName,
		Namespace:        releaseIdentifier.ReleaseNamespace,
		ValuesYaml:       request.ValuesYaml,
		ChartName:        chartName,
		Version:          request.ChartVersion,
		DependencyUpdate: true,
		UpgradeCRDs:      true,
		CreateNamespace:  true,
		DryRun:           dryRun,
		RegistryClient:   registryClient,
	}

	impl.logger.Debugw("Installing release", "name", releaseIdentifier.ReleaseName, "namespace", releaseIdentifier.ReleaseNamespace, "dry-run", dryRun)
	switch impl.helmReleaseConfig.RunHelmInstallInAsyncMode {
	case false:
		impl.logger.Debugw("Installing release", "name", releaseIdentifier.ReleaseName, "namespace", releaseIdentifier.ReleaseNamespace, "dry-run", dryRun)
		rel, err := helmClientObj.InstallChart(context.Background(), chartSpec)
		if err != nil {
			impl.logger.Errorw("Error in install release ", "err", err)
			return nil, err
		}
		//helmInstallMessage := HelmReleaseStatusConfig{
		//	InstallAppVersionHistoryId: int(request.InstallAppVersionHistoryId),
		//}
		//helmInstallMessagedata, err := impl.GetNatsMessageForHelmInstallSuccess(helmInstallMessage)
		//if err != nil {
		//	impl.logger.Errorw("Error in parsing nats message for helm install success ", "err", err)
		//}
		//_ = impl.pubsubClient.Publish(pubsub_lib.HELM_CHART_INSTALL_STATUS_TOPIC, helmInstallMessagedata)
		// Install release ends
		return rel, nil
	case true:
		go func() {
			helmInstallMessage := HelmReleaseStatusConfig{
				InstallAppVersionHistoryId: int(request.InstallAppVersionHistoryId),
			}
			// Checking release exist because there can be case when release already exist with same name
			releaseExist := impl.K8sInformer.CheckReleaseExists(releaseIdentifier.ClusterConfig.ClusterId, releaseIdentifier.ReleaseName)
			if releaseExist {
				// release with name already exist, will not continue with release
				helmInstallMessage.ErrorInInstallation = true
				helmInstallMessage.IsReleaseInstalled = false
				helmInstallMessage.Message = fmt.Sprintf("Release with name - %s already exist", releaseIdentifier.ReleaseName)
				data, err := json.Marshal(helmInstallMessage)
				if err != nil {
					impl.logger.Errorw("error in marshalling nats message")
					return
				}
				_ = impl.pubsubClient.Publish(pubsub_lib.HELM_CHART_INSTALL_STATUS_TOPIC, string(data))
			}

			_, err = helmClientObj.InstallChart(context.Background(), chartSpec)

			if err != nil {
				HelmInstallFailureNatsMessage, err := impl.GetNatsMessageForHelmInstallError(ctx, helmInstallMessage, releaseIdentifier, err)
				if err != nil {
					impl.logger.Errorw("Error in parsing nats message for helm install failure")
				}
				// in case of err we will communicate about the error to orchestrator
				_ = impl.pubsubClient.Publish(pubsub_lib.HELM_CHART_INSTALL_STATUS_TOPIC, HelmInstallFailureNatsMessage)
				return
			}
			helmInstallMessage.Message = RELEASE_INSTALLED
			helmInstallMessage.IsReleaseInstalled = true
			helmInstallMessage.ErrorInInstallation = false
			data, err := json.Marshal(helmInstallMessage)
			if err != nil {
				impl.logger.Errorw("error in marshalling nats message")
			}
			_ = impl.pubsubClient.Publish(pubsub_lib.HELM_CHART_INSTALL_STATUS_TOPIC, string(data))
		}()
	}
	// Install release ends
	return nil, nil
}

func (impl HelmAppServiceImpl) GetNotes(ctx context.Context, request *client.InstallReleaseRequest) (string, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	chartSpec := &helmClient.ChartSpec{
		ReleaseName:   releaseIdentifier.ReleaseName,
		Namespace:     releaseIdentifier.ReleaseNamespace,
		ChartName:     request.ChartName,
		CleanupOnFail: true, // allow deletion of new resources created in this rollback when rollback fails
		MaxHistory:    0,    // limit the maximum number of revisions saved per release. Use 0 for no limit (default 10)
		RepoURL:       request.ChartRepository.Url,
		Version:       request.ChartVersion,
	}
	HelmTemplateOptions := &helmClient.HelmTemplateOptions{}
	if request.K8SVersion != "" {
		HelmTemplateOptions.KubeVersion = &chartutil.KubeVersion{
			Version: request.K8SVersion,
		}
	}
	release, err := helmClientObj.GetNotes(chartSpec, HelmTemplateOptions)
	if err != nil {
		impl.logger.Errorw("Error in fetching Notes ", "err", err)
		return "", err
	}
	if release == nil {
		impl.logger.Errorw("no release found for", "name", releaseIdentifier.ReleaseName)
		return "", err
	}

	return string(release), nil
}

func (impl HelmAppServiceImpl) UpgradeReleaseWithChartInfo(ctx context.Context, request *client.InstallReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		return nil, err
	}
	var registryClient *registry.Client
	var chartName string

	switch request.IsOCIRepo {
	case true:
		username, password, err := impl.ExtractCredentialsForRegistry(request.RegistryCredential)
		if err != nil {
			return nil, err
		}
		// Updating registry credentials
		request.RegistryCredential.Username = username
		request.RegistryCredential.Password = password
		registryClient, err = registry.NewClient()
		if err != nil {
			impl.logger.Errorw(HELM_CLIENT_ERROR, "err", err)
			return nil, err
		}
		err = impl.OCIRegistryLogin(registryClient, request.RegistryCredential)
		if err != nil {
			return nil, err
		}
		chartName = fmt.Sprintf("%s://%s/%s", "oci", request.RegistryCredential.RegistryUrl, request.RegistryCredential.RepoName)
	case false:
		chartRepoRequest := request.ChartRepository
		chartRepoName := chartRepoRequest.Name
		// Add or update chart repo starts
		chartRepo := repo.Entry{
			Name:     chartRepoName,
			URL:      chartRepoRequest.Url,
			Username: chartRepoRequest.Username,
			Password: chartRepoRequest.Password,
			// Since helm 3.6.1 it is necessary to pass 'PassCredentialsAll = true'.
			PassCredentialsAll:    true,
			InsecureSkipTLSverify: true,
		}
		impl.logger.Debug("Adding/Updating Chart repo")
		err = helmClientObj.AddOrUpdateChartRepo(chartRepo)
		if err != nil {
			impl.logger.Errorw("Error in add/update chart repo ", "err", err)
			return nil, err
		}
		chartName = fmt.Sprintf("%s/%s", chartRepoName, request.ChartName)
		// Add or update chart repo ends
	}

	// Update release starts
	chartSpec := &helmClient.ChartSpec{
		ReleaseName:      releaseIdentifier.ReleaseName,
		Namespace:        releaseIdentifier.ReleaseNamespace,
		ValuesYaml:       request.ValuesYaml,
		ChartName:        chartName,
		Version:          request.ChartVersion,
		DependencyUpdate: true,
		UpgradeCRDs:      true,
		MaxHistory:       int(request.HistoryMax),
		RegistryClient:   registryClient,
	}

	switch impl.helmReleaseConfig.RunHelmInstallInAsyncMode {
	case false:
		impl.logger.Debug("Upgrading release with chart info")
		_, err = helmClientObj.UpgradeReleaseWithChartInfo(context.Background(), chartSpec)
		if UpgradeErr, ok := err.(*driver.StorageDriverError); ok {
			if UpgradeErr != nil {
				if UpgradeErr.Err == driver.ErrReleaseNotFound {
					_, err := helmClientObj.InstallChart(context.Background(), chartSpec)
					if err != nil {
						impl.logger.Errorw("Error in install release ", "err", err)
						return nil, err
					}

				} else {
					impl.logger.Errorw("Error in upgrade release with chart info", "err", err)
					return nil, err

				}
			}
		}
		//helmInstallMessage := HelmReleaseStatusConfig{
		//	InstallAppVersionHistoryId: int(request.InstallAppVersionHistoryId),
		//}
		//helmInstallMessagedata, err := impl.GetNatsMessageForHelmInstallSuccess(helmInstallMessage)
		//if err != nil {
		//	impl.logger.Errorw("Error in parsing nats message for helm install success ", "err", err)
		//}
		//_ = impl.pubsubClient.Publish(pubsub_lib.HELM_CHART_INSTALL_STATUS_TOPIC, helmInstallMessagedata)
	case true:
		go func() {
			impl.logger.Debug("Upgrading release with chart info")
			_, err = helmClientObj.UpgradeReleaseWithChartInfo(context.Background(), chartSpec)
			helmInstallMessage := HelmReleaseStatusConfig{
				InstallAppVersionHistoryId: int(request.InstallAppVersionHistoryId),
			}
			var HelmInstallFailureNatsMessage string

			if UpgradeErr, ok := err.(*driver.StorageDriverError); ok {
				if UpgradeErr != nil {
					if UpgradeErr.Err == driver.ErrReleaseNotFound {
						_, err := helmClientObj.InstallChart(context.Background(), chartSpec)
						if err != nil {
							HelmInstallFailureNatsMessage, _ = impl.GetNatsMessageForHelmInstallError(ctx, helmInstallMessage, releaseIdentifier, err)
						}
					} else {
						HelmInstallFailureNatsMessage, _ = impl.GetNatsMessageForHelmInstallError(ctx, helmInstallMessage, releaseIdentifier, err)
						impl.logger.Errorw("Error in upgrade release with chart info", "err", err)

					}
					_ = impl.pubsubClient.Publish(pubsub_lib.HELM_CHART_INSTALL_STATUS_TOPIC, HelmInstallFailureNatsMessage)
					return
				}
			} else if err != nil {
				HelmInstallFailureNatsMessage, _ = impl.GetNatsMessageForHelmInstallError(ctx, helmInstallMessage, releaseIdentifier, err)
				_ = impl.pubsubClient.Publish(pubsub_lib.HELM_CHART_INSTALL_STATUS_TOPIC, HelmInstallFailureNatsMessage)
				return
			}
			helmInstallMessage.Message = RELEASE_INSTALLED
			helmInstallMessage.IsReleaseInstalled = true
			data, err := json.Marshal(helmInstallMessage)
			if err != nil {
				impl.logger.Errorw("error in marshalling nats message")
			}
			_ = impl.pubsubClient.Publish(pubsub_lib.HELM_CHART_INSTALL_STATUS_TOPIC, string(data))
			// Update release ends
		}()
	}

	upgradeReleaseResponse := &client.UpgradeReleaseResponse{
		Success: true,
	}

	return upgradeReleaseResponse, nil

}

func (impl HelmAppServiceImpl) IsReleaseInstalled(ctx context.Context, releaseIdentifier *client.ReleaseIdentifier) (bool, error) {
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		return false, err
	}

	isInstalled, err := helmClientObj.IsReleaseInstalled(ctx, releaseIdentifier.ReleaseName, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		impl.logger.Errorw("Error in checking if the release is installed", "err", err)
		return false, err
	}

	return isInstalled, err
}

func (impl HelmAppServiceImpl) RollbackRelease(request *client.RollbackReleaseRequest) (bool, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		return false, err
	}

	// Rollback release starts
	chartSpec := &helmClient.ChartSpec{
		ReleaseName:   releaseIdentifier.ReleaseName,
		Namespace:     releaseIdentifier.ReleaseNamespace,
		CleanupOnFail: true, // allow deletion of new resources created in this rollback when rollback fails
		MaxHistory:    0,    // limit the maximum number of revisions saved per release. Use 0 for no limit (default 10)
	}

	impl.logger.Debug("Rollback release starts")
	err = helmClientObj.RollbackRelease(chartSpec, int(request.Version))
	if err != nil {
		impl.logger.Errorw("Error in Rollback release", "err", err)
		return false, err
	}

	return true, nil
}

func (impl HelmAppServiceImpl) TemplateChart(ctx context.Context, request *client.InstallReleaseRequest) (string, error) {

	releaseIdentifier := request.ReleaseIdentifier

	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		impl.logger.Errorw("error in getting helm app client")
	}

	registryClient, err := registry.NewClient()
	if err != nil {
		impl.logger.Errorw(HELM_CLIENT_ERROR, "err", err)
		return "", err
	}

	var chartName, repoURL string

	switch request.IsOCIRepo {
	case true:
		if request.RegistryCredential != nil && !request.RegistryCredential.IsPublic {
			err = impl.OCIRegistryLogin(registryClient, request.RegistryCredential)
			if err != nil {
				return "", err
			}
		}
		chartName = impl.GetOCIChartName(request.RegistryCredential.RegistryUrl, request.RegistryCredential.RepoName)
	case false:
		chartName = request.ChartName
		repoURL = request.ChartRepository.Url
	}

	chartSpec := &helmClient.ChartSpec{
		ReleaseName:    releaseIdentifier.ReleaseName,
		Namespace:      releaseIdentifier.ReleaseNamespace,
		ChartName:      chartName,
		CleanupOnFail:  true, // allow deletion of new resources created in this rollback when rollback fails
		MaxHistory:     0,    // limit the maximum number of revisions saved per release. Use 0 for no limit (default 10)
		RepoURL:        repoURL,
		Version:        request.ChartVersion,
		ValuesYaml:     request.ValuesYaml,
		RegistryClient: registryClient,
	}

	HelmTemplateOptions := &helmClient.HelmTemplateOptions{}
	if request.K8SVersion != "" {
		HelmTemplateOptions.KubeVersion = &chartutil.KubeVersion{
			Version: request.K8SVersion,
		}
	}
	var content []byte
	if request.ChartContent != nil {
		content = request.ChartContent.Content
	}
	rel, err := helmClientObj.TemplateChart(chartSpec, HelmTemplateOptions, content)
	if err != nil {
		impl.logger.Errorw("error occured while generating manifest in helm app service", "err:", err)
		return "", err
	}

	if rel == nil {
		return "", errors.New("release is found nil")
	}
	return string(rel), nil
}

func (impl HelmAppServiceImpl) getHelmRelease(clusterConfig *client.ClusterConfig, namespace string, releaseName string) (*release.Release, error) {

	k8sClusterConfig := GetClusterConfigFromClientBean(clusterConfig)
	conf, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		return nil, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
			Namespace: namespace,
		},
		RestConfig: conf,
	}
	helmClient, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		return nil, err
	}
	release, err := helmClient.GetRelease(releaseName)
	if err != nil {
		return nil, err
	}
	return release, nil
}

func (impl HelmAppServiceImpl) getHelmReleaseHistory(clusterConfig *client.ClusterConfig, releaseNamespace string, releaseName string, countOfHelmReleaseHistory int) ([]*release.Release, error) {
	k8sClusterConfig := GetClusterConfigFromClientBean(clusterConfig)
	conf, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		return nil, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
			Namespace: releaseNamespace,
		},
		RestConfig: conf,
	}

	helmClient, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		return nil, err
	}

	releases, err := helmClient.ListReleaseHistory(releaseName, countOfHelmReleaseHistory)
	if err != nil {
		return nil, err
	}

	return releases, nil
}

func buildReleaseInfoBasicData(helmRelease *release.Release) (*client.ReleaseInfo, error) {
	defaultValues := helmRelease.Chart.Values
	overrideValues := helmRelease.Config
	var defaultValString, overrideValuesString, mergedValuesString []byte
	var err error
	defaultValString, err = json.Marshal(defaultValues)
	if err != nil {
		return nil, err
	}
	if overrideValues == nil {
		mergedValuesString = defaultValString
	} else {
		overrideValuesString, err = json.Marshal(overrideValues)
		if err != nil {
			return nil, err
		}
		mergedValuesString, err = jsonpatch.MergePatch(defaultValString, overrideValuesString)
		if err != nil {
			return nil, err
		}
	}
	var readme string
	for _, file := range helmRelease.Chart.Files {
		if file.Name == ReadmeFileName {
			readme = string(file.Data)
			break
		}
	}
	res := &client.ReleaseInfo{
		DefaultValues:    string(defaultValString),
		OverrideValues:   string(overrideValuesString),
		MergedValues:     string(mergedValuesString),
		Readme:           readme,
		ValuesSchemaJson: string(helmRelease.Chart.Schema),
	}

	return res, nil
}

func (impl *HelmAppServiceImpl) getNodes(appDetailRequest *client.AppDetailRequest, release *release.Release) ([]*bean.ResourceNode, []*bean.HealthStatus, error) {
	k8sClusterConfig := GetClusterConfigFromClientBean(appDetailRequest.ClusterConfig)
	conf, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		return nil, nil, err
	}
	manifests, err := yamlUtil.SplitYAMLs([]byte(release.Manifest))
	if err != nil {
		return nil, nil, err
	}
	// get live manifests from kubernetes
	desiredOrLiveManifests, err := impl.getDesiredOrLiveManifests(conf, manifests, appDetailRequest.Namespace)
	if err != nil {
		return nil, nil, err
	}
	// build resource nodes
	nodes, healthStatusArray, err := impl.buildNodes(conf, desiredOrLiveManifests, appDetailRequest.Namespace, nil)
	if err != nil {
		return nil, nil, err
	}
	return nodes, healthStatusArray, nil
}

func (impl HelmAppServiceImpl) buildResourceTree(appDetailRequest *client.AppDetailRequest, release *release.Release) (*bean.ResourceTreeResponse, error) {
	k8sClusterConfig := GetClusterConfigFromClientBean(appDetailRequest.ClusterConfig)
	conf, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		return nil, err
	}
	manifests, err := yamlUtil.SplitYAMLs([]byte(release.Manifest))
	if err != nil {
		return nil, err
	}
	// get live manifests from kubernetes
	desiredOrLiveManifests, err := impl.getDesiredOrLiveManifests(conf, manifests, appDetailRequest.Namespace)
	if err != nil {
		return nil, err
	}
	// build resource nodes
	nodes, _, err := impl.buildNodes(conf, desiredOrLiveManifests, appDetailRequest.Namespace, nil)
	if err != nil {
		return nil, err
	}

	// filter nodes based on ResourceTreeFilter
	resourceTreeFilter := appDetailRequest.ResourceTreeFilter
	if resourceTreeFilter != nil && len(nodes) > 0 {
		nodes = impl.filterNodes(resourceTreeFilter, nodes)
	}

	// build pods metadata
	podsMetadata, err := buildPodMetadata(nodes)
	if err != nil {
		return nil, err
	}
	resourceTreeResponse := &bean.ResourceTreeResponse{
		ApplicationTree: &bean.ApplicationTree{
			Nodes: nodes,
		},
		PodMetadata: podsMetadata,
	}
	return resourceTreeResponse, nil
}

func (impl HelmAppServiceImpl) filterNodes(resourceTreeFilter *client.ResourceTreeFilter, nodes []*bean.ResourceNode) []*bean.ResourceNode {
	resourceFilters := resourceTreeFilter.ResourceFilters
	globalFilter := resourceTreeFilter.GlobalFilter
	if globalFilter == nil && (resourceFilters == nil || len(resourceFilters) == 0) {
		return nodes
	}

	filteredNodes := make([]*bean.ResourceNode, 0, len(nodes))

	// handle global
	if globalFilter != nil && len(globalFilter.Labels) > 0 {
		globalLabels := globalFilter.Labels
		for _, node := range nodes {
			toAdd := util.IsMapSubset(node.NetworkingInfo.Labels, globalLabels)
			if toAdd {
				filteredNodes = append(filteredNodes, node)
			}
		}
		return filteredNodes
	}

	// handle gvk level
	var gvkVsLabels map[schema.GroupVersionKind]map[string]string
	for _, resourceFilter := range resourceTreeFilter.ResourceFilters {
		gvk := resourceFilter.Gvk
		gvkVsLabels[schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind,
		}] = resourceFilter.ResourceIdentifier.Labels
	}

	for _, node := range nodes {
		nodeGvk := node.Manifest.GroupVersionKind()
		if val, ok := gvkVsLabels[nodeGvk]; ok {
			toAdd := util.IsMapSubset(node.NetworkingInfo.Labels, val)
			if toAdd {
				filteredNodes = append(filteredNodes, node)
			}
		}
	}

	return filteredNodes
}

func (impl HelmAppServiceImpl) getDesiredOrLiveManifests(restConfig *rest.Config, desiredManifests []unstructured.Unstructured, releaseNamespace string) ([]*bean.DesiredOrLiveManifest, error) {

	totalManifestCount := len(desiredManifests)
	desiredOrLiveManifestArray := make([]*bean.DesiredOrLiveManifest, totalManifestCount)
	batchSize := impl.helmReleaseConfig.ManifestFetchBatchSize

	for i := 0; i < totalManifestCount; {
		//requests left to process
		remainingBatch := totalManifestCount - i
		if remainingBatch < batchSize {
			batchSize = remainingBatch
		}
		var wg sync.WaitGroup
		for j := 0; j < batchSize; j++ {
			wg.Add(1)
			go func(j int) {
				defer wg.Done()
				desiredOrLiveManifest := impl.getManifestData(restConfig, releaseNamespace, desiredManifests[i+j])
				desiredOrLiveManifestArray[i+j] = desiredOrLiveManifest
			}(j)
		}
		wg.Wait()
		i += batchSize
	}

	return desiredOrLiveManifestArray, nil
}

func (impl HelmAppServiceImpl) getManifestData(restConfig *rest.Config, releaseNamespace string, desiredManifest unstructured.Unstructured) *bean.DesiredOrLiveManifest {
	gvk := desiredManifest.GroupVersionKind()
	_namespace := desiredManifest.GetNamespace()
	if _namespace == "" {
		_namespace = releaseNamespace
	}
	liveManifest, _, err := impl.k8sService.GetLiveManifest(restConfig, _namespace, &gvk, desiredManifest.GetName())
	desiredOrLiveManifest := &bean.DesiredOrLiveManifest{}

	if err != nil {
		impl.logger.Errorw("Error in getting live manifest ", "err", err)
		statusError, _ := err.(*errors2.StatusError)
		desiredOrLiveManifest = &bean.DesiredOrLiveManifest{
			// using deep copy as it replaces item in manifest in loop
			Manifest:                 desiredManifest.DeepCopy(),
			IsLiveManifestFetchError: true,
		}
		if statusError != nil {
			desiredOrLiveManifest.LiveManifestFetchErrorCode = statusError.Status().Code
		}
	} else {
		desiredOrLiveManifest = &bean.DesiredOrLiveManifest{
			Manifest: liveManifest,
		}
	}
	return desiredOrLiveManifest
}

func (impl HelmAppServiceImpl) buildNodes(restConfig *rest.Config, desiredOrLiveManifests []*bean.DesiredOrLiveManifest, releaseNamespace string, parentResourceRef *bean.ResourceRef) ([]*bean.ResourceNode, []*bean.HealthStatus, error) {
	var nodes []*bean.ResourceNode
	var healthStatusArray []*bean.HealthStatus
	for _, desiredOrLiveManifest := range desiredOrLiveManifests {
		manifest := desiredOrLiveManifest.Manifest
		gvk := manifest.GroupVersionKind()
		_namespace := manifest.GetNamespace()
		if _namespace == "" {
			_namespace = releaseNamespace
		}
		ports := make([]int64, 0)
		if k8sUtils.IsService(gvk) || gvk.Kind == "Service" {
			if manifest.Object["spec"] != nil {
				spec := manifest.Object["spec"].(map[string]interface{})
				if spec["ports"] != nil {
					portList := spec["ports"].([]interface{})
					for _, portItem := range portList {
						if portItem.(map[string]interface{}) != nil {
							_portNumber := portItem.(map[string]interface{})["port"]
							portNumber := _portNumber.(int64)
							if portNumber != 0 {
								ports = append(ports, portNumber)
							} else {
								impl.logger.Errorw("there is no port", "err", portNumber)
							}
						} else {
							impl.logger.Errorw("there are no port list availabe", "err", portItem)
						}
					}
				}
			}
		}
		if manifest.Object["kind"] == "EndpointSlice" {
			if manifest.Object["ports"] != nil {
				endPointsSlicePorts := manifest.Object["ports"].([]interface{})
				for _, val := range endPointsSlicePorts {
					_portNumber := val.(map[string]interface{})["port"]
					portNumber := _portNumber.(int64)
					if portNumber != 0 {
						ports = append(ports, portNumber)
					}
				}
			}
		}
		if gvk.Kind == "Endpoints" {
			if manifest.Object["subsets"] != nil {
				subsets := manifest.Object["subsets"].([]interface{})
				for _, subset := range subsets {
					subsetObj := subset.(map[string]interface{})
					if subsetObj != nil {
						portsIfs := subsetObj["ports"].([]interface{})
						for _, portsIf := range portsIfs {
							portsIfObj := portsIf.(map[string]interface{})
							if portsIfObj != nil {
								port := portsIfObj["port"].(int64)
								ports = append(ports, port)
							}
						}
					}
				}
			}
		}
		resourceRef := buildResourceRef(gvk, *manifest, _namespace)

		if impl.k8sService.CanHaveChild(gvk) {
			children, err := impl.k8sService.GetChildObjects(restConfig, _namespace, gvk, manifest.GetName(), manifest.GetAPIVersion())
			if err != nil {
				return nil, nil, err
			}
			desiredOrLiveManifestsChildren := make([]*bean.DesiredOrLiveManifest, 0, len(children))
			for _, child := range children {
				desiredOrLiveManifestsChildren = append(desiredOrLiveManifestsChildren, &bean.DesiredOrLiveManifest{
					Manifest: child,
				})
			}
			childNodes, _, err := impl.buildNodes(restConfig, desiredOrLiveManifestsChildren, releaseNamespace, resourceRef)
			if err != nil {
				return nil, nil, err
			}

			for _, childNode := range childNodes {
				nodes = append(nodes, childNode)
				healthStatusArray = append(healthStatusArray, childNode.Health)
			}
		}

		creationTimeStamp := ""
		val, found, err := unstructured.NestedString(manifest.Object, "metadata", "creationTimestamp")
		if found && err == nil {
			creationTimeStamp = val
		}
		node := &bean.ResourceNode{
			ResourceRef:     resourceRef,
			ResourceVersion: manifest.GetResourceVersion(),
			NetworkingInfo: &bean.ResourceNetworkingInfo{
				Labels: manifest.GetLabels(),
			},
			CreatedAt: creationTimeStamp,
			Port:      ports,
		}

		if parentResourceRef != nil {
			node.ParentRefs = append(make([]*bean.ResourceRef, 0), parentResourceRef)
		}

		// set health of node
		if desiredOrLiveManifest.IsLiveManifestFetchError {
			if desiredOrLiveManifest.LiveManifestFetchErrorCode == http.StatusNotFound {
				node.Health = &bean.HealthStatus{
					Status:  bean.HealthStatusMissing,
					Message: "Resource missing as live manifest not found",
				}
			} else {
				node.Health = &bean.HealthStatus{
					Status:  bean.HealthStatusUnknown,
					Message: "Resource state unknown as error while fetching live manifest",
				}
			}
		} else {
			//TODO: verify
			if k8sUtils.IsService(gvk) && node.Name == k8sUtils.DEVTRON_SERVICE_NAME && k8sUtils.IsDevtronApp(node.NetworkingInfo.Labels) {

				node.Health = &bean.HealthStatus{
					Status: bean.HealthStatusHealthy,
				}
			} else {
				if healthCheck := health.GetHealthCheckFunc(gvk); healthCheck != nil {
					health, err := healthCheck(manifest)
					if err != nil {
						node.Health = &bean.HealthStatus{
							Status:  bean.HealthStatusUnknown,
							Message: err.Error(),
						}
					} else if health != nil {
						node.Health = &bean.HealthStatus{
							Status:  string(health.Status),
							Message: health.Message,
						}
					}
				}
			}
		}

		// hibernate set starts
		if parentResourceRef == nil {

			// set CanBeHibernated
			replicas, found, _ := unstructured.NestedInt64(node.Manifest.UnstructuredContent(), "spec", "replicas")
			if found {
				node.CanBeHibernated = true
			}

			// set IsHibernated
			annotations := node.Manifest.GetAnnotations()
			if annotations != nil {
				if val, ok := annotations[hibernateReplicaAnnotation]; ok {
					if val != "0" && replicas == 0 {
						node.IsHibernated = true
					}
				}
			}

		}
		// hibernate set ends

		if k8sUtils.IsPod(gvk) {
			infoItems, _ := argo.PopulatePodInfo(manifest)
			node.Info = infoItems
		}

		nodes = append(nodes, node)
		healthStatusArray = append(healthStatusArray, node.Health)
	}

	return nodes, healthStatusArray, nil
}

func buildResourceRef(gvk schema.GroupVersionKind, manifest unstructured.Unstructured, namespace string) *bean.ResourceRef {
	resourceRef := &bean.ResourceRef{
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
		Namespace: namespace,
		Name:      manifest.GetName(),
		UID:       string(manifest.GetUID()),
		Manifest:  manifest,
	}
	return resourceRef
}

func buildPodMetadata(nodes []*bean.ResourceNode) ([]*bean.PodMetadata, error) {
	podsMetadata := make([]*bean.PodMetadata, 0, len(nodes))
	for _, node := range nodes {
		if node.Kind != k8sCommonBean.PodKind {
			continue
		}

		var pod coreV1.Pod
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(node.Manifest.UnstructuredContent(), &pod)
		if err != nil {
			return nil, err
		}

		// check if pod is new
		isNew := true
		if len(node.ParentRefs) > 0 {
			parentRef := node.ParentRefs[0]
			parentKind := parentRef.Kind

			// if parent is StatefulSet - then pod label controller-revision-hash should match StatefulSet's update revision
			if parentKind == k8sCommonBean.StatefulSetKind {
				var statefulSet appsV1.StatefulSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &statefulSet)
				if err != nil {
					return nil, err
				}
				isNew = statefulSet.Status.UpdateRevision == pod.GetLabels()["controller-revision-hash"]
			}

			// if parent is Job - then pod label controller-revision-hash should match StatefulSet's update revision
			if parentKind == k8sCommonBean.JobKind {
				//TODO - new or old logic not built in orchestrator for Job's pods. hence not implementing here. as don't know the logic :)
				isNew = true
			}

			// if parent kind is replica set then
			if parentKind == k8sCommonBean.ReplicaSetKind {
				var replicaSet appsV1.ReplicaSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &replicaSet)
				if err != nil {
					return nil, err
				}
				replicaSetNode := getMatchingNode(nodes, parentKind, replicaSet.Name)

				// if parent of replicaset is deployment, compare label pod-template-hash
				if replicaSetNode != nil && len(replicaSetNode.ParentRefs) > 0 && replicaSetNode.ParentRefs[0].Kind == k8sCommonBean.DeploymentKind {
					isNew = replicaSet.GetLabels()["pod-template-hash"] == pod.GetLabels()["pod-template-hash"]
				}
			}

			// if parent kind is DaemonSet then compare DaemonSet's Child ControllerRevision's label controller-revision-hash with pod label controller-revision-hash
			if parentKind == k8sCommonBean.DaemonSetKind {
				var daemonSet appsV1.DaemonSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &daemonSet)
				if err != nil {
					return nil, err
				}

				controllerRevisionNodes := getMatchingNodes(nodes, "ControllerRevision")
				for _, controllerRevisionNode := range controllerRevisionNodes {
					if len(controllerRevisionNode.ParentRefs) > 0 && controllerRevisionNode.ParentRefs[0].Kind == parentKind &&
						controllerRevisionNode.ParentRefs[0].Name == daemonSet.Name {

						var controlRevision appsV1.ControllerRevision
						err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &controlRevision)
						if err != nil {
							return nil, err
						}
						isNew = controlRevision.GetLabels()["controller-revision-hash"] == pod.GetLabels()["controller-revision-hash"]
					}
				}
			}
		}

		// set containers,initContainers and ephemeral container names
		containerNames := make([]string, 0, len(pod.Spec.Containers))
		initContainerNames := make([]string, 0, len(pod.Spec.InitContainers))
		ephemeralContainers := make([]*bean.EphemeralContainerData, 0, len(pod.Spec.EphemeralContainers))
		for _, container := range pod.Spec.Containers {
			containerNames = append(containerNames, container.Name)
		}
		for _, initContainer := range pod.Spec.InitContainers {
			initContainerNames = append(initContainerNames, initContainer.Name)
		}

		ephemeralContainerStatusMap := make(map[string]bool)
		for _, c := range pod.Status.EphemeralContainerStatuses {
			//c.state contains three states running,waiting and terminated
			// at any point of time only one state will be there
			if c.State.Running != nil {
				ephemeralContainerStatusMap[c.Name] = true
			}
		}

		//sending only running ephemeral containers in the list
		for _, ec := range pod.Spec.EphemeralContainers {
			if _, ok := ephemeralContainerStatusMap[ec.Name]; ok {
				containerData := &bean.EphemeralContainerData{
					Name:       ec.Name,
					IsExternal: isExternalEphemeralContainer(ec.Command, ec.Name),
				}
				ephemeralContainers = append(ephemeralContainers, containerData)
			}
		}

		podMetadata := &bean.PodMetadata{
			Name:                node.Name,
			UID:                 node.UID,
			Containers:          containerNames,
			InitContainers:      initContainerNames,
			EphemeralContainers: ephemeralContainers,
			IsNew:               isNew,
		}

		podsMetadata = append(podsMetadata, podMetadata)

	}
	return podsMetadata, nil
}

func isExternalEphemeralContainer(cmds []string, name string) bool {
	isExternal := true
	matchingCmd := fmt.Sprintf("sh %s-devtron.sh", name)
	for _, cmd := range cmds {
		if strings.Contains(cmd, matchingCmd) {
			isExternal = false
			break
		}
	}
	return isExternal
}

func getMatchingNode(nodes []*bean.ResourceNode, kind string, name string) *bean.ResourceNode {
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return node
		}
	}
	return nil
}

func getMatchingNodes(nodes []*bean.ResourceNode, kind string) []*bean.ResourceNode {
	nodesRes := make([]*bean.ResourceNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Kind == kind {
			nodesRes = append(nodesRes, node)
		}
	}
	return nodesRes
}

func (impl HelmAppServiceImpl) getHelmClient(clusterConfig *client.ClusterConfig, releaseNamespace string) (helmClient.Client, error) {
	k8sClusterConfig := GetClusterConfigFromClientBean(clusterConfig)
	conf, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("Error in getting rest config ", "err", err)
		return nil, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
			Namespace: releaseNamespace,
		},
		RestConfig: conf,
	}
	helmClientObj, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		impl.logger.Errorw("Error in building client from rest config ", "err", err)
		return nil, err
	}
	return helmClientObj, nil
}

func (impl HelmAppServiceImpl) InstallReleaseWithCustomChart(request *client.HelmInstallCustomRequest) (bool, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}
	var b bytes.Buffer
	writer := gzip.NewWriter(&b)
	_, err = writer.Write(request.ChartContent.Content)
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}
	err = writer.Close()
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}

	if _, err := os.Stat(chartWorkingDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(chartWorkingDirectory, os.ModePerm)
		if err != nil {
			impl.logger.Errorw("err in creating dir", "err", err)
			return false, err
		}
	}
	dir := impl.GetRandomString()
	referenceChartDir := filepath.Join(chartWorkingDirectory, dir)
	referenceChartDir = fmt.Sprintf("%s.tgz", referenceChartDir)
	defer impl.CleanDir(referenceChartDir)
	err = ioutil.WriteFile(referenceChartDir, b.Bytes(), os.ModePerm)
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}
	impl.logger.Debugw("tar file write at", "referenceChartDir", referenceChartDir)
	// Update release starts
	chartSpec := &helmClient.ChartSpec{
		ReleaseName: releaseIdentifier.ReleaseName,
		Namespace:   releaseIdentifier.ReleaseNamespace,
		ValuesYaml:  request.ValuesYaml,
		ChartName:   referenceChartDir,
	}

	//	impl.logger.Debug("Upgrading release with chart info")
	_, err = helmClientObj.InstallChart(context.Background(), chartSpec)
	if err != nil {
		impl.logger.Errorw("Error in install chart", "err", err)
		return false, err
	}
	// Update release ends

	return true, nil
}

func (impl HelmAppServiceImpl) UpgradeReleaseWithCustomChart(ctx context.Context, request *client.UpgradeReleaseRequest) (bool, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}
	var b bytes.Buffer
	writer := gzip.NewWriter(&b)
	_, err = writer.Write(request.ChartContent.Content)
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}
	err = writer.Close()
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}

	if _, err := os.Stat(chartWorkingDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(chartWorkingDirectory, os.ModePerm)
		if err != nil {
			impl.logger.Errorw("err in creating dir", "err", err)
			return false, err
		}
	}
	dir := impl.GetRandomString()
	referenceChartDir := filepath.Join(chartWorkingDirectory, dir)
	referenceChartDir = fmt.Sprintf("%s.tgz", referenceChartDir)
	defer impl.CleanDir(referenceChartDir)
	err = ioutil.WriteFile(referenceChartDir, b.Bytes(), os.ModePerm)
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}
	impl.logger.Debugw("tar file write at", "referenceChartDir", referenceChartDir)
	// Update release starts
	updateChartSpec := &helmClient.ChartSpec{
		ReleaseName: releaseIdentifier.ReleaseName,
		Namespace:   releaseIdentifier.ReleaseNamespace,
		ValuesYaml:  request.ValuesYaml,
		ChartName:   referenceChartDir,
		MaxHistory:  int(request.HistoryMax),
	}

	impl.logger.Debug("Upgrading release")
	_, err = helmClientObj.UpgradeReleaseWithChartInfo(context.Background(), updateChartSpec)
	if UpgradeErr, ok := err.(*driver.StorageDriverError); ok {
		if UpgradeErr != nil {
			if UpgradeErr.Err == driver.ErrNoDeployedReleases {
				_, err := helmClientObj.InstallChart(context.Background(), updateChartSpec)
				if err != nil {
					impl.logger.Errorw("Error in install release ", "err", err)
					return false, err
				}

			} else {
				impl.logger.Errorw("Error in upgrade release with chart info", "err", err)
				return false, err

			}
		}
	}
	return true, nil
}

func (impl HelmAppServiceImpl) ExtractCredentialsForRegistry(registryCredential *client.RegistryCredential) (string, string, error) {
	username := registryCredential.Username
	pwd := registryCredential.Password
	if (registryCredential.RegistryType == REGISTRYTYPE_GCR || registryCredential.RegistryType == REGISTRYTYPE_ARTIFACT_REGISTRY) && username == JSON_KEY_USERNAME {
		if strings.HasPrefix(pwd, "'") {
			pwd = pwd[1:]
		}
		if strings.HasSuffix(pwd, "'") {
			pwd = pwd[:len(pwd)-1]
		}
	}
	if registryCredential.RegistryType == REGISTRY_TYPE_ECR {
		accessKey, secretKey := registryCredential.AccessKey, registryCredential.SecretKey
		var creds *credentials.Credentials

		if len(registryCredential.AccessKey) == 0 || len(registryCredential.SecretKey) == 0 {
			sess, err := session.NewSession(&aws.Config{
				Region: &registryCredential.AwsRegion,
			})
			if err != nil {
				impl.logger.Errorw("Error in creating AWS client", "err", err)
				return "", "", err
			}
			creds = ec2rolecreds.NewCredentials(sess)
		} else {
			creds = credentials.NewStaticCredentials(accessKey, secretKey, "")
		}
		sess, err := session.NewSession(&aws.Config{
			Region:      &registryCredential.AwsRegion,
			Credentials: creds,
		})
		if err != nil {
			impl.logger.Errorw("Error in creating AWS client session", "err", err)
			return "", "", err
		}
		svc := ecr.New(sess)
		input := &ecr.GetAuthorizationTokenInput{}
		authData, err := svc.GetAuthorizationToken(input)
		if err != nil {
			impl.logger.Errorw("Error fetching authData", "err", err)
			return "", "", err
		}
		// decode token
		token := authData.AuthorizationData[0].AuthorizationToken
		decodedToken, err := base64.StdEncoding.DecodeString(*token)
		if err != nil {
			impl.logger.Errorw("Error in decoding auth token", "err", err)
			return "", "", err
		}
		credsSlice := strings.Split(string(decodedToken), ":")
		username = credsSlice[0]
		pwd = credsSlice[1]

	}
	return username, pwd, nil
}

func (impl HelmAppServiceImpl) OCIRegistryLogin(client *registry.Client, registryCredential *client.RegistryCredential) error {
	username, pwd, err := impl.ExtractCredentialsForRegistry(registryCredential)
	if err != nil {
		return err
	}
	// helm registry login --username "" --password ""
	err = client.Login(registryCredential.RegistryUrl,
		registry.LoginOptBasicAuth(username, pwd), registry.LoginOptInsecure(false))
	if err != nil {
		impl.logger.Errorw("Failed to login to registry", "registryURL", registryCredential.RegistryUrl, "err", err)
		return err
	}
	return nil
}

func (impl HelmAppServiceImpl) ValidateOCIRegistryLogin(ctx context.Context, OCIRegistryRequest *client.RegistryCredential) (*client.OCIRegistryResponse, error) {
	helmClient, err := registry.NewClient()
	if err != nil {
		impl.logger.Errorw(HELM_CLIENT_ERROR, "err", err)
		return nil, err
	}
	err = impl.OCIRegistryLogin(helmClient, OCIRegistryRequest)
	if err != nil {
		return nil, err
	}
	return &client.OCIRegistryResponse{
		IsLoggedIn: true,
	}, err
}

func (impl HelmAppServiceImpl) PushHelmChartToOCIRegistryRepo(ctx context.Context, OCIRegistryRequest *client.OCIRegistryRequest) (*client.OCIRegistryResponse, error) {
	// Login to OCI registry
	registryPushResponse := &client.OCIRegistryResponse{}
	helmClient, err := registry.NewClient()
	if err != nil {
		impl.logger.Errorw(HELM_CLIENT_ERROR, "err", err)
		return nil, err
	}
	err = impl.OCIRegistryLogin(helmClient, OCIRegistryRequest.RegistryCredential)
	if err != nil {
		registryPushResponse.IsLoggedIn = false
		return registryPushResponse, err
	}
	// LoggedIn successfully
	registryPushResponse.IsLoggedIn = true

	var pushOpts []registry.PushOption
	provRef := fmt.Sprintf("%s.prov", OCIRegistryRequest.Chart)
	if _, err := os.Stat(provRef); err == nil {
		provBytes, err := ioutil.ReadFile(provRef)
		if err != nil {
			impl.logger.Errorw("Error in extracting prov bytes", "err", err)
			return registryPushResponse, err
		}
		pushOpts = append(pushOpts, registry.PushOptProvData(provBytes))
	}

	var ref string
	withStrictMode := registry.PushOptStrictMode(true)
	repoURL := path.Join(OCIRegistryRequest.RegistryCredential.RegistryUrl, OCIRegistryRequest.RegistryCredential.RepoName)

	if OCIRegistryRequest.ChartName == "" || OCIRegistryRequest.ChartVersion == "" {
		// extract meta data from chart
		meta, err := loader.LoadArchive(bytes.NewReader(OCIRegistryRequest.Chart))
		if err != nil {
			impl.logger.Errorw("Error in loading chart bytes", "err", err)
			return nil, err
		}
		// add chart name and version from the chart metadata
		ref = fmt.Sprintf("%s:%s",
			path.Join(strings.TrimPrefix(repoURL, fmt.Sprintf("%s://", registry.OCIScheme)), meta.Metadata.Name),
			meta.Metadata.Version)
	} else {
		// disable strict mode for configuring chartName in repo
		withStrictMode = registry.PushOptStrictMode(false)
		// add chartName and version to url
		ref = fmt.Sprintf("%s:%s",
			path.Join(strings.TrimPrefix(repoURL, fmt.Sprintf("%s://", registry.OCIScheme)), OCIRegistryRequest.ChartName),
			OCIRegistryRequest.ChartVersion)
	}

	pushResult, err := helmClient.Push(OCIRegistryRequest.Chart, ref, withStrictMode)
	if err != nil {
		impl.logger.Errorw("Error in pushing helm chart to OCI registry", "err", err)
		return registryPushResponse, err
	}
	registryPushResponse.PushResult = &client.OCIRegistryPushResponse{
		Digest:    pushResult.Manifest.Digest,
		PushedURL: pushResult.Ref,
	}
	return registryPushResponse, err
}

func (impl HelmAppServiceImpl) GetNatsMessageForHelmInstallError(ctx context.Context, helmInstallMessage HelmReleaseStatusConfig, releaseIdentifier *client.ReleaseIdentifier, installationErr error) (string, error) {
	helmInstallMessage.Message = installationErr.Error()
	isReleaseInstalled, err := impl.IsReleaseInstalled(ctx, releaseIdentifier)
	if err != nil {
		impl.logger.Errorw("error in checking if release is installed or not")
		return "", err
	}
	if isReleaseInstalled {
		helmInstallMessage.IsReleaseInstalled = true
	} else {
		helmInstallMessage.IsReleaseInstalled = false
	}
	helmInstallMessage.ErrorInInstallation = true
	data, err := json.Marshal(helmInstallMessage)
	if err != nil {
		impl.logger.Errorw("error in marshalling nats message")
		return string(data), err
	}
	return string(data), nil
}

func (impl HelmAppServiceImpl) GetNatsMessageForHelmInstallSuccess(helmInstallMessage HelmReleaseStatusConfig) (string, error) {
	helmInstallMessage.Message = RELEASE_INSTALLED
	helmInstallMessage.IsReleaseInstalled = true
	helmInstallMessage.ErrorInInstallation = false
	data, err := json.Marshal(helmInstallMessage)
	if err != nil {
		impl.logger.Errorw("error in marshalling nats message")
		return string(data), err
	}
	return string(data), nil
}
func GetClusterConfigFromClientBean(config *client.ClusterConfig) *k8sUtils.ClusterConfig {
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
