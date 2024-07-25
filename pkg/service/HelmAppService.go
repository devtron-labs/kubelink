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

package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	registry2 "github.com/devtron-labs/common-lib/helmLib/registry"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	k8sObjectUtils "github.com/devtron-labs/common-lib/utils/k8sObjectsUtil"
	"github.com/devtron-labs/common-lib/workerPool"
	globalConfig "github.com/devtron-labs/kubelink/config"
	"github.com/devtron-labs/kubelink/converter"
	error2 "github.com/devtron-labs/kubelink/error"
	"github.com/devtron-labs/kubelink/pkg/cache"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"net/url"
	"path"
	"strings"
	"time"

	"sync"

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
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

const (
	hibernateReplicaAnnotation            = "hibernator.devtron.ai/replicas"
	hibernatePatch                        = `[{"op": "replace", "path": "/spec/replicas", "value":%d}, {"op": "add", "path": "/metadata/annotations", "value": {"%s":"%s"}}]`
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
	FetchApplicationStatus(req *client.AppDetailRequest) (*client.AppStatus, error)
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
	TemplateChart(ctx context.Context, request *client.InstallReleaseRequest, getChart bool) (string, []byte, error)
	TemplateChartBulk(ctx context.Context, request []*client.InstallReleaseRequest) (map[string]string, error)
	InstallReleaseWithCustomChart(ctx context.Context, req *client.HelmInstallCustomRequest) (bool, error)
	GetNotes(ctx context.Context, installReleaseRequest *client.InstallReleaseRequest) (string, error)
	UpgradeReleaseWithCustomChart(ctx context.Context, request *client.UpgradeReleaseRequest) (bool, error)
	// ValidateOCIRegistryLogin Validates the OCI registry credentials by login
	ValidateOCIRegistryLogin(ctx context.Context, OCIRegistryRequest *client.RegistryCredential) (*client.OCIRegistryResponse, error)
	// PushHelmChartToOCIRegistryRepo Pushes the helm chart to the OCI registry and returns the generated digest and pushedUrl
	PushHelmChartToOCIRegistryRepo(ctx context.Context, OCIRegistryRequest *client.OCIRegistryRequest) (*client.OCIRegistryResponse, error)
	GetResourceTreeForExternalResources(req *client.ExternalResourceTreeRequest) (*bean.ResourceTreeResponse, error)
}

type HelmAppServiceImpl struct {
	logger            *zap.SugaredLogger
	k8sService        K8sService
	randSource        rand.Source
	K8sInformer       k8sInformer.K8sInformer
	helmReleaseConfig *globalConfig.HelmReleaseConfig
	k8sUtil           k8sUtils.K8sService
	pubsubClient      *pubsub_lib.PubSubClientServiceImpl
	clusterRepository repository.ClusterRepository
	converter         converter.ClusterBeanConverter
	registrySettings  registry2.SettingsFactory
}

func NewHelmAppServiceImpl(logger *zap.SugaredLogger, k8sService K8sService,
	k8sInformer k8sInformer.K8sInformer, helmReleaseConfig *globalConfig.HelmReleaseConfig,
	k8sUtil k8sUtils.K8sService, converter converter.ClusterBeanConverter,
	clusterRepository repository.ClusterRepository,
	registrySettings registry2.SettingsFactory,
) (*HelmAppServiceImpl, error) {

	var pubsubClient *pubsub_lib.PubSubClientServiceImpl
	var err error
	if helmReleaseConfig.RunHelmInstallInAsyncMode {
		pubsubClient, err = pubsub_lib.NewPubSubClientServiceImpl(logger)
		if err != nil {
			return nil, err
		}
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
		converter:         converter,
		registrySettings:  registrySettings,
	}
	err = os.MkdirAll(helmReleaseConfig.ChartWorkingDirectory, os.ModePerm)
	if err != nil {
		helmAppServiceImpl.logger.Errorw("err in creating dir", "err", err)
		return nil, err
	}

	return helmAppServiceImpl, nil
}

func (impl *HelmAppServiceImpl) CleanDir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		impl.logger.Warnw("error in deleting dir ", "dir", dir)
	}
}

func (impl *HelmAppServiceImpl) GetRandomString() string {
	/* #nosec */
	r1 := rand.New(impl.randSource).Int63()
	return strconv.FormatInt(r1, 10)
}

func (impl *HelmAppServiceImpl) getDeployedAppDetails(config *client.ClusterConfig) (deployedApps []*client.DeployedAppDetail, err error) {
	if impl.helmReleaseConfig.IsHelmReleaseCachingEnabled() {
		impl.logger.Infow("Fetching helm release using Cache")
		return impl.K8sInformer.GetAllReleaseByClusterId(int(config.GetClusterId())), nil
	}

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("error in building rest config ", "clusterId", config.ClusterId, "err", err)
		return deployedApps, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options:    &helmClient.Options{},
		RestConfig: restConfig,
	}

	helmAppClient, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		impl.logger.Errorw("error in building client from rest config ", "clusterId", config.ClusterId, "err", err)
		return deployedApps, err
	}

	impl.logger.Debug("fetching all release list from helm")
	releases, err := helmAppClient.ListAllReleases()
	if err != nil {
		impl.logger.Errorw("error in getting releases list ", "clusterId", config.ClusterId, "err", err)
		return deployedApps, err
	}

	for _, item := range releases {
		deployedApps = append(deployedApps, NewDeployedAppDetail(config, item))
	}
	return deployedApps, nil
}

func (impl *HelmAppServiceImpl) GetApplicationListForCluster(config *client.ClusterConfig) *client.DeployedAppList {
	impl.logger.Debugw("Fetching application list ", "clusterId", config.ClusterId, "clusterName", config.ClusterName)
	deployedApp := &client.DeployedAppList{ClusterId: config.GetClusterId()}
	deployedApps, err := impl.getDeployedAppDetails(config)
	if err != nil {
		impl.logger.Errorw("error in getting deployed app details", "clusterId", config.ClusterId, "err", err)
		deployedApp.Errored = true
		deployedApp.ErrorMsg = err.Error()
		return deployedApp
	}
	deployedApp.DeployedAppDetail = deployedApps
	return deployedApp
}

func (impl *HelmAppServiceImpl) GetResourceTreeForExternalResources(req *client.ExternalResourceTreeRequest) (*bean.ResourceTreeResponse, error) {
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(req.ClusterConfig)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting RestConfig", "err", err)
		return nil, err
	}

	manifests := impl.getManifestsForExternalResources(restConfig, req.ExternalResourceDetail)
	// build resource Nodes
	buildNodesRequest := NewBuildNodesRequest(NewBuildNodesConfig(restConfig)).
		WithDesiredOrLiveManifests(manifests...).
		WithBatchWorker(impl.helmReleaseConfig.BuildNodesBatchSize, impl.logger)
	buildNodesResponse, err := impl.buildNodes(buildNodesRequest)
	if err != nil {
		impl.logger.Errorw("error in building Nodes", "err", err)
		return nil, err
	}
	// build pods metadata
	podsMetadata, err := impl.buildPodMetadata(buildNodesResponse.Nodes, restConfig)
	if err != nil {
		return nil, err
	}
	resourceTreeResponse := &bean.ResourceTreeResponse{
		ApplicationTree: &bean.ApplicationTree{
			Nodes: buildNodesResponse.Nodes,
		},
		PodMetadata: podsMetadata,
	}
	return resourceTreeResponse, nil
}

