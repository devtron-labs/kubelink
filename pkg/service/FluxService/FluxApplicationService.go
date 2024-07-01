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
	kustomizationApps, helmReleaseApps, err := impl.fetchFluxApplicationResources(restConf)
	if err != nil {
		impl.logger.Errorw("Error in getting the list of flux app list ", "clusterId", config.ClusterId, "err", err)
		deployedApp.Errored = true
		deployedApp.ErrorMsg = err.Error()
		return deployedApp
	}
	appDetailList := impl.convertFluxAppResources(kustomizationApps, helmReleaseApps, config)
	deployedApp.FluxApplication = convertFluxAppDetailsToDtos(appDetailList)
	return deployedApp
	//appListFinal := make([]*FluxApplicationDto, 0)
	//var appDetailList []*FluxApplicationDto
	//
	//// Process Kustomization data (even if empty or with error)
	//if kustomizationApps != nil {
	//	kustomizationAppLists := GetApplicationListDtos(kustomizationApps.Items, config.ClusterName, int(config.ClusterId), FluxAppKustomizationKind)
	//	appDetailList = append(appDetailList, kustomizationAppLists...)
	//} else if kustomizationErr != nil {
	//	// Log the Kustomization error but continue processing
	//	impl.logger.Errorw("Error in fetching Kustomization resources", "err", kustomizationErr)
	//}
	//
	//// Process HelmRelease data if successful
	//if helmReleaseApps != nil {
	//	helmReleaseAppLists := GetApplicationListDtos(helmReleaseApps.Items, config.ClusterName, int(config.ClusterId), HelmReleaseFluxAppType)
	//	appDetailList = append(appDetailList, helmReleaseAppLists...)
	//}
	//
	//deployedApp := &client.FluxApplicationList{ClusterId: config.GetClusterId()}
	//deployedApp.FluxApplication = convertFluxAppDetailsToDtos(appDetailList)
	//return deployedApp, nil
	//
	//var restConfig2 rest.Config
	//restConfig2 = *restConfig
	//kustomizationResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, GvkForKustomizationFluxApp, AllNamespaces, true, nil)
	//if err != nil {
	//	impl.logger.Errorw("Error in fetching kustomizationList Resource", "err", err)
	//	return &client.FluxApplicationList{}
	//} else {
	//	if kustomizationResp != nil {
	//		kustomizationAppLists := GetApplicationListDtos(kustomizationResp.Resources, config.ClusterName, int(config.ClusterId), FluxAppKustomizationKind)
	//		if len(kustomizationAppLists) > 0 {
	//			parentKustomizationList := impl.childFilterMapping(kustomizationAppLists)
	//			if len(parentKustomizationList) > 0 {
	//				appListFinal = append(appListFinal, parentKustomizationList...)
	//			}
	//
	//		}
	//	}
	//}
	//helmReleaseResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), &restConfig2, GvkForHelmreleaseFluxApp, AllNamespaces, true, nil)
	//if err != nil {
	//	impl.logger.Errorw("Error in fetching helmReleaseList Resources", "err", err)
	//	return &client.FluxApplicationList{}
	//} else {
	//	if helmReleaseResp.Resources.Object != nil {
	//		helmReleaseAppLists := GetApplicationListDtos(helmReleaseResp.Resources, config.ClusterName, int(config.ClusterId), HelmReleaseFluxAppType)
	//		if len(helmReleaseAppLists) > 0 {
	//			appListFinal = append(appListFinal, helmReleaseAppLists...)
	//		}
	//	}
	//}
	////if helmReleaseResp == nil && kustomizationResp == nil {
	////	return &client.FluxApplicationList{}
	////}
	//
	//appListFinalDto := make([]*client.FluxApplication, 0)
	//
	//for _, appDetail := range appListFinal {
	//	fluxAppDetailDto := GetFluxAppDetailDto(appDetail)
	//	appListFinalDto = append(appListFinalDto, fluxAppDetailDto)
	//}
	//finalAppListDto := &client.FluxApplicationList{
	//	ClusterId:       config.ClusterId,
	//	FluxApplication: appListFinalDto,
	//}
	//return finalAppListDto
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
		appDetailList = append(appDetailList, helmReleaseAppLists...)
	}
	return appDetailList
}
func convertFluxAppDetailsToDtos(appDetails []*FluxApplicationDto) []*client.FluxApplication {
	var appListFinalDto []*client.FluxApplication
	for _, appDetail := range appDetails {
		fluxAppDetailDto := GetFluxAppDetailDto(appDetail)
		appListFinalDto = append(appListFinalDto, fluxAppDetailDto)
	}
	return appListFinalDto
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

			inventoryMap, err := getInventoryMap(resp.Manifest.Object)
			if err != nil {
				impl.logger.Errorw("error in inventories", "err", err)
			}

			for id, version := range inventoryMap {
				var fluxResource FluxKsResourceDetail
				fluxResource, err = parseObjMetadata(id)
				if err != nil {
					fmt.Println("issue is here for some reason , r", err)
				}
				fluxResource.Version = version
				if fluxResource.Group == FluxKustomizationGroup && fluxResource.Kind == FluxAppKustomizationKind {
					if fluxResource.Name == app.Name && fluxResource.Namespace == app.EnvironmentDetails.Namespace {
						continue
					}
					childParentMap[fluxResource.Name] = true
				}
			}
		}
	}

	//for _, app := range appList {
	//
	//	if app.HealthStatus == "False" && childParentMap[app.Name] != false {
	//		childParentMap[app.Name] = true
	//	}
	//}

	for _, app := range appList {
		if app.Name == "flux-system" && app.EnvironmentDetails.Namespace == "flux-system" {
			continue
		} else if childParentMap[app.Name] != true {
			childFilteredAppList = append(childFilteredAppList, kustomizationMap[app.Name])
		}
	}
	return childFilteredAppList
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

	if err != nil {

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
		releaseName, _namespace = getReleaseNameNamespace(resp.Manifest.Object, request.Name)

		appStatus, err = getKsAppStatus(resp.Manifest.Object)
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

	//releaseName, namespace, appStatus, err := impl.getHelmReleaseNameNamespaceandStatusDetail(request.Name, request.Namespace, request.Config)
	//if err != nil {
	//	return nil, nil, fmt.Errorf("failed to get Helm release inventory: %v", err)
	//}
	//
	//helmRelease, err := impl.common.GetHelmRelease(request.Config, namespace, releaseName)
	//if err != nil {
	//	return nil, appStatus, fmt.Errorf("failed to get Helm release: %v", err)
	//}

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
	appStatus, err := getKsAppStatus(resp.Manifest.Object)
	if err != nil {
		impl.logger.Errorw("failed to get app status", "err", err)
		return nil, nil, err
	}
	if !inventoryExists(resp.Manifest.Object) {
		impl.logger.Errorw("Inventory is empty ", err)
		return nil, appStatus, err
	}

	fluxK8sResourceList := make([]*client.ExternalResourceDetail, 0)
	fluxHrList := make([]*FluxHr, 0)
	impl.getFluxResourceAndFluxHrList(request, &fluxK8sResourceList, &fluxHrList, resp.Manifest.Object)
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
		resourceTreeResponse, _ := impl.common.GetResourceTreeForExternalResources(req)
		//if err != nil {
		//	return nil, appStatus, fmt.Errorf("failed to get resource tree for external resources: %v", err)
		//}
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
func (impl *FluxApplicationServiceImpl) getFluxResourceAndFluxHrList(app *FluxAppDetailRequest, fluxK8sResourceList *[]*client.ExternalResourceDetail, fluxHrList *[]*FluxHr, obj map[string]interface{}) {
	inventoryMap, err := getInventoryMap(obj)
	if err != nil {
		impl.logger.Errorw("error in getting inventory map", "err", err)
		return
	}
	fluxKsSpecKubeConfig := getFluxSpecKubeConfig(obj)
	for id, version := range inventoryMap {
		var fluxResource FluxKsResourceDetail
		fluxResource, err = parseObjMetadata(id)
		if err != nil {
			impl.logger.Errorw("error in parsing the flux resource", "err", err)
			return
		}
		fluxResource.Version = version
		*fluxK8sResourceList = append(*fluxK8sResourceList, &client.ExternalResourceDetail{
			Namespace: fluxResource.Namespace,
			Name:      fluxResource.Name,
			Group:     fluxResource.Group,
			Kind:      fluxResource.Kind,
			Version:   fluxResource.Version,
		})
		if fluxResource.Group == FluxKustomizationGroup && fluxResource.Kind == FluxAppKustomizationKind && fluxResource.Name == app.Name && fluxResource.Namespace == app.Namespace {
			continue
		}

		if fluxResource.Group == FluxHelmReleaseGroup &&
			fluxResource.Kind == FluxAppHelmreleaseKind {
			var releaseName, nameSpace string
			releaseName, nameSpace, _, err = impl.getHelmReleaseNameNamespaceandStatusDetail(fluxResource.Name, fluxResource.Namespace, app.Config)
			if err != nil {
				impl.logger.Errorw("error in getting helm release", "err", err)
				return
			}
			if releaseName != "" && nameSpace != "" {
				*fluxHrList = append(*fluxHrList, &FluxHr{Name: releaseName, Namespace: nameSpace})
			}
		}

		if fluxResource.Group == FluxKustomizationGroup &&
			fluxResource.Kind == FluxAppKustomizationKind &&
			// skip kustomization if it targets a remote clusters
			fluxKsSpecKubeConfig == false {
			fluxKsAppChild := &FluxAppDetailRequest{
				Name:        fluxResource.Name,
				Namespace:   fluxResource.Namespace,
				IsKustomize: true,
				Config:      app.Config,
			}

			k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(app.Config)
			restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
			if err != nil {
				impl.logger.Errorw("error in getting k8s cluster config", "err", err)
				return
			}
			resp, err := impl.k8sUtil.GetResource(context.Background(), fluxResource.Namespace, fluxResource.Name, GvkForKustomizationFluxApp, restConfig)
			if err != nil {
				impl.logger.Errorw("error in getting resource list", "err", err)
				return
			}
			if resp == nil && resp.Manifest.Object == nil {
				return
			}
			if inventoryExists(resp.Manifest.Object) {
				impl.getFluxResourceAndFluxHrList(fluxKsAppChild, fluxK8sResourceList, fluxHrList, resp.Manifest.Object)

			}

		}
	}
	return
}
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
		releaseName, _namespace = getReleaseNameNamespace(resp.Manifest.Object, name)

		appStatus, err = getKsAppStatus(resp.Manifest.Object)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to get Kustomize app status: %v", err)
		}
	}

	return releaseName, _namespace, appStatus, nil
}
