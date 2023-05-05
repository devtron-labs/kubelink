package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	"github.com/caarlos0/env"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/helmClient"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/util"
	"github.com/devtron-labs/kubelink/pkg/util/argo"
	gitops_engine "github.com/devtron-labs/kubelink/pkg/util/gitops-engine"
	k8sUtils "github.com/devtron-labs/kubelink/pkg/util/k8s"
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
	"time"
)

const (
	hibernateReplicaAnnotation = "hibernator.devtron.ai/replicas"
	hibernatePatch             = `[{"op": "replace", "path": "/spec/replicas", "value":%d}, {"op": "add", "path": "/metadata/annotations", "value": {"%s":"%s"}}]`
	chartWorkingDirectory      = "/home/devtron/devtroncd/charts/"
)

type HelmAppService interface {
	GetApplicationListForCluster(config *client.ClusterConfig) *client.DeployedAppList
	BuildAppDetail(req *client.AppDetailRequest) (*bean.AppDetail, error)
	FetchApplicationStatusWithLastDeployed(req *client.AppDetailRequest) (*AppStatusWithLastDeployment, error)
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
}

type HelmReleaseConfig struct {
	EnableHelmReleaseCache bool `env:"ENABLE_HELM_RELEASE_CACHE" envDefault:"true"`
}

func GetHelmReleaseConfig() (*HelmReleaseConfig, error) {
	cfg := &HelmReleaseConfig{}
	err := env.Parse(cfg)
	return cfg, err
}

type AppStatusWithLastDeployment struct {
	appStatus        *bean.HealthStatusCode
	lastDeployedTime *timestamppb.Timestamp
}

type HelmAppServiceImpl struct {
	logger            *zap.SugaredLogger
	k8sService        K8sService
	randSource        rand.Source
	K8sInformer       k8sInformer.K8sInformer
	helmReleaseConfig *HelmReleaseConfig
}

func NewHelmAppServiceImpl(logger *zap.SugaredLogger, k8sService K8sService, k8sInformer k8sInformer.K8sInformer, helmReleaseConfig *HelmReleaseConfig) *HelmAppServiceImpl {

	helmAppServiceImpl := &HelmAppServiceImpl{
		logger:            logger,
		k8sService:        k8sService,
		randSource:        rand.NewSource(time.Now().UnixNano()),
		K8sInformer:       k8sInformer,
		helmReleaseConfig: helmReleaseConfig,
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
		restConfig, err := k8sUtils.GetRestConfig(config)
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
	helmRelease, err := getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
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
	}

	return appDetail, nil
}

func (impl *HelmAppServiceImpl) FetchApplicationStatusWithLastDeployed(req *client.AppDetailRequest) (*AppStatusWithLastDeployment, error) {
	helmRelease, err := getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		return &AppStatusWithLastDeployment{}, err
	}
	nodes, err := impl.getNodes(req, helmRelease)
	if err != nil {
		impl.logger.Errorw("Error in getting nodes", "err", err, "req", req)
		return &AppStatusWithLastDeployment{}, err
	}
	//getting app status on basis of healthy/non-healthy as this api is used for deployment status
	//in orchestrator and not for app status
	appStatusWithLastDeployment := &AppStatusWithLastDeployment{
		appStatus:        util.GetAppStatusOnBasisOfHealthyNonHealthy(nodes),
		lastDeployedTime: timestamppb.New(helmRelease.Info.LastDeployed.Time),
	}
	return appStatusWithLastDeployment, nil
}

func (impl HelmAppServiceImpl) GetHelmAppValues(req *client.AppDetailRequest) (*client.ReleaseInfo, error) {

	helmRelease, err := getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
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
	conf, err := k8sUtils.GetRestConfig(clusterConfig)
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
	helmReleases, err := getHelmReleaseHistory(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release history ", "err", err)
		return nil, err
	}
	var helmAppDeployments []*client.HelmAppDeploymentDetail
	for _, helmRelease := range helmReleases {
		chartMetadata := helmRelease.Chart.Metadata
		manifests := helmRelease.Manifest
		parsedManifests, err := util.SplitYAMLs([]byte(manifests))
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
	helmRelease, err := getHelmRelease(req.ClusterConfig, req.ReleaseNamespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		return nil, err
	}

	manifests, err := util.SplitYAMLs([]byte(helmRelease.Manifest))
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
	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		return nil, err
	}

	helmRelease, err := getHelmRelease(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace, releaseIdentifier.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		return nil, err
	}

	updateChartSpec := &helmClient.ChartSpec{
		ReleaseName: releaseIdentifier.ReleaseName,
		Namespace:   releaseIdentifier.ReleaseNamespace,
		ValuesYaml:  request.ValuesYaml,
	}

	impl.logger.Debug("Upgrading release")
	_, err = helmClientObj.UpgradeRelease(context.Background(), helmRelease.Chart, updateChartSpec)
	if err != nil {
		impl.logger.Errorw("Error in upgrade release ", "err", err)
		return nil, err
	}

	upgradeReleaseResponse := &client.UpgradeReleaseResponse{
		Success: true,
	}

	return upgradeReleaseResponse, nil
}