func (impl *HelmAppServiceImpl) getManifestsForExternalResources(restConfig *rest.Config, externalResourceDetails []*client.ExternalResourceDetail) []*bean.DesiredOrLiveManifest {
	var manifests []*bean.DesiredOrLiveManifest
	for _, resource := range externalResourceDetails {
		gvk := &schema.GroupVersionKind{
			Group:   resource.GetGroup(),
			Version: resource.GetVersion(),
			Kind:    resource.GetKind(),
		}
		manifest, _, err := impl.k8sService.GetLiveManifest(restConfig, resource.GetNamespace(), gvk, resource.GetName())
		if err != nil {
			impl.logger.Errorw("Error in getting live manifest", "err", err)
			statusError, _ := err.(*errors2.StatusError)
			desiredManifest := &unstructured.Unstructured{}
			desiredManifest.SetGroupVersionKind(*gvk)
			desiredManifest.SetName(resource.Name)
			desiredManifest.SetNamespace(resource.Namespace)
			desiredOrLiveManifest := &bean.DesiredOrLiveManifest{
				Manifest: desiredManifest,
				// using deep copy as it replaces item in manifest in loop
				IsLiveManifestFetchError: true,
			}
			if statusError != nil {
				desiredOrLiveManifest.LiveManifestFetchErrorCode = statusError.Status().Code
			}
			manifests = append(manifests, desiredOrLiveManifest)
		} else {
			manifests = append(manifests, &bean.DesiredOrLiveManifest{
				Manifest: manifest,
			})
		}
	}
	return manifests
}

func (impl *HelmAppServiceImpl) BuildAppDetail(req *client.AppDetailRequest) (*bean.AppDetail, error) {
	helmRelease, err := impl.getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return &bean.AppDetail{ReleaseExists: false}, err
		}
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
		return nil, err
	}
	resourceTreeResponse, err := impl.buildResourceTree(req, helmRelease)
	if err != nil {
		impl.logger.Errorw("error in building resource tree ", "err", err)
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

//
// func (k *Resource) action(resource *clustercache.Resource, _ map[kube.ResourceKey]*clustercache.Resource) bool {
//	if resourceNode, ok := resource.Info.(*bean.ResourceNode); ok {
//		k.UidToResourceRefMapping[resourceNode.UID] = resourceNode.ResourceRef
//	}
//	k.CacheResources = append(k.CacheResources, resource)
//	return true
// }

func (impl *HelmAppServiceImpl) addHookResourcesInManifest(helmRelease *release.Release, manifests []unstructured.Unstructured) []unstructured.Unstructured {
	for _, helmHook := range helmRelease.Hooks {
		var hook unstructured.Unstructured
		err := yaml.Unmarshal([]byte(helmHook.Manifest), &hook)
		if err != nil {
			impl.logger.Errorw("error in converting string manifest into unstructured obj", "hookName", helmHook.Name, "releaseName", helmRelease.Name, "err", err)
			continue
		}
		manifests = append(manifests, hook)
	}
	return manifests
}

func (impl *HelmAppServiceImpl) getLiveManifests(config *rest.Config, helmRelease *release.Release) ([]*bean.DesiredOrLiveManifest, error) {
	manifests, err := yamlUtil.SplitYAMLs([]byte(helmRelease.Manifest))
	manifests = impl.addHookResourcesInManifest(helmRelease, manifests)
	// get live manifests from kubernetes
	desiredOrLiveManifests, err := impl.getDesiredOrLiveManifests(config, manifests, helmRelease.Namespace)
	if err != nil {
		impl.logger.Errorw("error in getting desired or live manifest", "host", config.Host, "helmReleaseName", helmRelease.Name, "err", err)
		return nil, err
	}
	return desiredOrLiveManifests, nil
}

func (impl *HelmAppServiceImpl) getRestConfigForClusterConfig(clusterConfig *client.ClusterConfig) (*rest.Config, error) {
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(clusterConfig)
	conf, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting rest config by cluster", "clusterName", k8sClusterConfig.ClusterName)
		return nil, err
	}
	return conf, nil
}

// func getNodeInfoFromHierarchy(Node *clustercache.Resource, uidToResourceRefMapping map[string]*bean.ResourceRef) *bean.ResourceNode {
//	resourceNode := &bean.ResourceNode{}
//	var ok bool
//	if resourceNode, ok = Node.Info.(*bean.ResourceNode); ok {
//		if Node.OwnerRefs != nil {
//			parentRefs := make([]*bean.ResourceRef, 0)
//			for _, ownerRef := range Node.OwnerRefs {
//				parentRefs = append(parentRefs, uidToResourceRefMapping[string(ownerRef.UID)])
//			}
//			resourceNode.ParentRefs = parentRefs
//		}
//	}
//	return resourceNode
// }

func (impl *HelmAppServiceImpl) FetchApplicationStatus(req *client.AppDetailRequest) (*client.AppStatus, error) {
	helmAppStatus := &client.AppStatus{}
	helmRelease, err := impl.getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
		return helmAppStatus, err
	}
	if helmRelease.Info != nil {
		helmAppStatus.Description = helmRelease.Info.Description
		helmAppStatus.ReleaseStatus = string(helmRelease.Info.Status)
		helmAppStatus.LastDeployed = timestamppb.New(helmRelease.Info.LastDeployed.Time)
	}
	_, healthStatusArray, err := impl.getNodes(req, helmRelease)
	if err != nil {
		impl.logger.Errorw("Error in getting Nodes", "err", err, "req", req)
		return helmAppStatus, err
	}
	// getting app status on basis of healthy/non-healthy as this api is used for deployment status
	// in orchestrator and not for app status
	helmAppStatus.ApplicationStatus = *util.GetAppStatusOnBasisOfHealthyNonHealthy(healthStatusArray)
	return helmAppStatus, nil
}

