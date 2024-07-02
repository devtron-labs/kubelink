package fluxService

import (
	"context"
	"fmt"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/converter"
	client "github.com/devtron-labs/kubelink/grpc"
	clusterRepository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/service/commonHelmService"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	common            commonHelmService.CommonHelmService
}

func NewFluxApplicationServiceImpl(logger *zap.SugaredLogger,
	clusterRepository clusterRepository.ClusterRepository,
	k8sUtil k8sUtils.K8sService,
	converter converter.ClusterBeanConverter, common commonHelmService.CommonHelmService) *FluxApplicationServiceImpl {
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
	deployedApp := &client.FluxApplicationList{ClusterId: config.GetClusterId()}
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("Error in building rest config ", "clusterId", config.ClusterId, "err", err)
		deployedApp.Errored = true
		deployedApp.ErrorMsg = err.Error()
		return deployedApp
	}
	restConf := *restConfig
	kustomizationList, helmReleaseList, err := impl.fetchFluxApplicationResources(restConf)
	if err != nil && kustomizationList == nil {
		impl.logger.Errorw("Error in getting the list of flux app list ", "clusterId", config.ClusterId, "err", err)
		deployedApp.Errored = true
		deployedApp.ErrorMsg = err.Error()
		return deployedApp
	}
	appDetailList := impl.convertFluxAppResources(kustomizationList, helmReleaseList, config)
	deployedApp.FluxApplication = convertFluxAppDetailsToDtos(appDetailList)
	return deployedApp
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
	fluxKsAppDetail := &FluxKsAppDetail{}
	if appStatus != nil {
		fluxKsAppDetail.AppStatusDto = appStatus
		fluxKsAppDetail.FluxApplicationDto = &FluxApplicationDto{
			Name:         request.Name,
			HealthStatus: appStatus.Status,
			SyncStatus:   appStatus.Message,
			EnvironmentDetails: &EnvironmentDetail{
				ClusterId:   int(req.Config.ClusterId),
				ClusterName: req.Config.ClusterName,
				Namespace:   req.Namespace,
			},
			FluxAppDeploymentType: deploymentType,
		}
	}
	if err != nil {
		if appStatus != nil {
			return fluxKsAppDetail, nil
		}
		return nil, err
	}
	fluxKsAppDetail.TreeResponse = fluxAppTreeResponse
	return fluxKsAppDetail, nil
}
func (impl *FluxApplicationServiceImpl) fetchFluxApplicationResources(restConfig rest.Config) (*k8sUtils.ResourceListResponse, *k8sUtils.ResourceListResponse, error) {
	kustomizationList, err := impl.getFluxAppList(restConfig, GvkForKustomizationFluxApp)
	if err != nil {
		return nil, nil, err
	}
	helmReleaseList, err := impl.getFluxAppList(restConfig, GvkForHelmreleaseFluxApp)
	if err != nil {
		return kustomizationList, nil, err
	}
	return kustomizationList, helmReleaseList, nil
}
func (impl *FluxApplicationServiceImpl) convertFluxAppResources(kustomizationList, helmReleaseList *k8sUtils.ResourceListResponse, config *client.ClusterConfig) []*FluxApplicationDto {
	var appDetailList []*FluxApplicationDto
	if kustomizationList.Resources.Object != nil {
		kustomizationAppLists := GetApplicationListDtos(kustomizationList.Resources, config.ClusterName, int(config.ClusterId), FluxAppKustomizationKind)
		if len(kustomizationAppLists) > 0 {
			parentKustomizationList := impl.childFilterMapping(kustomizationAppLists)
			appDetailList = append(appDetailList, parentKustomizationList...)
		}
	}
	if helmReleaseList.Resources.Object != nil {
		helmReleaseAppLists := GetApplicationListDtos(helmReleaseList.Resources, config.ClusterName, int(config.ClusterId), HelmReleaseFluxAppType)
		if len(helmReleaseAppLists) > 0 {
			//parentKustomizationList := impl.childFilterMapping(helmReleaseAppLists)
			appDetailList = append(appDetailList, helmReleaseAppLists...)
		}
		//appDetailList = append(appDetailList, helmReleaseAppLists...)
	}
	return appDetailList
}
func (impl *FluxApplicationServiceImpl) getFluxAppList(restConfig rest.Config, gvk schema.GroupVersionKind) (*k8sUtils.ResourceListResponse, error) {
	resp, _, err := impl.k8sUtil.GetResourceList(context.Background(), &restConfig, gvk, AllNamespaces, true, nil)
	if err != nil {
		impl.logger.Errorw("Error in fetching Resource list", "err", err)
		return nil, err
	}
	return resp, nil
}
func (impl *FluxApplicationServiceImpl) childFilterMapping(appList []*FluxApplicationDto) []*FluxApplicationDto {
	childParentMap := make(map[string]bool)
	kustomizationMap := make(map[string]*FluxApplicationDto, len(appList)-1)
	childFilteredAppList := make([]*FluxApplicationDto, 0)
	for _, app := range appList {
		if app.Name == "flux-system" && app.EnvironmentDetails.Namespace == "flux-system" {
			continue
		}
		kustomizationMap[app.Name] = app

		if app.HealthStatus != "False" {
			cluster, err := impl.clusterRepository.FindById(app.EnvironmentDetails.ClusterId)
			if err != nil {
				impl.logger.Errorw("error in getting cluster repository", "err", err)
			}
			clusterInfo := impl.converter.GetClusterInfo(cluster)
			clusterConfig := impl.converter.GetClusterConfig(clusterInfo)
			restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
			if err != nil {
				impl.logger.Errorw("error in getting cluster config", "err", err)
			}
			resp, err := impl.k8sUtil.GetResource(context.Background(), app.EnvironmentDetails.Namespace, app.Name, GvkForKustomizationFluxApp, restConfig)
			if err != nil || resp == nil {
				impl.logger.Errorw("error in getting response", "err", err)
			}

			inventoryMap, err := getInventoryObjMetadataFromResponseObj(resp.Manifest.Object)
			if err != nil {
				impl.logger.Errorw("error in inventories", "err", err)
			}

			for id, version := range inventoryMap {
				var fluxResource *FluxKsResourceDetail
				fluxResource, err = decodeObjMetadata(id, version)
				if err != nil {
					fmt.Println("issue is here for some reason , r", err)
				}
				if fluxResource.Group == FluxKustomizationGroup && fluxResource.Kind == FluxAppKustomizationKind {
					if fluxResource.Name == app.Name && fluxResource.Namespace == app.EnvironmentDetails.Namespace {
						continue
					}
					childParentMap[fluxResource.Name] = true
				}
			}
		}
	}

	for _, app := range appList {
		if app.Name == "flux-system" && app.EnvironmentDetails.Namespace == "flux-system" {
			continue
		} else if childParentMap[app.Name] != true {
			childFilteredAppList = append(childFilteredAppList, kustomizationMap[app.Name])
		}
	}
	return childFilteredAppList
}
func (impl *FluxApplicationServiceImpl) buildFluxAppDetailForHelmRelease(request *FluxAppDetailRequest) ([]*bean.ResourceTreeResponse, *FluxAppStatusDetail, error) {
	fluxAppTreeResponse := make([]*bean.ResourceTreeResponse, 0)
	k8sConfig := impl.converter.GetClusterConfigFromClientBean(request.Config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sConfig)
	resp, err := impl.k8sUtil.GetResource(context.Background(), request.Namespace, request.Name, GvkForHelmreleaseFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting resource", "err", err)
		return nil, nil, err
	}

	releaseName := ""
	_namespace := ""
	var appStatus *FluxAppStatusDetail

	if resp != nil && resp.Manifest.Object != nil {
		releaseName, _namespace, err = getReleaseNameNamespace(resp.Manifest.Object, request.Name)
		if err != nil {
			return nil, nil, err
		}

		appStatus, err = getKsAppStatus(resp.Manifest.Object, GvkForHelmreleaseFluxApp)
		if err != nil {
			impl.logger.Errorw("error in getting app status", "err", err)
			return nil, nil, err
		}

		if releaseName == "" && _namespace == "" {
			impl.logger.Errorw("error in getting release name and namespace", "err", err)
			return nil, appStatus, err
		}
	}
	helmRelease, err := impl.common.GetHelmRelease(request.Config, _namespace, releaseName)
	if err != nil {
		impl.logger.Errorw("error in getting helm release", "err", err)
		return nil, appStatus, err
	}
	appDetailRequest := &client.AppDetailRequest{
		ReleaseName:   releaseName,
		Namespace:     _namespace,
		ClusterConfig: request.Config,
	}

	resourceTreeResponse, err := impl.common.BuildResourceTree(appDetailRequest, helmRelease)
	if err != nil {
		impl.logger.Errorw("error in building resource tree", "err", err)
		return nil, appStatus, err
	}
	fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)

	return fluxAppTreeResponse, appStatus, nil
}
func (impl *FluxApplicationServiceImpl) buildFluxAppDetailForKustomize(request *FluxAppDetailRequest) ([]*bean.ResourceTreeResponse, *FluxAppStatusDetail, error) {
	fluxAppTreeResponse := make([]*bean.ResourceTreeResponse, 0)

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(request.Config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting the rest config", "error", err)
		return nil, nil, err
	}
	resp, err := impl.k8sUtil.GetResource(context.Background(), request.Namespace, request.Name, GvkForKustomizationFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting app manifest", "err", err)
		return nil, nil, err
	}
	if resp == nil || resp.Manifest.Object == nil {
		impl.logger.Errorw("app manifest's response or manifest object is nil", "err", err)
		return nil, nil, err
	}
	appStatus, err := getKsAppStatus(resp.Manifest.Object, GvkForKustomizationFluxApp)
	if err != nil {
		impl.logger.Errorw("failed to get app status", "err", err)
		return nil, nil, err
	}
	inventory, exists := inventoryExists(resp.Manifest.Object)
	if !exists {
		impl.logger.Errorw("Inventory is empty ", err)
		return nil, appStatus, err
	}
	//
	//fluxK8sResourceList := make([]*client.ExternalResourceDetail, 0)
	//fluxHrList := make([]*FluxHr, 0)
	_, isKubeConfigExist, err := unstructured.NestedFieldCopy(resp.Manifest.Object, "spec", "kubeconfig")
	if err != nil {
		return nil, appStatus, err

	}
	fluxK8sResourceList, fluxHrList, err := impl.getFluxResourceAndFluxHrList(request, inventory, isKubeConfigExist)
	if len(fluxHrList) > 0 {
		hrTreeResponse, _ := impl.getTreeForHrList(request, fluxHrList)
		if hrTreeResponse != nil {
			fluxAppTreeResponse = append(fluxAppTreeResponse, hrTreeResponse...)
		}
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
		if len(resourceTreeResponse.Nodes) > 0 {
			fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)
		}
	}
	return fluxAppTreeResponse, appStatus, err
}
func (impl *FluxApplicationServiceImpl) getTreeForHrList(request *FluxAppDetailRequest, fluxHrList []*FluxHr) ([]*bean.ResourceTreeResponse, error) {
	fluxAppTreeResponse := make([]*bean.ResourceTreeResponse, 0)
	var err error
	for _, fluxHr := range fluxHrList {
		releaseName := fluxHr.Name
		namespace := fluxHr.Namespace
		helmRelease, err := impl.common.GetHelmRelease(request.Config, namespace, releaseName)
		if err != nil {
			impl.logger.Errorw("error in getting helm release", "err", err)
		}
		appDetailRequest := &client.AppDetailRequest{
			ReleaseName:   releaseName,
			Namespace:     namespace,
			ClusterConfig: request.Config,
		}
		if helmRelease != nil {
			resourceTreeResponse, err := impl.common.BuildResourceTree(appDetailRequest, helmRelease)
			if err != nil {
				//return nil, appStatus, fmt.Errorf("failed to build resource tree for Helm release: %v", err)
				impl.logger.Errorw("error in building resource tree for HelmResource", "err", err)
			}
			fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)
		}

	}
	return fluxAppTreeResponse, err
}