func (impl HelmAppServiceImpl) GetDeploymentDetail(request *client.DeploymentDetailRequest) (*client.DeploymentDetailResponse, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmReleases, err := getHelmReleaseHistory(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace, releaseIdentifier.ReleaseName)
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
	_, err := impl.installRelease(request, false)
	if err != nil {
		return nil, err
	}
	// Install release ends

	installReleaseResponse := &client.InstallReleaseResponse{
		Success: true,
	}

	return installReleaseResponse, nil

}
func (impl HelmAppServiceImpl) installRelease(request *client.InstallReleaseRequest, dryRun bool) (*release.Release, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		return nil, err
	}

	// Add or update chart repo starts
	chartRepoRequest := request.ChartRepository
	chartRepoName := chartRepoRequest.Name
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
	// Add or update chart repo ends

	// Install release starts
	chartSpec := &helmClient.ChartSpec{
		ReleaseName:      releaseIdentifier.ReleaseName,
		Namespace:        releaseIdentifier.ReleaseNamespace,
		ValuesYaml:       request.ValuesYaml,
		ChartName:        fmt.Sprintf("%s/%s", chartRepoName, request.ChartName),
		Version:          request.ChartVersion,
		DependencyUpdate: true,
		UpgradeCRDs:      true,
		CreateNamespace:  true,
		DryRun:           dryRun,
	}

	impl.logger.Debugw("Installing release", "name", releaseIdentifier.ReleaseName, "namespace", releaseIdentifier.ReleaseNamespace, "dry-run", dryRun)
	rel, err := helmClientObj.InstallChart(context.Background(), chartSpec)
	if err != nil {
		impl.logger.Errorw("Error in install release ", "err", err)
		return nil, err
	}
	// Install release ends
	return rel, nil
}

// 1. run this method using main
// 2. write unit test case
// 3. expose this method over grpc
// 4. write rest handler, router, and servie in orchestrator
// 5 .invoke this method using grpc
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

	// Add or update chart repo starts
	chartRepoRequest := request.ChartRepository
	chartRepoName := chartRepoRequest.Name
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
	// Add or update chart repo ends

	// Update release starts
	chartSpec := &helmClient.ChartSpec{
		ReleaseName:      releaseIdentifier.ReleaseName,
		Namespace:        releaseIdentifier.ReleaseNamespace,
		ValuesYaml:       request.ValuesYaml,
		ChartName:        fmt.Sprintf("%s/%s", chartRepoName, request.ChartName),
		Version:          request.ChartVersion,
		DependencyUpdate: true,
		UpgradeCRDs:      true,
	}

	impl.logger.Debug("Upgrading release with chart info")
	_, err = helmClientObj.UpgradeReleaseWithChartInfo(context.Background(), chartSpec)
	if err != nil {
		impl.logger.Errorw("Error in upgrade release with chart info", "err", err)
		return nil, err
	}
	// Update release ends

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
	chartSpec := &helmClient.ChartSpec{
		ReleaseName:   releaseIdentifier.ReleaseName,
		Namespace:     releaseIdentifier.ReleaseNamespace,
		ChartName:     request.ChartName,
		CleanupOnFail: true, // allow deletion of new resources created in this rollback when rollback fails
		MaxHistory:    0,    // limit the maximum number of revisions saved per release. Use 0 for no limit (default 10)
		RepoURL:       request.ChartRepository.Url,
	}

	HelmTemplateOptions := &helmClient.HelmTemplateOptions{}
	if request.K8SVersion != "" {
		HelmTemplateOptions.KubeVersion = &chartutil.KubeVersion{
			Version: request.K8SVersion,
		}
	}
	rel, err := helmClientObj.TemplateChart(chartSpec, HelmTemplateOptions)
	if err != nil {
		impl.logger.Errorw("error occured while generating manifest in helm app service", "err:", err)
		return "", err
	}
	if rel == nil {
		return "", errors.New("release is found nil")
	}
	return string(rel), nil
}

