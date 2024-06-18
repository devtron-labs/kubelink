package FluxService

import (
	"context"
	"fmt"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/converter"
	client "github.com/devtron-labs/kubelink/grpc"
	clusterRepository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/service/CommonHelperService"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

type FluxApplicationService interface {
	GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList
	BuildFluxAppDetail(request *client.FluxAppDetailRequest) (*FluxKsAppDetail, error)
}

type FluxApplicationServiceImpl struct {
	logger            *zap.SugaredLogger
	clusterRepository clusterRepository.ClusterRepository
	k8sUtil           k8sUtils.K8sService
	converter         converter.ClusterBeanConverter
	common            CommonHelperService.CommonUtilsI
}

func NewFluxApplicationServiceImpl(logger *zap.SugaredLogger,
	clusterRepository clusterRepository.ClusterRepository,
	k8sUtil k8sUtils.K8sService,
	converter converter.ClusterBeanConverter, common CommonHelperService.CommonUtilsI) *FluxApplicationServiceImpl {
	return &FluxApplicationServiceImpl{
		logger:            logger,
		clusterRepository: clusterRepository,
		k8sUtil:           k8sUtil,
		converter:         converter,
		common:            common,
	}

}

func (impl *FluxApplicationServiceImpl) GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList {
	impl.logger.Debugw("Fetching application list ", "clusterId", config.ClusterId, "clusterName", config.ClusterName)

	appListFinal := make([]*FluxApplicationDto, 0)
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("Error in building rest config ", "clusterId", config.ClusterId, "err", err)
		return &client.FluxApplicationList{}
	}

	var restConfig2 rest.Config
	restConfig2 = *restConfig
	kustomizationResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, GvkForKustomizationFluxApp, AllNamespaces, true, nil)
	if err != nil {
		impl.logger.Errorw("Error in fetching kustomizationList Resource", "err", err)
		return &client.FluxApplicationList{}
	} else {
		if kustomizationResp != nil {
			kustomizationAppLists := GetApplicationListDtos(kustomizationResp.Resources, config.ClusterName, int(config.ClusterId), FluxAppKustomizationKind)
			if len(kustomizationAppLists) > 0 {
				appListFinal = append(appListFinal, kustomizationAppLists...)
			}
		}
	}

	helmReleaseResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), &restConfig2, GvkForHelmreleaseFluxApp, AllNamespaces, true, nil)
	if err != nil {
		impl.logger.Errorw("Error in fetching helmReleaseList Resources", "err", err)
		return &client.FluxApplicationList{}
	} else {
		if helmReleaseResp != nil {
			helmReleaseAppLists := GetApplicationListDtos(helmReleaseResp.Resources, config.ClusterName, int(config.ClusterId), HelmReleaseFluxAppType)
			if len(helmReleaseAppLists) > 0 {
				appListFinal = append(appListFinal, helmReleaseAppLists...)
			}
		}
	}

	appListFinalDto := make([]*client.FluxApplication, 0)

	for _, appDetail := range appListFinal {
		fluxAppDetailDto := GetFluxAppDetailDto(appDetail)
		appListFinalDto = append(appListFinalDto, fluxAppDetailDto)
	}
	finalAppListDto := &client.FluxApplicationList{
		ClusterId:       config.ClusterId,
		FluxApplication: appListFinalDto,
	}
	return finalAppListDto
}
func (impl *FluxApplicationServiceImpl) BuildFluxAppDetail(request *client.FluxAppDetailRequest) (*FluxKsAppDetail, error) {
	var fluxAppTreeResponse []*bean.ResourceTreeResponse
	var err error
	var appStatus *FluxAppStatusDetail
	var deploymentType FluxAppType
	req := &FluxAppDetailRequest{
		Name:        request.Name,
		Config:      request.ClusterConfig,
		Namespace:   request.Namespace,
		IsKustomize: request.IsKustomizeApp,
	}
	if request.IsKustomizeApp {
		deploymentType = FluxAppKustomizationKind
		fluxAppTreeResponse, appStatus, err = impl.buildFluxAppDetailForKustomize(req)
	} else {
		deploymentType = FluxAppHelmreleaseKind
		fluxAppTreeResponse, appStatus, err = impl.buildFluxAppDetailForHelmRelease(req)
	}

	if err != nil && appStatus != nil {

		if appStatus != nil {

			return &FluxKsAppDetail{
				FluxApplicationDto: &FluxApplicationDto{
					Name:         request.Name,
					HealthStatus: appStatus.Status,
					SyncStatus:   appStatus.Message,
					EnvironmentDetails: &EnvironmentDetail{
						ClusterId:   int(req.Config.ClusterId),
						ClusterName: req.Config.ClusterName,
						Namespace:   req.Namespace,
					},
					FluxAppDeploymentType: deploymentType,
				},
				AppStatusDto: appStatus,
			}, nil
		}
		return nil, err
	}

	fluxKsAppDetail := &FluxKsAppDetail{
		FluxApplicationDto: &FluxApplicationDto{
			Name:         request.Name,
			HealthStatus: appStatus.Status,
			SyncStatus:   appStatus.Message,
			EnvironmentDetails: &EnvironmentDetail{
				ClusterId:   int(req.Config.ClusterId),
				ClusterName: req.Config.ClusterName,
				Namespace:   req.Namespace,
			},
			FluxAppDeploymentType: deploymentType,
		},
		AppStatusDto: appStatus,
		TreeResponse: fluxAppTreeResponse,
	}

	return fluxKsAppDetail, nil

}
func (impl *FluxApplicationServiceImpl) buildFluxAppDetailForHelmRelease(request *FluxAppDetailRequest) ([]*bean.ResourceTreeResponse, *FluxAppStatusDetail, error) {
	fluxAppTreeResponse := make([]*bean.ResourceTreeResponse, 0)

	releaseName, namespace, appStatus, err := impl.getHelmReleaseInventory(request.Name, request.Namespace, request.Config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Helm release inventory: %v", err)
	}

	helmRelease, err := impl.common.GetHelmRelease(request.Config, namespace, releaseName)
	if err != nil {
		return nil, appStatus, fmt.Errorf("failed to get Helm release: %v", err)
	}

	appDetailRequest := &client.AppDetailRequest{
		ReleaseName:   releaseName,
		Namespace:     namespace,
		ClusterConfig: request.Config,
	}

	resourceTreeResponse, err := impl.common.BuildResourceTree(appDetailRequest, helmRelease)
	if err != nil {
		return nil, appStatus, fmt.Errorf("failed to build resource tree for Helm release: %v", err)
	}
	fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)

	return fluxAppTreeResponse, appStatus, nil
}
func (impl *FluxApplicationServiceImpl) buildFluxAppDetailForKustomize(request *FluxAppDetailRequest) ([]*bean.ResourceTreeResponse, *FluxAppStatusDetail, error) {
	fluxAppTreeResponse := make([]*bean.ResourceTreeResponse, 0)

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(request.Config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Kubernetes rest config: %v", err)
	}

	resp, err := impl.k8sUtil.GetResource(context.Background(), request.Namespace, request.Name, GvkForKustomizationFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting resource list", "err", err)
		return nil, nil, fmt.Errorf("failed to get Kubernetes resource: %v", err)
	}
	if resp == nil || resp.Manifest.Object == nil {
		return nil, nil, fmt.Errorf("response or manifest object is nil")
	}
	appStatus, err := getKsAppStatus(resp.Manifest.Object)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Kustomize app status: %v", err)
	}
	fluxK8sResourceList := make([]*client.ExternalResourceDetail, 0)
	fluxHrList := make([]*FluxHr, 0)

	err = impl.getFluxResourceAndFluxHrList(request, &fluxK8sResourceList, &fluxHrList, resp.Manifest.Object)
	if err != nil {
		return nil, appStatus, fmt.Errorf("failed to get Flux resources and Helm releases: %v", err)
	}

	for _, fluxHr := range fluxHrList {
		releaseName, namespace, _, err := impl.getHelmReleaseInventory(fluxHr.Name, fluxHr.Namespace, request.Config)
		if err != nil {
			return nil, appStatus, fmt.Errorf("failed to get Helm release inventory: %v", err)
		}

		helmRelease, err := impl.common.GetHelmRelease(request.Config, namespace, releaseName)
		if err != nil {
			return nil, appStatus, fmt.Errorf("failed to get Helm release: %v", err)
		}

		appDetailRequest := &client.AppDetailRequest{
			ReleaseName:   releaseName,
			Namespace:     namespace,
			ClusterConfig: request.Config,
		}

		resourceTreeResponse, err := impl.common.BuildResourceTree(appDetailRequest, helmRelease)
		if err != nil {
			return nil, appStatus, fmt.Errorf("failed to build resource tree for Helm release: %v", err)
		}
		fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)
	}

	if len(fluxK8sResourceList) > 0 {
		req := &client.ExternalResourceTreeRequest{
			ClusterConfig:          request.Config,
			ExternalResourceDetail: fluxK8sResourceList,
		}

		resourceTreeResponse, err := impl.common.GetResourceTreeForExternalResources(req)
		if err != nil {
			return nil, appStatus, fmt.Errorf("failed to get resource tree for external resources: %v", err)
		}

		fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)
	}

	return fluxAppTreeResponse, appStatus, nil
}
func (impl *FluxApplicationServiceImpl) getFluxResourceAndFluxHrList(app *FluxAppDetailRequest, fluxK8sResourceList *[]*client.ExternalResourceDetail, fluxHrList *[]*FluxHr, obj map[string]interface{}) error {

	inventoryMap, err := getInventoryMap(obj)
	if err != nil {
		return err
	}
	fluxKsSpecKubeConfig := getFluxSpecKubeConfig(obj)

	for id, version := range inventoryMap {
		var fluxResource FluxKsResourceDetail
		fluxResource, err = parseObjMetadata(id)
		if err != nil {
			fmt.Println("issue is here for some reason , r", err)
			return err
		}
		fluxResource.Version = version
		if fluxResource.Group == FluxKustomizationGroup && fluxResource.Kind == FluxAppKustomizationKind && fluxResource.Name == app.Name && fluxResource.Namespace == app.Namespace {
			continue
		}

		if fluxResource.Kind != FluxAppHelmreleaseKind && fluxResource.Kind != FluxAppKustomizationKind {
			//fluxK8sResources = append(fluxK8sResources, &fluxResource)
			*fluxK8sResourceList = append(*fluxK8sResourceList, &client.ExternalResourceDetail{
				Namespace: fluxResource.Namespace,
				Name:      fluxResource.Name,
				Group:     fluxResource.Group,
				Version:   fluxResource.Version,
				Kind:      fluxResource.Kind,
			})
		}

		if fluxResource.Group == FluxHelmReleaseGroup &&
			fluxResource.Kind == FluxAppHelmreleaseKind {
			var releaseName, nameSpace string
			releaseName, nameSpace, _, err = impl.getHelmReleaseInventory(fluxResource.Name, fluxResource.Namespace, app.Config)
			if err != nil {
				fmt.Println(err)
				return err
			}
			*fluxK8sResourceList = append(*fluxK8sResourceList, &client.ExternalResourceDetail{
				Namespace: fluxResource.Namespace,
				Name:      fluxResource.Name,
				Group:     fluxResource.Group,
				Kind:      fluxResource.Kind,
				Version:   fluxResource.Version,
			})

			*fluxHrList = append(*fluxHrList, &FluxHr{Name: releaseName, Namespace: nameSpace})
		}

		if fluxResource.Group == FluxKustomizationGroup &&
			fluxResource.Kind == FluxAppKustomizationKind &&
			// skip kustomization if it targets a remote clusters
			fluxKsSpecKubeConfig == false {
			*fluxK8sResourceList = append(*fluxK8sResourceList, &client.ExternalResourceDetail{
				Namespace: fluxResource.Namespace,
				Name:      fluxResource.Name,
				Group:     fluxResource.Group,
				Kind:      fluxResource.Kind,
				Version:   fluxResource.Version,
			})

			fluxKsAppChild := &FluxAppDetailRequest{
				Name:        fluxResource.Name,
				Namespace:   fluxResource.Namespace,
				IsKustomize: true,
				Config:      app.Config,
			}

			k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(app.Config)
			restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
			if err != nil {
				return err
			}
			resp, err := impl.k8sUtil.GetResource(context.Background(), fluxResource.Namespace, fluxResource.Name, GvkForKustomizationFluxApp, restConfig)
			if err != nil {
				impl.logger.Errorw("error in getting resource list", "err", err)
				return err
			}
			if resp == nil && resp.Manifest.Object == nil {
				return err
			}
			err = impl.getFluxResourceAndFluxHrList(fluxKsAppChild, fluxK8sResourceList, fluxHrList, obj)
			if err != nil {
				fmt.Println(err)
				return err
			}
		}
	}
	return nil
}
func (impl *FluxApplicationServiceImpl) getHelmReleaseInventory(name string, namespace string, config *client.ClusterConfig) (string, string, *FluxAppStatusDetail, error) {
	k8sConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sConfig)
	resp, err := impl.k8sUtil.GetResource(context.Background(), namespace, name, GvkForHelmreleaseFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting resource list", "err", err)
		return "", "", nil, err
	}
	var releaseName, _namespace string
	var appStatus *FluxAppStatusDetail
	if resp != nil && resp.Manifest.Object != nil {
		releaseName, _namespace = getReleaseNameNamespace(resp.Manifest.Object)

		appStatus, err = getKsAppStatus(resp.Manifest.Object)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to get Kustomize app status: %v", err)
		}
	}

	return releaseName, _namespace, appStatus, nil

}