func (impl *FluxApplicationServiceImpl) getFluxResourceAndFluxHrList(app *FluxAppDetailRequest, inventory map[string]interface{}, isKubeConfigExist bool) ([]*client.ExternalResourceDetail, []*FluxHr, error) {
	var fluxK8sResourceList []*client.ExternalResourceDetail
	var fluxHrList []*FluxHr

	childResourcesList, err := fetchInventoryList(inventory)
	if err != nil {
		impl.logger.Errorw("error in getting inventory list", "err", err)
		return nil, nil, err
	}
	//isKubeConfig, err := getFluxSpecKubeConfig(obj)
	//if err != nil {
	//	impl.logger.Errorw("error in getting flux spec kubeConfig", "err", err)
	//	return nil, nil, err
	//}
	for _, childResource := range childResourcesList {
		fluxK8sResourceList = append(fluxK8sResourceList, &client.ExternalResourceDetail{
			Namespace: childResource.Namespace,
			Name:      childResource.Name,
			Group:     childResource.Group,
			Kind:      childResource.Kind,
			Version:   childResource.Version,
		})
		if childResource.Group == FluxKustomizationGroup && childResource.Kind == FluxAppKustomizationKind && childResource.Name == app.Name && childResource.Namespace == app.Namespace {
			continue
		}
		if childResource.Group == FluxHelmReleaseGroup && childResource.Kind == FluxAppHelmreleaseKind {
			var releaseName, nameSpace string
			releaseName, nameSpace, _, err = impl.getHelmReleaseNameNamespaceandStatusDetail(childResource.Name, childResource.Namespace, app.Config)
			if err != nil {
				impl.logger.Errorw("error in getting helm release", "err", err)
				return nil, nil, err
			}
			if releaseName != "" && nameSpace != "" {
				fluxHrList = append(fluxHrList, &FluxHr{Name: releaseName, Namespace: nameSpace})
			}
		}
		if childResource.Group == FluxKustomizationGroup && childResource.Kind == FluxAppKustomizationKind && isKubeConfigExist == false {
			fluxKsAppChild := &FluxAppDetailRequest{
				Name:        childResource.Name,
				Namespace:   childResource.Namespace,
				IsKustomize: true,
				Config:      app.Config,
			}

			k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(app.Config)
			restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
			if err != nil {
				impl.logger.Errorw("error in getting k8s cluster config", "err", err)
				return nil, nil, err
			}
			resp, err := impl.k8sUtil.GetResource(context.Background(), childResource.Namespace, childResource.Name, GvkForKustomizationFluxApp, restConfig)
			if err != nil {
				impl.logger.Errorw("error in getting resource list", "err", err)
				return nil, nil, err
			}
			if resp == nil || resp.Manifest.Object == nil {
				return nil, nil, nil
			}
			_, isKubeConfigExistOfChild, err := unstructured.NestedFieldCopy(resp.Manifest.Object, "spec", "kubeconfig")
			if err != nil {
				return nil, nil, err
			}
			if childInventory, exists := inventoryExists(resp.Manifest.Object); exists {
				childFluxK8sResourceList, childFluxHrList, err := impl.getFluxResourceAndFluxHrList(fluxKsAppChild, childInventory, isKubeConfigExistOfChild)
				if err != nil {
					return nil, nil, err
				}
				fluxK8sResourceList = append(fluxK8sResourceList, childFluxK8sResourceList...)
				fluxHrList = append(fluxHrList, childFluxHrList...)
			}
		}
	}
	return fluxK8sResourceList, fluxHrList, nil
}