func (impl *HelmAppServiceImpl) GetHelmAppValues(req *client.AppDetailRequest) (*client.ReleaseInfo, error) {

	helmRelease, err := impl.getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
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

func (impl *HelmAppServiceImpl) ScaleObjects(ctx context.Context, clusterConfig *client.ClusterConfig, objects []*client.ObjectIdentifier, scaleDown bool) (*client.HibernateResponse, error) {
	response := &client.HibernateResponse{}
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(clusterConfig)
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

func (impl *HelmAppServiceImpl) GetDeploymentHistory(req *client.AppDetailRequest) (*client.HelmAppDeploymentHistory, error) {
	helmReleases, err := impl.getHelmReleaseHistory(req.ClusterConfig, req.Namespace, req.ReleaseName, impl.helmReleaseConfig.MaxCountForHelmRelease)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release history ", "err", err)
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
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
		if helmRelease.Info != nil {
			deploymentDetail.DeployedAt = timestamppb.New(helmRelease.Info.LastDeployed.Time)
			deploymentDetail.Status = string(helmRelease.Info.Status)
		}
		helmAppDeployments = append(helmAppDeployments, deploymentDetail)
	}
	return &client.HelmAppDeploymentHistory{DeploymentHistory: helmAppDeployments}, nil
}

func (impl *HelmAppServiceImpl) GetDesiredManifest(req *client.ObjectRequest) (*client.DesiredManifestResponse, error) {
	objectIdentifier := req.ObjectIdentifier
	helmRelease, err := impl.getHelmRelease(req.ClusterConfig, req.ReleaseNamespace, req.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release ", "err", err)
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
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

func (impl *HelmAppServiceImpl) UninstallRelease(releaseIdentifier *client.ReleaseIdentifier) (*client.UninstallReleaseResponse, error) {
	helmClient, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		return nil, err
	}

	err = helmClient.UninstallReleaseByName(releaseIdentifier.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in uninstall release ", "err", err)
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
		return nil, err
	}

	uninstallReleaseResponse := &client.UninstallReleaseResponse{
		Success: true,
	}

	return uninstallReleaseResponse, nil
}

func (impl *HelmAppServiceImpl) UpgradeRelease(ctx context.Context, request *client.UpgradeReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	upgradeReleaseResponse := &client.UpgradeReleaseResponse{
		Success: true,
	}
	if request.ChartContent == nil {
		// external helm app upgrade flow
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

		helmRelease, err := impl.getHelmRelease(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace, releaseIdentifier.ReleaseName)
		if err != nil {
			impl.logger.Errorw("Error in getting helm release ", "err", err)
			internalErr := error2.ConvertHelmErrorToInternalError(err)
			if internalErr != nil {
				err = internalErr
			}
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
			internalErr := error2.ConvertHelmErrorToInternalError(err)
			if internalErr != nil {
				err = internalErr
			}
			return nil, err
		}

		return upgradeReleaseResponse, nil
	}
	// handling for running helm Upgrade operation with context in Devtron app; used in Async Install mode
	switch request.RunInCtx {
	case true:
		res, err := impl.UpgradeReleaseWithCustomChart(ctx, request)
		if err != nil {
			impl.logger.Errorw("Error in upgrade release ", "err", err)
			return nil, err
		}
		upgradeReleaseResponse.Success = res
	case false:
		res, err := impl.UpgradeReleaseWithCustomChart(ctx, request)
		if err != nil {
			impl.logger.Errorw("Error in upgrade release ", "err", err)
			return nil, err
		}
		upgradeReleaseResponse.Success = res
	}
	return upgradeReleaseResponse, nil
}

func (impl *HelmAppServiceImpl) GetDeploymentDetail(request *client.DeploymentDetailRequest) (*client.DeploymentDetailResponse, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmReleases, err := impl.getHelmReleaseHistory(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace, releaseIdentifier.ReleaseName, impl.helmReleaseConfig.MaxCountForHelmRelease)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release history ", "err", err)
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
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

func (impl *HelmAppServiceImpl) InstallRelease(ctx context.Context, request *client.InstallReleaseRequest) (*client.InstallReleaseResponse, error) {
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

func parseOCIChartName(registryUrl, repoName string) (string, error) {
	// helm package expects chart name to be in this format
	if !strings.Contains(strings.ToLower(registryUrl), "https") && !strings.Contains(strings.ToLower(registryUrl), "http") {
		registryUrl = fmt.Sprintf("//%s", registryUrl)
	}
	registryUrl = strings.TrimSpace(registryUrl)
	parsedUrl, err := url.Parse(registryUrl)
	if err != nil {
		return registryUrl, err
	}
	var chartName string
	parsedUrlPath := strings.TrimSpace(parsedUrl.Path)
	parsedHost := strings.TrimSpace(parsedUrl.Host)
	parsedRepoName := strings.TrimSpace(repoName)
	// Join handles empty strings
	uri := filepath.Join(parsedHost, parsedUrlPath, parsedRepoName)
	chartName = fmt.Sprintf("%s://%s", "oci", uri)
	return chartName, nil
}

func (impl *HelmAppServiceImpl) installRelease(ctx context.Context, request *client.InstallReleaseRequest, dryRun bool) (*release.Release, error) {

	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		impl.logger.Errorw("error in creating helm client object", "releaseIdentifier", releaseIdentifier, "err", err)
		return nil, err
	}

	registryConfig, err := NewRegistryConfig(request.RegistryCredential)
	defer func() {
		if registryConfig != nil {
			err := registry2.DeleteCertificateFolder(registryConfig.RegistryCAFilePath)
			if err != nil {
				impl.logger.Errorw("error in deleting certificate folder", "registryName", registryConfig.RegistryId, "err", err)
			}
		}
	}()
	if err != nil {
		impl.logger.Errorw("error in getting registry config from registry proto", "registryName", request.RegistryCredential.RegistryName, "err", err)
		return nil, err
	}
	// oci registry client
	var registryClient *registry.Client
	if registryConfig != nil {
		settingsGetter, err := impl.registrySettings.GetSettings(registryConfig)
		if err != nil {
			impl.logger.Errorw("error in getting registry settings", "registryName", request.RegistryCredential.RegistryName, "err", err)
			return nil, err
		}
		settings, err := settingsGetter.GetRegistrySettings(registryConfig)
		if err != nil {
			impl.logger.Errorw(HELM_CLIENT_ERROR, "registryName", request.RegistryCredential.RegistryName, "err", err)
			return nil, err
		}
		registryClient = settings.RegistryClient
		request.RegistryCredential.RegistryUrl = settings.RegistryHostURL
	}

	var chartName string
	switch request.IsOCIRepo {
	case true:
		chartName, err = parseOCIChartName(request.RegistryCredential.RegistryUrl, request.RegistryCredential.RepoName)
		if err != nil {
			impl.logger.Errorw("error in parsing oci chart name", "registryUrl", request.RegistryCredential.RegistryUrl, "repoName", request.RegistryCredential.RepoName, "err", err)
			return nil, err
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
			InsecureSkipTLSverify: chartRepoRequest.GetAllowInsecureConnection(),
		}
		impl.logger.Debug("Adding/Updating Chart repo")
		err = helmClientObj.AddOrUpdateChartRepo(chartRepo)
		if err != nil {
			impl.logger.Errorw("Error in add/update chart repo ", "err", err)
			internalErr := error2.ConvertHelmErrorToInternalError(err)
			if internalErr != nil {
				err = internalErr
			}
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
		CreateNamespace:  false,
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
			internalErr := error2.ConvertHelmErrorToInternalError(err)
			if internalErr != nil {
				err = internalErr
			}
			return nil, err
		}

		return rel, nil
	case true:
		go func() {
			helmInstallMessage := HelmReleaseStatusConfig{
				InstallAppVersionHistoryId: int(request.InstallAppVersionHistoryId),
			}
			// Checking release exist because there can be case when release already exist with same name
			releaseExist := impl.K8sInformer.CheckReleaseExists(releaseIdentifier.ClusterConfig.ClusterId, getUniqueReleaseIdentifierName(releaseIdentifier))
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
				return
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

func (impl *HelmAppServiceImpl) GetNotes(ctx context.Context, request *client.InstallReleaseRequest) (string, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)

	var chartName, repoURL, username, password string
	var allowInsecureConnection bool
	switch request.IsOCIRepo {
	case true:
		chartName, err = parseOCIChartName(request.RegistryCredential.RegistryUrl, request.RegistryCredential.RepoName)
		if err != nil {
			impl.logger.Errorw("error in parsing oci chart name", "registryUrl", request.RegistryCredential.RegistryUrl, "repoName", request.RegistryCredential.RepoName, "err", err)
			return "", err
		}
	case false:
		chartName = request.ChartName
		repoURL = request.ChartRepository.Url
		username = request.ChartRepository.Username
		password = request.ChartRepository.Password
		allowInsecureConnection = request.ChartRepository.AllowInsecureConnection
	}

	chartSpec := &helmClient.ChartSpec{
		ReleaseName:             releaseIdentifier.ReleaseName,
		Namespace:               releaseIdentifier.ReleaseNamespace,
		ChartName:               chartName,
		CleanupOnFail:           true, // allow deletion of new resources created in this rollback when rollback fails
		MaxHistory:              0,    // limit the maximum number of revisions saved per release. Use 0 for no limit (default 10)
		RepoURL:                 repoURL,
		Version:                 request.ChartVersion,
		ValuesYaml:              request.ValuesYaml,
		Username:                username,
		Password:                password,
		AllowInsecureConnection: allowInsecureConnection,
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
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
		return "", err
	}
	if release == nil {
		impl.logger.Errorw("no release found for", "name", releaseIdentifier.ReleaseName)
		return "", err
	}

	return string(release), nil
}

func (impl *HelmAppServiceImpl) UpgradeReleaseWithChartInfo(ctx context.Context, request *client.InstallReleaseRequest) (*client.UpgradeReleaseResponse, error) {

	releaseIdentifier := request.ReleaseIdentifier
	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		return nil, err
	}

	registryConfig, err := NewRegistryConfig(request.RegistryCredential)
	defer func() {
		if registryConfig != nil {
			err := registry2.DeleteCertificateFolder(registryConfig.RegistryCAFilePath)
			if err != nil {
				impl.logger.Errorw("error in deleting certificate folder", "registryName", registryConfig.RegistryId, "err", err)
			}
		}
	}()
	if err != nil {
		impl.logger.Errorw("error in getting registry config from registry proto", "registryName", request.RegistryCredential.RegistryName, "err", err)
		return nil, err
	}
	var registryClient *registry.Client
	if registryConfig != nil {
		settingsGetter, err := impl.registrySettings.GetSettings(registryConfig)
		if err != nil {
			impl.logger.Errorw("error in getting registry settings", "registryName", request.RegistryCredential.RegistryName, "err", err)
			return nil, err
		}
		settings, err := settingsGetter.GetRegistrySettings(registryConfig)
		if err != nil {
			impl.logger.Errorw(HELM_CLIENT_ERROR, "registryName", request.RegistryCredential.RegistryName, "err", err)
			return nil, err
		}
		registryClient = settings.RegistryClient
		request.RegistryCredential.RegistryUrl = settings.RegistryHostURL
	}

	var chartName string
	switch request.IsOCIRepo {
	case true:
		chartName, err = parseOCIChartName(request.RegistryCredential.RegistryUrl, request.RegistryCredential.RepoName)
		if err != nil {
			impl.logger.Errorw("error in parsing oci chart name", "registryUrl", request.RegistryCredential.RegistryUrl, "repoName", request.RegistryCredential.RepoName, "err", err)
			return nil, err
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
			InsecureSkipTLSverify: chartRepoRequest.GetAllowInsecureConnection(),
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
				if util.IsReleaseNotFoundError(UpgradeErr.Err) {
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
					if util.IsReleaseNotFoundError(UpgradeErr.Err) {
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

func (impl *HelmAppServiceImpl) IsReleaseInstalled(ctx context.Context, releaseIdentifier *client.ReleaseIdentifier) (bool, error) {
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

func (impl *HelmAppServiceImpl) RollbackRelease(request *client.RollbackReleaseRequest) (bool, error) {
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

func (impl *HelmAppServiceImpl) TemplateChartBulk(ctx context.Context, request []*client.InstallReleaseRequest) (map[string]string, error) {
	manifestResponse := make(map[string]string)
	for _, req := range request {
		manifest, _, err := impl.TemplateChart(ctx, req, false)
		if err != nil {
			impl.logger.Errorw("error in fetching template chart", "req", req, "err", err)
			return nil, err
		}
		manifestResponse[req.AppName] = manifest
	}
	return manifestResponse, nil
}

func (impl *HelmAppServiceImpl) TemplateChart(ctx context.Context, request *client.InstallReleaseRequest, getChart bool) (string, []byte, error) {

	releaseIdentifier := request.ReleaseIdentifier

	helmClientObj, err := impl.getHelmClient(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace)
	if err != nil {
		impl.logger.Errorw("error in getting helm app client")
		return "", nil, err
	}

	registryConfig, err := NewRegistryConfig(request.RegistryCredential)
	defer func() {
		if registryConfig != nil {
			err := registry2.DeleteCertificateFolder(registryConfig.RegistryCAFilePath)
			if err != nil {
				impl.logger.Errorw("error in deleting certificate folder", "registryName", registryConfig.RegistryId, "err", err)
			}
		}
	}()
	if err != nil {
		impl.logger.Errorw("error in getting registry config from registry proto", "registryName", request.RegistryCredential.RegistryName, "err", err)
		return "", nil, err
	}

	var registryClient *registry.Client
	if registryConfig != nil {
		settingsGetter, err := impl.registrySettings.GetSettings(registryConfig)
		if err != nil {
			impl.logger.Errorw("error in getting registry settings", "registryName", request.RegistryCredential.RegistryName, "err", err)
			return "", nil, err
		}
		settings, err := settingsGetter.GetRegistrySettings(registryConfig)
		if err != nil {
			impl.logger.Errorw(HELM_CLIENT_ERROR, "registryName", request.RegistryCredential.RegistryName, "err", err)
			return "", nil, err
		}
		registryClient = settings.RegistryClient
		request.RegistryCredential.RegistryUrl = settings.RegistryHostURL
	}

	var chartName, repoURL, username, password string
	var allowInsecureConnection bool
	switch request.IsOCIRepo {
	case true:
		chartName, err = parseOCIChartName(request.RegistryCredential.RegistryUrl, request.RegistryCredential.RepoName)
		if err != nil {
			impl.logger.Errorw("error in parsing oci chart name", "registryUrl", request.RegistryCredential.RegistryUrl, "repoName", request.RegistryCredential.RepoName, "err", err)
			return "", nil, err
		}
	case false:
		chartName = request.ChartName
		repoURL = request.ChartRepository.Url
		username = request.ChartRepository.Username
		password = request.ChartRepository.Password
		allowInsecureConnection = request.ChartRepository.AllowInsecureConnection
	}

	chartSpec := &helmClient.ChartSpec{
		ReleaseName:             releaseIdentifier.ReleaseName,
		Namespace:               releaseIdentifier.ReleaseNamespace,
		ChartName:               chartName,
		CleanupOnFail:           true, // allow deletion of new resources created in this rollback when rollback fails
		MaxHistory:              0,    // limit the maximum number of revisions saved per release. Use 0 for no limit (default 10)
		RepoURL:                 repoURL,
		Version:                 request.ChartVersion,
		ValuesYaml:              request.ValuesYaml,
		RegistryClient:          registryClient,
		Username:                username,
		Password:                password,
		AllowInsecureConnection: allowInsecureConnection,
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
	rel, chartBytes, err := helmClientObj.TemplateChart(chartSpec, HelmTemplateOptions, content, getChart)
	if err != nil {
		impl.logger.Errorw("error occured while generating manifest in helm app service", "err:", err)
		return "", nil, err
	}

	if rel == nil {
		return "", nil, errors.New("release is found nil")
	}
	return string(rel), chartBytes, nil
}

func (impl *HelmAppServiceImpl) getHelmRelease(clusterConfig *client.ClusterConfig, namespace string, releaseName string) (*release.Release, error) {

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(clusterConfig)
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

func (impl *HelmAppServiceImpl) getHelmReleaseHistory(clusterConfig *client.ClusterConfig, releaseNamespace string, releaseName string, countOfHelmReleaseHistory int) ([]*release.Release, error) {
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(clusterConfig)
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
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(appDetailRequest.ClusterConfig)
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
	// build resource Nodes
	req := NewBuildNodesRequest(NewBuildNodesConfig(conf).
		WithReleaseNamespace(appDetailRequest.Namespace)).
		WithDesiredOrLiveManifests(desiredOrLiveManifests...).
		WithBatchWorker(impl.helmReleaseConfig.BuildNodesBatchSize, impl.logger)
	buildNodesResponse, err := impl.buildNodes(req)
	if err != nil {
		return nil, nil, err
	}
	return buildNodesResponse.Nodes, buildNodesResponse.HealthStatusArray, nil
}

func (impl *HelmAppServiceImpl) buildResourceTree(appDetailRequest *client.AppDetailRequest, release *release.Release) (*bean.ResourceTreeResponse, error) {
	conf, err := impl.getRestConfigForClusterConfig(appDetailRequest.ClusterConfig)
	if err != nil {
		return nil, err
	}
	desiredOrLiveManifests, err := impl.getLiveManifests(conf, release)
	if err != nil {
		return nil, err
	}
	// build resource Nodes
	req := NewBuildNodesRequest(NewBuildNodesConfig(conf).
		WithReleaseNamespace(appDetailRequest.Namespace)).
		WithDesiredOrLiveManifests(desiredOrLiveManifests...).
		WithBatchWorker(impl.helmReleaseConfig.BuildNodesBatchSize, impl.logger)
	buildNodesResponse, err := impl.buildNodes(req)
	if err != nil {
		return nil, err
	}
	updateHookInfoForChildNodes(buildNodesResponse.Nodes)

	// filter Nodes based on ResourceTreeFilter
	resourceTreeFilter := appDetailRequest.ResourceTreeFilter
	if resourceTreeFilter != nil && len(buildNodesResponse.Nodes) > 0 {
		buildNodesResponse.Nodes = impl.filterNodes(resourceTreeFilter, buildNodesResponse.Nodes)
	}

	// build pods metadata
	podsMetadata, err := impl.buildPodMetadata(buildNodesResponse.Nodes, conf)
	if err != nil {
		return nil, err
	}
	resourceTreeResponse := &bean.ResourceTreeResponse{
		ApplicationTree: &bean.ApplicationTree{
			Nodes: buildNodesResponse.Nodes,
		},
		PodMetadata: podsMetadata,
	}
	return resourceTreeResponse, nil
}

func updateHookInfoForChildNodes(nodes []*bean.ResourceNode) {
	hookUidToHookTypeMap := make(map[string]string)
	for _, node := range nodes {
		if node.IsHook {
			hookUidToHookTypeMap[node.UID] = node.HookType
		}
	}
	// if Node's parentRef is a hook then add hook info in child Node also
	if len(hookUidToHookTypeMap) > 0 {
		for _, node := range nodes {
			if node.ParentRefs != nil && len(node.ParentRefs) > 0 {
				if hookType, ok := hookUidToHookTypeMap[node.ParentRefs[0].UID]; ok {
					node.IsHook = true
					node.HookType = hookType
				}
			}
		}
	}
}

func (impl *HelmAppServiceImpl) filterNodes(resourceTreeFilter *client.ResourceTreeFilter, nodes []*bean.ResourceNode) []*bean.ResourceNode {
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

func (impl *HelmAppServiceImpl) getDesiredOrLiveManifests(restConfig *rest.Config, desiredManifests []unstructured.Unstructured, releaseNamespace string) ([]*bean.DesiredOrLiveManifest, error) {

	totalManifestCount := len(desiredManifests)
	desiredOrLiveManifestArray := make([]*bean.DesiredOrLiveManifest, totalManifestCount)
	batchSize := impl.helmReleaseConfig.ManifestFetchBatchSize

	for i := 0; i < totalManifestCount; {
		// requests left to process
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

func (impl *HelmAppServiceImpl) getManifestData(restConfig *rest.Config, releaseNamespace string, desiredManifest unstructured.Unstructured) *bean.DesiredOrLiveManifest {
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

func (impl *HelmAppServiceImpl) getNodeFromDesiredOrLiveManifest(request *GetNodeFromManifest) (*GetNodeFromManifestResponse, error) {
	response := NewGetNodesFromManifestResponse()
	manifest := request.DesiredOrLiveManifest.Manifest
	gvk := manifest.GroupVersionKind()
	_namespace := manifest.GetNamespace()
	if _namespace == "" {
		_namespace = request.ReleaseNamespace
	}
	ports := util.GetPorts(manifest, gvk)
	resourceRef := buildResourceRef(gvk, *manifest, _namespace)

	if impl.k8sService.CanHaveChild(gvk) {
		children, err := impl.k8sService.GetChildObjects(request.RestConfig, _namespace, gvk, manifest.GetName(), manifest.GetAPIVersion())
		if err != nil {
			return response, err
		}
		desiredOrLiveManifestsChildren := make([]*bean.DesiredOrLiveManifest, 0, len(children))
		for _, child := range children {
			desiredOrLiveManifestsChildren = append(desiredOrLiveManifestsChildren, &bean.DesiredOrLiveManifest{
				Manifest: child,
			})
		}
		response.WithParentResourceRef(resourceRef).
			WithDesiredOrLiveManifests(desiredOrLiveManifestsChildren...)
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
	node.IsHook, node.HookType = util.GetHookMetadata(manifest)

	if request.ParentResourceRef != nil {
		node.ParentRefs = append(make([]*bean.ResourceRef, 0), request.ParentResourceRef)
	}

	// set health of Node
	if request.DesiredOrLiveManifest.IsLiveManifestFetchError {
		if request.DesiredOrLiveManifest.LiveManifestFetchErrorCode == http.StatusNotFound {
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
		cache.SetHealthStatusForNode(node, manifest, gvk)
	}

	// hibernate set starts
	if request.ParentResourceRef == nil {

		// set CanBeHibernated
		cache.SetHibernationRules(node, &node.Manifest)
	}
	// hibernate set ends

	if k8sUtils.IsPod(gvk) {
		infoItems, _ := argo.PopulatePodInfo(manifest)
		node.Info = infoItems
	}
	util.AddSelectiveInfoInResourceNode(node, gvk, manifest.Object)

	response.WithNode(node).WithHealthStatus(node.Health)
	return response, nil
}

// buildNodes builds Nodes from desired or live manifest.
//   - It uses recursive approach to build child Nodes.
//   - It uses batch worker to build child Nodes in parallel.
//   - Batch workers configuration is provided in BuildNodesConfig.
//   - NOTE: To avoid creating batch worker recursively, it does not use batch worker for child Nodes.
func (impl *HelmAppServiceImpl) buildNodes(request *BuildNodesConfig) (*BuildNodeResponse, error) {
	var buildChildNodesRequests []*BuildNodesConfig
	response := NewBuildNodeResponse()
	for _, desiredOrLiveManifest := range request.DesiredOrLiveManifests {

		// build request to get Nodes from desired or live manifest
		getNodesFromManifest := NewGetNodesFromManifest(NewBuildNodesConfig(request.RestConfig).
			WithParentResourceRef(request.ParentResourceRef).
			WithReleaseNamespace(request.ReleaseNamespace)).
			WithDesiredOrLiveManifest(desiredOrLiveManifest)

		// get Node from desired or live manifest
		getNodesFromManifestResponse, err := impl.getNodeFromDesiredOrLiveManifest(getNodesFromManifest)
		if err != nil {
			return response, err
		}
		// add Node and health status
		if getNodesFromManifestResponse.Node != nil {
			response.Nodes = append(response.Nodes, getNodesFromManifestResponse.Node)
			response.HealthStatusArray = append(response.HealthStatusArray, getNodesFromManifestResponse.Node.Health)
		}

		// add child Nodes request
		if len(getNodesFromManifestResponse.DesiredOrLiveManifests) > 0 {
			req := NewBuildNodesRequest(NewBuildNodesConfig(request.RestConfig).
				WithReleaseNamespace(request.ReleaseNamespace).
				WithParentResourceRef(getNodesFromManifestResponse.ResourceRef)).
				WithDesiredOrLiveManifests(getNodesFromManifestResponse.DesiredOrLiveManifests...)
			// NOTE:  Do not use batch worker for child Nodes as it will create batch worker recursively
			buildChildNodesRequests = append(buildChildNodesRequests, req)
		}
	}
	// build child Nodes, if any.
	// NOTE: build child Nodes calls buildNodes recursively
	childNodeResponse, err := impl.buildChildNodesInBatch(request.batchWorker, buildChildNodesRequests)
	if err != nil {
		return response, err
	}
	// add child Nodes and health status to response
	response.WithNodes(childNodeResponse.Nodes).WithHealthStatusArray(childNodeResponse.HealthStatusArray)
	return response, nil
}

// buildChildNodes builds child Nodes sequentially from desired or live manifest.
func (impl *HelmAppServiceImpl) buildChildNodes(buildChildNodesRequests []*BuildNodesConfig) (*BuildNodeResponse, error) {
	response := NewBuildNodeResponse()
	// for recursive calls, build child Nodes sequentially
	for _, req := range buildChildNodesRequests {
		// build child Nodes
		childNodesResponse, err := impl.buildNodes(req)
		if err != nil {
			impl.logger.Errorw("error in building child Nodes", "ReleaseNamespace", req.ReleaseNamespace, "parentResource", req.ParentResourceRef.GetGvk(), "err", err)
			return response, err
		}
		response.WithNodes(childNodesResponse.Nodes).WithHealthStatusArray(childNodesResponse.HealthStatusArray)
	}
	return response, nil
}

// buildChildNodesInBatch builds child Nodes in parallel from desired or live manifest.
//   - It uses batch workers workerPool.WorkerPool[*BuildNodeResponse] to build child Nodes in parallel.
//   - If workerPool is not defined, it builds child Nodes sequentially.
func (impl *HelmAppServiceImpl) buildChildNodesInBatch(wp *workerPool.WorkerPool[*BuildNodeResponse], buildChildNodesRequests []*BuildNodesConfig) (*BuildNodeResponse, error) {
	if wp == nil {
		// build child Nodes sequentially
		return impl.buildChildNodes(buildChildNodesRequests)
	}
	response := NewBuildNodeResponse()
	for index := range buildChildNodesRequests {
		// passing buildChildNodesRequests[index] to closure as it will be updated in next iteration and the func call is async
		func(req *BuildNodesConfig) {
			// submit child Nodes build request to workerPool
			wp.Submit(func() (*BuildNodeResponse, error) {
				// build child Nodes
				return impl.buildNodes(req)
			})
		}(buildChildNodesRequests[index])
	}
	// wait for all child Nodes build requests to complete
	wp.StopWait()
	// return error from workerPool error channel
	if err := wp.Error(); err != nil {
		return response, err
	}
	// extract the children nodes from workerPool response
	for _, childNode := range wp.GetResponse() {
		response.WithNodes(childNode.Nodes).WithHealthStatusArray(childNode.HealthStatusArray)
	}
	return response, nil
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

func getExtraNodeInfoMappings(nodes []*bean.ResourceNode) (map[string]string, map[string]*util.ExtraNodeInfo, map[string]*util.ExtraNodeInfo) {
	deploymentPodHashMap := make(map[string]string)
	rolloutNameVsExtraNodeInfoMapping := make(map[string]*util.ExtraNodeInfo)
	uidVsExtraNodeInfoMapping := make(map[string]*util.ExtraNodeInfo)
	for _, node := range nodes {
		if node.Kind == k8sCommonBean.DeploymentKind {
			deploymentPodHashMap[node.Name] = node.DeploymentPodHash
		} else if node.Kind == k8sCommonBean.K8sClusterResourceRolloutKind {
			rolloutNameVsExtraNodeInfoMapping[node.Name] = &util.ExtraNodeInfo{
				RolloutCurrentPodHash: node.RolloutCurrentPodHash,
			}
		} else if node.Kind == k8sCommonBean.StatefulSetKind || node.Kind == k8sCommonBean.DaemonSetKind {
			if _, ok := uidVsExtraNodeInfoMapping[node.UID]; !ok {
				uidVsExtraNodeInfoMapping[node.UID] = &util.ExtraNodeInfo{UpdateRevision: node.UpdateRevision, ResourceNetworkingInfo: node.NetworkingInfo}
			}
		}
	}
	return deploymentPodHashMap, rolloutNameVsExtraNodeInfoMapping, uidVsExtraNodeInfoMapping
}

func (impl *HelmAppServiceImpl) getReplicaSetObject(restConfig *rest.Config, replicaSetNode *bean.ResourceNode) (*v1beta1.ReplicaSet, error) {
	gvk := &schema.GroupVersionKind{
		Group:   replicaSetNode.Group,
		Version: replicaSetNode.Version,
		Kind:    replicaSetNode.Kind,
	}
	var replicaSetNodeObj map[string]interface{}
	var err error
	if replicaSetNode.Manifest.Object == nil {
		replicaSetNodeManifest, _, err := impl.k8sService.GetLiveManifest(restConfig, replicaSetNode.Namespace, gvk, replicaSetNode.Name)
		if err != nil {
			impl.logger.Errorw("error in getting replicaSet live manifest", "clusterName", restConfig.ServerName, "replicaSetName", replicaSetNode.Name)
			return nil, err
		}
		if replicaSetNodeManifest != nil {
			replicaSetNodeObj = replicaSetNodeManifest.Object
		}
	} else {
		replicaSetNodeObj = replicaSetNode.Manifest.Object
	}

	replicaSetObj, err := util.ConvertToV1ReplicaSet(replicaSetNodeObj)
	if err != nil {
		impl.logger.Errorw("error in converting replicaSet unstructured object to replicaSet object", "clusterName", restConfig.ServerName, "replicaSetName", replicaSetNode.Name)
		return nil, err
	}
	return replicaSetObj, nil
}

func (impl *HelmAppServiceImpl) getDeploymentCollisionCount(restConfig *rest.Config, deploymentInfo *bean.ResourceRef) (*int32, error) {
	parentGvk := &schema.GroupVersionKind{
		Group:   deploymentInfo.Group,
		Version: deploymentInfo.Version,
		Kind:    deploymentInfo.Kind,
	}
	var deploymentNodeObj map[string]interface{}
	var err error
	if deploymentInfo.Manifest.Object == nil {
		deploymentLiveManifest, _, err := impl.k8sService.GetLiveManifest(restConfig, deploymentInfo.Namespace, parentGvk, deploymentInfo.Name)
		if err != nil {
			impl.logger.Errorw("error in getting parent deployment live manifest", "clusterName", restConfig.ServerName, "deploymentName", deploymentInfo.Name)
			return nil, err
		}
		if deploymentLiveManifest != nil {
			deploymentNodeObj = deploymentLiveManifest.Object
		}
	} else {
		deploymentNodeObj = deploymentInfo.Manifest.Object
	}

	deploymentObj, err := util.ConvertToV1Deployment(deploymentNodeObj)
	if err != nil {
		impl.logger.Errorw("error in converting parent deployment unstructured object to replicaSet object", "clusterName", restConfig.ServerName, "deploymentName", deploymentInfo.Name)
		return nil, err
	}
	return deploymentObj.Status.CollisionCount, nil
}

func (impl *HelmAppServiceImpl) isPodNew(nodes []*bean.ResourceNode, node *bean.ResourceNode, deploymentPodHashMap map[string]string, rolloutMap map[string]*util.ExtraNodeInfo,
	uidVsExtraNodeInfoMap map[string]*util.ExtraNodeInfo, restConfig *rest.Config) (bool, error) {

	isNew := false
	parentRef := node.ParentRefs[0]
	parentKind := parentRef.Kind

	// if parent is StatefulSet - then pod label controller-revision-hash should match StatefulSet's update revision
	if parentKind == k8sCommonBean.StatefulSetKind && node.NetworkingInfo != nil {
		isNew = uidVsExtraNodeInfoMap[parentRef.UID].UpdateRevision == node.NetworkingInfo.Labels["controller-revision-hash"]
	}

	// if parent is Job - then pod label controller-revision-hash should match StatefulSet's update revision
	if parentKind == k8sCommonBean.JobKind {
		// TODO - new or old logic not built in orchestrator for Job's pods. hence not implementing here. as don't know the logic :)
		isNew = true
	}

	// if parent kind is replica set then
	if parentKind == k8sCommonBean.ReplicaSetKind {
		replicaSetNode := getMatchingNode(nodes, parentKind, parentRef.Name)

		// if parent of replicaset is deployment, compare label pod-template-hash
		if replicaSetParent := replicaSetNode.ParentRefs[0]; replicaSetNode != nil && len(replicaSetNode.ParentRefs) > 0 && replicaSetParent.Kind == k8sCommonBean.DeploymentKind {
			deploymentPodHash := deploymentPodHashMap[replicaSetParent.Name]
			replicaSetObj, err := impl.getReplicaSetObject(restConfig, replicaSetNode)
			if err != nil {
				return isNew, err
			}
			deploymentNode := getMatchingNode(nodes, replicaSetParent.Kind, replicaSetParent.Name)
			// TODO: why do we need deployment object for collisionCount ??
			var deploymentCollisionCount *int32
			if deploymentNode != nil && deploymentNode.DeploymentCollisionCount != nil {
				deploymentCollisionCount = deploymentNode.DeploymentCollisionCount
			} else {
				deploymentCollisionCount, err = impl.getDeploymentCollisionCount(restConfig, replicaSetParent)
				if err != nil {
					return isNew, err
				}
			}
			replicaSetPodHash := util.GetReplicaSetPodHash(replicaSetObj, deploymentCollisionCount)
			isNew = replicaSetPodHash == deploymentPodHash
		} else if replicaSetParent.Kind == k8sCommonBean.K8sClusterResourceRolloutKind {

			rolloutExtraInfo := rolloutMap[replicaSetParent.Name]
			rolloutPodHash := rolloutExtraInfo.RolloutCurrentPodHash
			replicasetPodHash := util.GetRolloutPodTemplateHash(replicaSetNode)

			isNew = rolloutPodHash == replicasetPodHash

		}
	}

	// if parent kind is DaemonSet then compare DaemonSet's Child ControllerRevision's label controller-revision-hash with pod label controller-revision-hash
	if parentKind == k8sCommonBean.DaemonSetKind {
		controllerRevisionNodes := getMatchingNodes(nodes, "ControllerRevision")
		for _, controllerRevisionNode := range controllerRevisionNodes {
			if len(controllerRevisionNode.ParentRefs) > 0 && controllerRevisionNode.ParentRefs[0].Kind == parentKind &&
				controllerRevisionNode.ParentRefs[0].Name == parentRef.Name && uidVsExtraNodeInfoMap[parentRef.UID].ResourceNetworkingInfo != nil &&
				node.NetworkingInfo != nil {

				isNew = uidVsExtraNodeInfoMap[parentRef.UID].ResourceNetworkingInfo.Labels["controller-revision-hash"] == node.NetworkingInfo.Labels["controller-revision-hash"]
			}
		}
	}
	return isNew, nil
}

func (impl *HelmAppServiceImpl) buildPodMetadata(nodes []*bean.ResourceNode, restConfig *rest.Config) ([]*bean.PodMetadata, error) {
	deploymentPodHashMap, rolloutMap, uidVsExtraNodeInfoMap := getExtraNodeInfoMappings(nodes)
	podsMetadata := make([]*bean.PodMetadata, 0, len(nodes))
	for _, node := range nodes {

		if node.Kind != k8sCommonBean.PodKind {
			continue
		}
		// check if pod is new
		var isNew bool
		var err error
		if len(node.ParentRefs) > 0 {
			isNew, err = impl.isPodNew(nodes, node, deploymentPodHashMap, rolloutMap, uidVsExtraNodeInfoMap, restConfig)
			if err != nil {
				return podsMetadata, err
			}
		}

		// set containers,initContainers and ephemeral container names
		var containerNames []string
		var initContainerNames []string
		var ephemeralContainersInfo []bean.EphemeralContainerInfo
		var ephemeralContainerStatus []bean.EphemeralContainerStatusesInfo

		for _, nodeInfo := range node.Info {
			switch nodeInfo.Name {
			case bean.ContainersNamesType:
				containerNames = nodeInfo.Value.([]string)
			case bean.InitContainersNamesType:
				initContainerNames = nodeInfo.Value.([]string)
			case bean.EphemeralContainersInfoType:
				ephemeralContainersInfo = nodeInfo.Value.([]bean.EphemeralContainerInfo)
			case bean.EphemeralContainersStatusType:
				ephemeralContainerStatus = nodeInfo.Value.([]bean.EphemeralContainerStatusesInfo)
			default:
				continue
			}
		}

		ephemeralContainerStatusMap := make(map[string]bool)
		for _, c := range ephemeralContainerStatus {
			// c.state contains three states running,waiting and terminated
			// at any point of time only one state will be there
			if c.State.Running != nil {
				ephemeralContainerStatusMap[c.Name] = true
			}
		}
		ephemeralContainers := make([]*bean.EphemeralContainerData, 0, len(ephemeralContainersInfo))
		// sending only running ephemeral containers in the list
		for _, ec := range ephemeralContainersInfo {
			if _, ok := ephemeralContainerStatusMap[ec.Name]; ok {
				containerData := &bean.EphemeralContainerData{
					Name:       ec.Name,
					IsExternal: k8sObjectUtils.IsExternalEphemeralContainer(ec.Command, ec.Name),
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

func (impl *HelmAppServiceImpl) getHelmClient(clusterConfig *client.ClusterConfig, releaseNamespace string) (helmClient.Client, error) {
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(clusterConfig)
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

func (impl *HelmAppServiceImpl) InstallReleaseWithCustomChart(ctx context.Context, request *client.HelmInstallCustomRequest) (bool, error) {
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

	if _, err := os.Stat(impl.helmReleaseConfig.ChartWorkingDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(impl.helmReleaseConfig.ChartWorkingDirectory, os.ModePerm)
		if err != nil {
			impl.logger.Errorw("err in creating dir", "err", err)
			return false, err
		}
	}
	dir := impl.GetRandomString()
	referenceChartDir := filepath.Join(impl.helmReleaseConfig.ChartWorkingDirectory, dir)
	referenceChartDir = fmt.Sprintf("%s.tgz", referenceChartDir)
	defer impl.CleanDir(referenceChartDir)
	err = ioutil.WriteFile(referenceChartDir, b.Bytes(), os.ModePerm)
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}
	impl.logger.Debugw("tar file write at", "referenceChartDir", referenceChartDir)
	// Install release starts
	chartSpec := &helmClient.ChartSpec{
		ReleaseName: releaseIdentifier.ReleaseName,
		Namespace:   releaseIdentifier.ReleaseNamespace,
		ValuesYaml:  request.ValuesYaml,
		ChartName:   referenceChartDir,
		KubeVersion: request.K8SVersion,
	}

	impl.logger.Debug("Installing release with chart info")
	_, err = helmClientObj.InstallChart(ctx, chartSpec)
	if err != nil {
		impl.logger.Errorw("Error in install chart", "err", err)
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
		return false, err
	}
	// Install release ends

	return true, nil
}

func (impl *HelmAppServiceImpl) UpgradeReleaseWithCustomChart(ctx context.Context, request *client.UpgradeReleaseRequest) (bool, error) {
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

	if _, err := os.Stat(impl.helmReleaseConfig.ChartWorkingDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(impl.helmReleaseConfig.ChartWorkingDirectory, os.ModePerm)
		if err != nil {
			impl.logger.Errorw("err in creating dir", "err", err)
			return false, err
		}
	}
	dir := impl.GetRandomString()
	referenceChartDir := filepath.Join(impl.helmReleaseConfig.ChartWorkingDirectory, dir)
	referenceChartDir = fmt.Sprintf("%s.tgz", referenceChartDir)
	defer impl.CleanDir(referenceChartDir)
	err = ioutil.WriteFile(referenceChartDir, b.Bytes(), os.ModePerm)
	if err != nil {
		impl.logger.Errorw("error on helm install custom while writing chartContent", "err", err)
		return false, err
	}
	impl.logger.Debugw("tar file write at", "referenceChartDir", referenceChartDir)
	// Install release spec
	installChartSpec := &helmClient.ChartSpec{
		ReleaseName: releaseIdentifier.ReleaseName,
		Namespace:   releaseIdentifier.ReleaseNamespace,
		ValuesYaml:  request.ValuesYaml,
		ChartName:   referenceChartDir,
		KubeVersion: request.K8SVersion,
	}
	// Update release spec
	updateChartSpec := installChartSpec
	updateChartSpec.MaxHistory = int(request.HistoryMax)

	impl.logger.Debug("Upgrading release")
	_, err = helmClientObj.UpgradeReleaseWithChartInfo(ctx, updateChartSpec)
	impl.logger.Debugw("response form UpgradeReleaseWithChartInfo", "err", err)
	if UpgradeErr, ok := err.(*driver.StorageDriverError); ok {
		if UpgradeErr != nil {
			if util.IsReleaseNotFoundError(UpgradeErr.Err) {
				_, err := helmClientObj.InstallChart(ctx, installChartSpec)
				if err != nil {
					impl.logger.Errorw("Error in install release ", "err", err)
					internalErr := error2.ConvertHelmErrorToInternalError(err)
					if internalErr != nil {
						err = internalErr
					}
					return false, err
				}

			} else {
				impl.logger.Errorw("Error in upgrade release with chart info", "err", err)
				return false, err

			}
		}
	} else if err != nil {
		impl.logger.Errorw("Error in upgrade release with chart info", "err", err)
		internalErr := error2.ConvertHelmErrorToInternalError(err)
		if internalErr != nil {
			err = internalErr
		}
		return false, err

	}
	return true, nil
}

func (impl *HelmAppServiceImpl) ValidateOCIRegistryLogin(ctx context.Context, OCIRegistryRequest *client.RegistryCredential) (*client.OCIRegistryResponse, error) {
	registryClient, err := registry.NewClient()
	if err != nil {
		impl.logger.Errorw(HELM_CLIENT_ERROR, "err", err)
		return nil, err
	}
	var caFilePath string
	if OCIRegistryRequest.Connection == registry2.SECURE_WITH_CERT {
		caFilePath, err = registry2.CreateCertificateFile(OCIRegistryRequest.RegistryName, OCIRegistryRequest.RegistryCertificate)
		if err != nil {
			impl.logger.Errorw("error in creating certificate file path", "registryName", OCIRegistryRequest.RegistryName, "err", err)
			return nil, err
		}
	}
	registryConfig, err := NewRegistryConfig(OCIRegistryRequest)
	defer func() {
		if registryConfig != nil {
			err := registry2.DeleteCertificateFolder(registryConfig.RegistryCAFilePath)
			if err != nil {
				impl.logger.Errorw("error in deleting certificate folder", "registryName", registryConfig.RegistryId, "err", err)
			}
		}
	}()
	if err != nil {
		impl.logger.Errorw("error in getting registry config from registry proto", "registryName", OCIRegistryRequest.RegistryName, "err", err)
		return nil, err
	}
	registryConfig.RegistryCAFilePath = caFilePath
	registryClient, err = registry2.GetLoggedInClient(registryClient, registryConfig)
	if err != nil {
		impl.logger.Errorw("error in registry login", "registryName", OCIRegistryRequest.RegistryName, "err", err)
		return nil, err
	}
	return &client.OCIRegistryResponse{
		IsLoggedIn: true,
	}, err
}

func (impl *HelmAppServiceImpl) PushHelmChartToOCIRegistryRepo(ctx context.Context, OCIRegistryRequest *client.OCIRegistryRequest) (*client.OCIRegistryResponse, error) {

	registryPushResponse := &client.OCIRegistryResponse{}

	registryConfig, err := NewRegistryConfig(OCIRegistryRequest.RegistryCredential)
	defer func() {
		if registryConfig != nil {
			err := registry2.DeleteCertificateFolder(registryConfig.RegistryCAFilePath)
			if err != nil {
				impl.logger.Errorw("error in deleting certificate folder", "registryName", registryConfig.RegistryId, "err", err)
			}
		}
	}()
	if err != nil {
		impl.logger.Errorw("error in getting registry config from registry proto", "registryName", OCIRegistryRequest.RegistryCredential.RegistryName, "err", err)
		return nil, err
	}

	settingsGetter, err := impl.registrySettings.GetSettings(registryConfig)
	if err != nil {
		impl.logger.Errorw("error in getting registry settings", "err", err)
		return nil, err
	}
	settings, err := settingsGetter.GetRegistrySettings(registryConfig)
	if err != nil {
		impl.logger.Errorw(HELM_CLIENT_ERROR, "registryName", OCIRegistryRequest.RegistryCredential.RegistryName, "err", err)
		return nil, err
	}
	registryClient := settings.RegistryClient
	OCIRegistryRequest.RegistryCredential.RegistryUrl = settings.RegistryHostURL

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
		trimmedURL := TrimSchemeFromURL(repoURL)
		if err != nil {
			impl.logger.Errorw("err in getting repo url without scheme", "repoURL", repoURL, "err", err)
			return nil, err
		}
		// add chart name and version from the chart metadata
		ref = fmt.Sprintf("%s:%s",
			path.Join(trimmedURL, meta.Metadata.Name),
			meta.Metadata.Version)
	} else {
		// disable strict mode for configuring chartName in repo
		withStrictMode = registry.PushOptStrictMode(false)
		trimmedURL := TrimSchemeFromURL(repoURL)
		if err != nil {
			impl.logger.Errorw("err in getting repo url without scheme", "repoURL", repoURL, "err", err)
			return nil, err
		}
		// add chartName and version to url
		ref = fmt.Sprintf("%s:%s",
			path.Join(trimmedURL, OCIRegistryRequest.ChartName),
			OCIRegistryRequest.ChartVersion)
	}

	pushResult, err := registryClient.Push(OCIRegistryRequest.Chart, ref, withStrictMode)
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

func TrimSchemeFromURL(registryUrl string) string {
	if !strings.Contains(strings.ToLower(registryUrl), "https") && !strings.Contains(strings.ToLower(registryUrl), "http") {
		registryUrl = fmt.Sprintf("//%s", registryUrl)
	}
	parsedUrl, err := url.Parse(registryUrl)
	if err != nil {
		return registryUrl
	}
	urlWithoutScheme := parsedUrl.Host + parsedUrl.Path
	urlWithoutScheme = strings.TrimPrefix(urlWithoutScheme, "/")
	return urlWithoutScheme
}

func (impl *HelmAppServiceImpl) GetNatsMessageForHelmInstallError(ctx context.Context, helmInstallMessage HelmReleaseStatusConfig, releaseIdentifier *client.ReleaseIdentifier, installationErr error) (string, error) {
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

func (impl *HelmAppServiceImpl) GetNatsMessageForHelmInstallSuccess(helmInstallMessage HelmReleaseStatusConfig) (string, error) {
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