func getHelmRelease(clusterConfig *client.ClusterConfig, namespace string, releaseName string) (*release.Release, error) {
	conf, err := k8sUtils.GetRestConfig(clusterConfig)
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

func getHelmReleaseHistory(clusterConfig *client.ClusterConfig, releaseNamespace string, releaseName string) ([]*release.Release, error) {
	conf, err := k8sUtils.GetRestConfig(clusterConfig)
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

	releases, err := helmClient.ListReleaseHistory(releaseName, 20)
	if err != nil {
		return nil, err
	}

	return releases, nil
}

func buildReleaseInfoBasicData(helmRelease *release.Release) (*client.ReleaseInfo, error) {
	defaultValues := helmRelease.Chart.Values
	overrideValues := helmRelease.Config
	var mergedValues map[string]interface{}
	if overrideValues == nil {
		mergedValues = defaultValues
	} else {
		defaultValuesByteArr, err := json.Marshal(defaultValues)
		if err != nil {
			return nil, err
		}
		overrideValuesByteArr, err := json.Marshal(overrideValues)
		if err != nil {
			return nil, err
		}
		mergedValuesByteArr, err := jsonpatch.MergePatch(defaultValuesByteArr, overrideValuesByteArr)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(mergedValuesByteArr, &mergedValues)
		if err != nil {
			return nil, err
		}
	}
	defaultValString, err := json.Marshal(defaultValues)
	if err != nil {
		return nil, err
	}
	overrideValuesString, err := json.Marshal(overrideValues)
	if err != nil {
		return nil, err
	}
	mergedValuesString, err := json.Marshal(mergedValues)
	if err != nil {
		return nil, err
	}
	var readme string
	for _, file := range helmRelease.Chart.Files {
		if file.Name == "README.md" {
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

func (impl *HelmAppServiceImpl) getNodes(appDetailRequest *client.AppDetailRequest, release *release.Release) ([]*bean.ResourceNode, error) {
	conf, err := k8sUtils.GetRestConfig(appDetailRequest.ClusterConfig)
	if err != nil {
		return nil, err
	}
	manifests, err := util.SplitYAMLs([]byte(release.Manifest))
	if err != nil {
		return nil, err
	}
	// get live manifests from kubernetes
	desiredOrLiveManifests, err := impl.getDesiredOrLiveManifests(conf, manifests, appDetailRequest.Namespace)
	if err != nil {
		return nil, err
	}
	// build resource nodes
	nodes, err := impl.buildNodes(conf, desiredOrLiveManifests, appDetailRequest.Namespace, nil)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (impl HelmAppServiceImpl) buildResourceTree(appDetailRequest *client.AppDetailRequest, release *release.Release) (*bean.ResourceTreeResponse, error) {
	conf, err := k8sUtils.GetRestConfig(appDetailRequest.ClusterConfig)
	if err != nil {
		return nil, err
	}
	manifests, err := util.SplitYAMLs([]byte(release.Manifest))
	if err != nil {
		return nil, err
	}
	// get live manifests from kubernetes
	desiredOrLiveManifests, err := impl.getDesiredOrLiveManifests(conf, manifests, appDetailRequest.Namespace)
	if err != nil {
		return nil, err
	}
	// build resource nodes
	nodes, err := impl.buildNodes(conf, desiredOrLiveManifests, appDetailRequest.Namespace, nil)
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

	var filteredNodes []*bean.ResourceNode

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

	var desiredOrLiveManifests []*bean.DesiredOrLiveManifest
	for _, desiredManifest := range desiredManifests {
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
		desiredOrLiveManifests = append(desiredOrLiveManifests, desiredOrLiveManifest)
	}

	return desiredOrLiveManifests, nil
}

func (impl HelmAppServiceImpl) buildNodes(restConfig *rest.Config, desiredOrLiveManifests []*bean.DesiredOrLiveManifest, releaseNamespace string, parentResourceRef *bean.ResourceRef) ([]*bean.ResourceNode, error) {
	var nodes []*bean.ResourceNode
	for _, desiredOrLiveManifest := range desiredOrLiveManifests {
		manifest := desiredOrLiveManifest.Manifest
		gvk := manifest.GroupVersionKind()

		_namespace := manifest.GetNamespace()
		if _namespace == "" {
			_namespace = releaseNamespace
		}

		resourceRef := buildResourceRef(gvk, *manifest, _namespace)

		if impl.k8sService.CanHaveChild(gvk) {
			children, err := impl.k8sService.GetChildObjects(restConfig, _namespace, gvk, manifest.GetName(), manifest.GetAPIVersion())
			if err != nil {
				return nil, err
			}
			var desiredOrLiveManifestsChildren []*bean.DesiredOrLiveManifest
			for _, child := range children {
				desiredOrLiveManifestsChildren = append(desiredOrLiveManifestsChildren, &bean.DesiredOrLiveManifest{
					Manifest: child,
				})
			}
			childNodes, err := impl.buildNodes(restConfig, desiredOrLiveManifestsChildren, releaseNamespace, resourceRef)
			if err != nil {
				return nil, err
			}

			for _, childNode := range childNodes {
				nodes = append(nodes, childNode)
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
			if k8sUtils.IsService(gvk) && node.Name == k8sUtils.DEVTRON_SERVICE_NAME && k8sUtils.IsDevtronApp(node.NetworkingInfo.Labels) {
				node.Health = &bean.HealthStatus{
					Status: bean.HealthStatusHealthy,
				}
			} else {
				if healthCheck := gitops_engine.GetHealthCheckFunc(gvk); healthCheck != nil {
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
	}

	return nodes, nil
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
	var podsMetadata []*bean.PodMetadata
	for _, node := range nodes {
		if node.Kind != kube.PodKind {
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
			if parentKind == kube.StatefulSetKind {
				var statefulSet appsV1.StatefulSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &statefulSet)
				if err != nil {
					return nil, err
				}
				isNew = statefulSet.Status.UpdateRevision == pod.GetLabels()["controller-revision-hash"]
			}

			// if parent is Job - then pod label controller-revision-hash should match StatefulSet's update revision
			if parentKind == kube.JobKind {
				//TODO - new or old logic not built in orchestrator for Job's pods. hence not implementing here. as don't know the logic :)
				isNew = true
			}

			// if parent kind is replica set then
			if parentKind == kube.ReplicaSetKind {
				var replicaSet appsV1.ReplicaSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &replicaSet)
				if err != nil {
					return nil, err
				}
				replicaSetNode := getMatchingNode(nodes, parentKind, replicaSet.Name)

				// if parent of replicaset is deployment, compare label pod-template-hash
				if replicaSetNode != nil && len(replicaSetNode.ParentRefs) > 0 && replicaSetNode.ParentRefs[0].Kind == kube.DeploymentKind {
					isNew = replicaSet.GetLabels()["pod-template-hash"] == pod.GetLabels()["pod-template-hash"]
				}
			}

			// if parent kind is DaemonSet then compare DaemonSet's Child ControllerRevision's label controller-revision-hash with pod label controller-revision-hash
			if parentKind == kube.DaemonSetKind {
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
		var containerNames []string
		var initContainerNames []string
		var ephemeralContainers []string
		for _, container := range pod.Spec.Containers {
			containerNames = append(containerNames, container.Name)
		}
		for _, initContainer := range pod.Spec.InitContainers {
			initContainerNames = append(initContainerNames, initContainer.Name)
		}

		for _, ephemeralContainer := range pod.Spec.EphemeralContainers {
			ephemeralContainers = append(ephemeralContainers, ephemeralContainer.Name)
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

func getMatchingNode(nodes []*bean.ResourceNode, kind string, name string) *bean.ResourceNode {
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return node
		}
	}
	return nil
}

func getMatchingNodes(nodes []*bean.ResourceNode, kind string) []*bean.ResourceNode {
	var nodesRes []*bean.ResourceNode
	for _, node := range nodes {
		if node.Kind == kind {
			nodesRes = append(nodesRes, node)
		}
	}
	return nodesRes
}

func (impl HelmAppServiceImpl) getHelmClient(clusterConfig *client.ClusterConfig, releaseNamespace string) (helmClient.Client, error) {
	conf, err := k8sUtils.GetRestConfig(clusterConfig)
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