//func (impl *FluxApplicationServiceImpl) getFluxResourceAndFluxHrList(app *FluxAppDetailRequest, fluxK8sResourceList []*client.ExternalResourceDetail, fluxHrList []*FluxHr, obj map[string]interface{}) {
//	childResourcesList, err := fetchInventoryList(obj)
//	if err != nil {
//		impl.logger.Errorw("error in getting inventory list", "err", err)
//		return
//	}
//	isKubeConfig, err := getFluxSpecKubeConfig(obj)
//	if err != nil {
//		impl.logger.Errorw("error in getting flux spec kubeConfig", "err", err)
//		return
//	}
//	for _, childResource := range childResourcesList {
//		fluxK8sResourceList = append(fluxK8sResourceList, &client.ExternalResourceDetail{
//			Namespace: childResource.Namespace,
//			Name:      childResource.Name,
//			Group:     childResource.Group,
//			Kind:      childResource.Kind,
//			Version:   childResource.Version,
//		})
//		if childResource.Group == FluxKustomizationGroup && childResource.Kind == FluxAppKustomizationKind && childResource.Name == app.Name && childResource.Namespace == app.Namespace {
//			continue
//		}
//		if childResource.Group == FluxHelmReleaseGroup &&
//			childResource.Kind == FluxAppHelmreleaseKind {
//			var releaseName, nameSpace string
//			releaseName, nameSpace, _, err = impl.getHelmReleaseNameNamespaceandStatusDetail(childResource.Name, childResource.Namespace, app.Config)
//			if err != nil {
//				impl.logger.Errorw("error in getting helm release", "err", err)
//				return
//			}
//			if releaseName != "" && nameSpace != "" {
//				fluxHrList = append(fluxHrList, &FluxHr{Name: releaseName, Namespace: nameSpace})
//			}
//		}
//
//		if childResource.Group == FluxKustomizationGroup &&
//			childResource.Kind == FluxAppKustomizationKind &&
//			// skip kustomization if it targets a remote clusters
//			isKubeConfig == false {
//			fluxKsAppChild := &FluxAppDetailRequest{
//				Name:        childResource.Name,
//				Namespace:   childResource.Namespace,
//				IsKustomize: true,
//				Config:      app.Config,
//			}
//
//			k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(app.Config)
//			restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
//			if err != nil {
//				impl.logger.Errorw("error in getting k8s cluster config", "err", err)
//				return
//			}
//			resp, err := impl.k8sUtil.GetResource(context.Background(), childResource.Namespace, childResource.Name, GvkForKustomizationFluxApp, restConfig)
//			if err != nil {
//				impl.logger.Errorw("error in getting resource list", "err", err)
//				return
//			}
//			if resp == nil && resp.Manifest.Object == nil {
//				return
//			}
//			if inventoryExists(resp.Manifest.Object) {
//				impl.getFluxResourceAndFluxHrList(fluxKsAppChild, fluxK8sResourceList, fluxHrList, resp.Manifest.Object)
//
//			}
//
//		}
//	}
//	return
//}

func (impl *FluxApplicationServiceImpl) getHelmReleaseNameNamespaceandStatusDetail(name string, namespace string, config *client.ClusterConfig) (string, string, *FluxAppStatusDetail, error) {
	k8sConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sConfig)
	resp, err := impl.k8sUtil.GetResource(context.Background(), namespace, name, GvkForHelmreleaseFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting resource", "err", err)
		return "", "", nil, err
	}
	var releaseName, _namespace string
	var appStatus *FluxAppStatusDetail
	if resp != nil && resp.Manifest.Object != nil {
		releaseName, _namespace, err = getReleaseNameNamespace(resp.Manifest.Object, name)

		appStatus, err = getKsAppStatus(resp.Manifest.Object, GvkForHelmreleaseFluxApp)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to get Kustomize app status: %v", err)
		}
	}

	return releaseName, _namespace, appStatus, nil
}
