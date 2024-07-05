package fluxService

import (
	"context"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
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

// GetFluxApplicationListForCluster Getting App list for the cluster
func (impl *FluxApplicationServiceImpl) GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList {
	impl.logger.Debugw("Fetching application list ", "clusterId", config.ClusterId, "clusterName", config.ClusterName)
	deployedApp := &client.FluxApplicationList{ClusterId: config.GetClusterId()}
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("Error in building rest config ", "clusterId", config.ClusterId, "err", err)
		impl.logger.Debugw("Error in building rest config ", "clusterId", config.ClusterId, "err", err)

		deployedApp.Errored = true
		deployedApp.ErrorMsg = err.Error()
		return deployedApp
	}

	//restConf := *restConfig
	kustomizationListResp, helmReleaseListResp, err := impl.fetchFluxK8sResponseLists(restConfig)
	if err != nil && kustomizationListResp == nil {
		impl.logger.Errorw("Error in fetching flux app k8s listResponse ", "clusterId", config.ClusterId, "err", err)
		deployedApp.Errored = true
		deployedApp.ErrorMsg = err.Error()
		return deployedApp
	}
	fluxAppList, err := impl.fetchFluxAppList(kustomizationListResp, helmReleaseListResp, config)
	if err != nil {
		impl.logger.Errorw("error in getting the list of flux apps", "err", err, "clusterId", config.ClusterId, "clusterName", config.ClusterName)
	}
	deployedApp.FluxApplication = convertFluxAppDetailsToDtos(fluxAppList)
	return deployedApp
}

// BuildFluxAppDetail Build Flux App Detail
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
		fluxAppTreeResponse, appStatus, err = impl.buildFluxAppDetailForKustomizationApp(req)
	} else {
		deploymentType = FluxAppHelmreleaseKind
		fluxAppTreeResponse, appStatus, err = impl.buildFluxAppDetailForHelmReleaseApp(req)
	}

	if err != nil && appStatus == nil {
		impl.logger.Errorw("error in getting fluxTreeResponse for flux app ", "err", err, "clusterId", request.ClusterConfig.ClusterId, "clusterName", request.ClusterConfig.ClusterName, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", deploymentType)
		return nil, err
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

	fluxKsAppDetail.TreeResponse = fluxAppTreeResponse
	return fluxKsAppDetail, nil
}

// get the k8sResponseList of the flux apps
func (impl *FluxApplicationServiceImpl) fetchFluxK8sResponseLists(restConfig *rest.Config) (*k8sUtils.ResourceListResponse, *k8sUtils.ResourceListResponse, error) {
	var restConf rest.Config
	restConf = *restConfig
	kustomizationList, err := impl.getK8sResponseList(restConfig, GvkForKustomizationFluxApp)
	if err != nil {
		return nil, nil, err
	}
	helmReleaseList, err := impl.getK8sResponseList(&restConf, GvkForHelmreleaseFluxApp)
	if err != nil {
		return kustomizationList, nil, err
	}
	return kustomizationList, helmReleaseList, nil
}

// fetching the list of flux app types HelmRelease and Kustomization
func (impl *FluxApplicationServiceImpl) fetchFluxAppList(kustomizationListResponse, helmReleaseListResponse *k8sUtils.ResourceListResponse, config *client.ClusterConfig) ([]*FluxApplicationDto, error) {
	var appDetailList []*FluxApplicationDto
	var err error
	if kustomizationListResponse.Resources.Object != nil {
		kustomizationAppLists, err := impl.GetApplicationListDtos(kustomizationListResponse.Resources, config.ClusterName, int(config.ClusterId), FluxAppKustomizationKind)
		if err != nil {
			impl.logger.Errorw("error in getting the listResponse of Kustomization apps ", "err", err, "clusterId", config.ClusterId, "err", err, "fluxAppListType", FluxAppKustomizationKind)
		}
		if len(kustomizationAppLists) > 0 {
			appDetailList = append(appDetailList, kustomizationAppLists...)
		}
	}
	if helmReleaseListResponse.Resources.Object != nil {
		helmReleaseAppLists, err := impl.GetApplicationListDtos(helmReleaseListResponse.Resources, config.ClusterName, int(config.ClusterId), HelmReleaseFluxAppType)
		if err != nil {
			impl.logger.Errorw("error in getting the listResponse of HelmRelease flux apps ", "err", err, "clusterId", config.ClusterId, "err", err, "fluxAppListType", FluxAppHelmreleaseKind)
		}
		if len(helmReleaseAppLists) > 0 {
			appDetailList = append(appDetailList, helmReleaseAppLists...)
		}
	}
	return appDetailList, err
}

// getKsAppInventoryMap gives inventories map that contains the resources  for given ks App using its name namespace and environment details
func (impl *FluxApplicationServiceImpl) getKsAppInventoryMap(app *FluxApplicationDto) (map[string]string, error) {
	cluster, err := impl.clusterRepository.FindById(app.EnvironmentDetails.ClusterId)
	if err != nil {
		impl.logger.Errorw("error in getting cluster repository", "err", err, "clusterId", app.EnvironmentDetails.ClusterId, "fluxKsName", app.Name, "namespace", app.EnvironmentDetails.Namespace)
		return nil, err
	}
	clusterInfo := impl.converter.GetClusterInfo(cluster)
	clusterConfig := impl.converter.GetClusterConfig(clusterInfo)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting cluster restConfig", "err", err, "clusterId", app.EnvironmentDetails.ClusterId, "fluxKsName", app.Name, "namespace", app.EnvironmentDetails.Namespace)
		return nil, err
	}

	resp, err := impl.k8sUtil.GetResource(context.Background(), app.EnvironmentDetails.Namespace, app.Name, GvkForKustomizationFluxApp, restConfig)
	if err != nil || resp == nil {
		impl.logger.Errorw("error in getting response", "err", err, "clusterId", app.EnvironmentDetails.ClusterId, "fluxKsName", app.Name, "namespace", app.EnvironmentDetails.Namespace)
		return nil, err
	}

	inventoryMap, err := getInventoryObjectsMapFromResponseObj(resp.Manifest.Object)
	if err != nil {
		impl.logger.Errorw("error in getting inventoryMap", "err", err, "clusterId", app.EnvironmentDetails.ClusterId, "fluxKsName", app.Name, "namespace", app.EnvironmentDetails.Namespace)
		return nil, err
	}
	return inventoryMap, nil
}

// GetApplicationListDtos fetching the list of filtered apps from resources for flux Apps i.e. combined the both Kustomization and flux type
func (impl *FluxApplicationServiceImpl) GetApplicationListDtos(resources unstructured.UnstructuredList, clusterName string, clusterId int, FluxAppType FluxAppType) ([]*FluxApplicationDto, error) {
	manifestObj := resources.Object
	var fluxAppDetailArray []*FluxApplicationDto

	columnDefinitions, found, err := unstructured.NestedSlice(manifestObj, k8sCommonBean.K8sClusterResourceColumnDefinitionKey)
	if err != nil || !found {
		impl.logger.Errorw("error in fetching columnDefinitions from Manifest", "err", err, "clusterId", clusterId, "FluxAppType", FluxAppType, "clusterName", clusterName)
		return fluxAppDetailArray, err
	}

	columnDefinitionMap := extractColumnDefinitions(columnDefinitions)

	rowsData, found, err := unstructured.NestedSlice(manifestObj, k8sCommonBean.K8sClusterResourceRowsKey)
	if err != nil || !found {
		impl.logger.Errorw("error in fetching rows Key from Manifest", "err", err, "clusterId", clusterId, "FluxAppType", FluxAppType, "clusterName", clusterName)
		return fluxAppDetailArray, err
	}

	childParentMap := make(map[string]bool)

	for _, rowData := range rowsData {
		rowDataMap, ok := rowData.(map[string]interface{})
		if !ok {
			continue
		}
		var appDetail *FluxApplicationDto
		appDetail = fetchFluxAppFields(rowDataMap, columnDefinitionMap, clusterId, clusterName, FluxAppType)

		if appDetail != nil && appDetail.FluxAppDeploymentType == FluxAppKustomizationKind {
			if appDetail.Name == "flux-system" && appDetail.EnvironmentDetails.Namespace == "flux-system" {
				continue
			}

			if appDetail.HealthStatus != "False" {

				childInventoryMap, err := impl.getKsAppInventoryMap(appDetail)
				if err != nil {
					impl.logger.Errorw("issue in decoding object metadata", "err", err)
					continue
				}

				for id, version := range childInventoryMap {
					fluxResource, err := decodeObjMetadata(id, version)
					if err != nil {
						impl.logger.Errorw("error in decoding Metadata details", "err", err, "id", id, "version", version, "clusterName", clusterName)
						continue
					}
					if fluxResource.Group == FluxKustomizationGroup && fluxResource.Kind == FluxAppKustomizationKind && fluxResource.Name != appDetail.Name && fluxResource.Namespace != appDetail.EnvironmentDetails.Namespace {
						childParentMap[fluxResource.Name] = true
					}
				}
			}
			fluxAppDetailArray = append(fluxAppDetailArray, appDetail)
		} else if appDetail != nil && appDetail.FluxAppDeploymentType == FluxAppHelmreleaseKind {
			fluxAppDetailArray = append(fluxAppDetailArray, appDetail)
		}

	}

	childFilteredAppList := make([]*FluxApplicationDto, 0, len(fluxAppDetailArray))
	for _, app := range fluxAppDetailArray {
		if !childParentMap[app.Name] {
			childFilteredAppList = append(childFilteredAppList, app)
		}
	}

	return childFilteredAppList, err
}

// getK8sResponseList fetch the k8s response List for flux apps according to its gvk types
func (impl *FluxApplicationServiceImpl) getK8sResponseList(restConfig *rest.Config, gvk schema.GroupVersionKind) (*k8sUtils.ResourceListResponse, error) {
	resp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, gvk, AllNamespaces, true, nil)
	if err != nil {
		impl.logger.Errorw("Error in fetching fluxAppList response ", "err", err, "fluxAppType", gvk.Kind)
		return nil, err
	}
	return resp, nil
}

// func (impl *FluxApplicationServiceImpl) InventoryListForKustomizationByClusterId(clusterId int)
//func (impl *FluxApplicationServiceImpl) childFilterMapping(appList []*FluxApplicationDto) []*FluxApplicationDto {
//	childParentMap := make(map[string]bool)
//	kustomizationMap := make(map[string]*FluxApplicationDto, len(appList)-1)
//	childFilteredAppList := make([]*FluxApplicationDto, 0)
//	for _, app := range appList {
//		if app.Name == "flux-system" && app.EnvironmentDetails.Namespace == "flux-system" {
//			continue
//		}
//		kustomizationMap[app.Name] = app
//
//		if app.HealthStatus != "False" {
//			cluster, err := impl.clusterRepository.FindById(app.EnvironmentDetails.ClusterId)
//			if err != nil {
//				impl.logger.Errorw("error in getting cluster repository", "err", err)
//			}
//			clusterInfo := impl.converter.GetClusterInfo(cluster)
//			clusterConfig := impl.converter.GetClusterConfig(clusterInfo)
//			restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
//			if err != nil {
//				impl.logger.Errorw("error in getting cluster config", "err", err)
//			}
//			resp, err := impl.k8sUtil.GetResource(context.Background(), app.EnvironmentDetails.Namespace, app.Name, GvkForKustomizationFluxApp, restConfig)
//			if err != nil || resp == nil {
//				impl.logger.Errorw("error in getting response", "err", err)
//			}
//
//			inventoryMap, err := getInventoryObjectsMapFromResponseObj(resp.Manifest.Object)
//			if err != nil {
//				impl.logger.Errorw("error in inventories", "err", err)
//			}
//
//			for id, version := range inventoryMap {
//				var fluxResource *FluxKsResourceDetail
//				fluxResource, err = decodeObjMetadata(id, version)
//				if err != nil {
//					fmt.Println("issue is here for some reason , r", err)
//				}
//				if fluxResource.Group == FluxKustomizationGroup && fluxResource.Kind == FluxAppKustomizationKind {
//					if fluxResource.Name == app.Name && fluxResource.Namespace == app.EnvironmentDetails.Namespace {
//						continue
//					}
//					childParentMap[fluxResource.Name] = true
//				}
//			}
//		}
//	}
//
//	for _, app := range appList {
//		if app.Name == "flux-system" && app.EnvironmentDetails.Namespace == "flux-system" {
//			continue
//		} else if childParentMap[app.Name] != true {
//			childFilteredAppList = append(childFilteredAppList, kustomizationMap[app.Name])
//		}
//	}
//	return childFilteredAppList
//}

// building the flux app detail of HelmRelease type
func (impl *FluxApplicationServiceImpl) buildFluxAppDetailForHelmReleaseApp(request *FluxAppDetailRequest) ([]*bean.ResourceTreeResponse, *FluxAppStatusDetail, error) {

	var fluxAppTreeResponse []*bean.ResourceTreeResponse

	k8sConfig := impl.converter.GetClusterConfigFromClientBean(request.Config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sConfig)
	resp, err := impl.k8sUtil.GetResource(context.Background(), request.Namespace, request.Name, GvkForHelmreleaseFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting resource response of flux app ", "err", err, "FluxAppName", request.Name, "appNamespace", request.Namespace, "FluxAppType", FluxAppKustomizationKind)
		return nil, nil, err
	}
	var releaseName string
	var _namespace string
	var appStatus *FluxAppStatusDetail

	if resp != nil && resp.Manifest.Object != nil {

		appStatus, err = getFluxAppStatus(resp.Manifest.Object, GvkForHelmreleaseFluxApp)
		if err != nil {
			impl.logger.Errorw("error in getting app status of flux app ", "err", err, "FluxAppName", request.Name, "appNamespace", request.Namespace, "FluxAppType", FluxAppKustomizationKind)
			return nil, nil, err
		}
		releaseName, _namespace, err = getReleaseNameNamespace(resp.Manifest.Object, request.Name)
		if err != nil || releaseName == "" || _namespace == "" {
			impl.logger.Errorw("error in getting the releaseName and namespace of flux app ", "err", err, "storageNamespace", _namespace, "FluxAppName", request.Name, "appNamespace", request.Namespace, "FluxAppType", FluxAppKustomizationKind)
			return nil, appStatus, err
		}

	}
	helmRelease, err := impl.common.GetHelmRelease(request.Config, _namespace, releaseName)
	if err != nil {
		impl.logger.Errorw("error in getting helm release of flux app ", "err", err, "releaseName", releaseName, "releaseNamespace", _namespace, "FluxAppName", request.Name, "appNamespace", request.Namespace, "FluxAppType", FluxAppKustomizationKind)
		return nil, appStatus, err
	}
	appDetailRequest := &client.AppDetailRequest{
		ReleaseName:   releaseName,
		Namespace:     _namespace,
		ClusterConfig: request.Config,
	}

	resourceTreeResponse, err := impl.common.BuildResourceTree(appDetailRequest, helmRelease)
	if err != nil {
		impl.logger.Errorw("error in building resource tree of flux app ", "err", err, "helmReleaseName", helmRelease.Name, "helmReleaseNamespace", helmRelease.Namespace, "FluxAppName", request.Name, "appNamespace", request.Namespace, "FluxAppType", FluxAppKustomizationKind)
		return nil, appStatus, err
	}
	fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)

	return fluxAppTreeResponse, appStatus, nil
}

// building the flux app detail of kustomization appType
func (impl *FluxApplicationServiceImpl) buildFluxAppDetailForKustomizationApp(request *FluxAppDetailRequest) ([]*bean.ResourceTreeResponse, *FluxAppStatusDetail, error) {
	var fluxAppTreeResponse []*bean.ResourceTreeResponse

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(request.Config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw(" error in getting the rest config", "err", err, "clusterId", request.Config.ClusterId, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
		return nil, nil, err
	}
	resp, err := impl.k8sUtil.GetResource(context.Background(), request.Namespace, request.Name, GvkForKustomizationFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting manifest of kustomization app  ", "err", err, "clusterId", request.Config.ClusterId, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
		return nil, nil, err
	}
	if resp == nil || resp.Manifest.Object == nil {
		impl.logger.Errorw("manifest of response is nil for kustomization app  ", "err", err, "clusterId", request.Config.ClusterId, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
		return nil, nil, err
	}
	appStatus, err := getFluxAppStatus(resp.Manifest.Object, GvkForKustomizationFluxApp)
	if err != nil {
		impl.logger.Errorw("error in getting appStatus of kustomization app  ", "err", err, "clusterId", request.Config.ClusterId, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
		return nil, nil, err
	}

	inventory, exists := fetchInventoryIfExists(resp.Manifest.Object)
	if !exists {
		impl.logger.Errorw("error in getting Inventory of kustomization app  ", "err", err, "clusterId", request.Config.ClusterId, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
		return nil, appStatus, err
	}

	_, isKubeConfigExist, err := unstructured.NestedFieldCopy(resp.Manifest.Object, SpecField, kubeConfigKey)
	if err != nil {
		impl.logger.Errorw("error in getting kubeconfig field of kustomization app  ", "err", err, "clusterId", request.Config.ClusterId, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
		return nil, appStatus, err
	}

	fluxK8sResourceList, fluxHrList, err := impl.getKsAppResourceAndFluxHrList(request, inventory, isKubeConfigExist)
	if err != nil {
		impl.logger.Errorw("error in getting kubeconfig field of kustomization app  ", "err", err, "clusterId", request.Config.ClusterId, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)

	}
	if len(fluxHrList) > 0 {
		hrTreeResponse, err := impl.getResponseTreeForKsChildrenHrList(request, fluxHrList)
		impl.logger.Errorw("Error in getting the tree response for all helmRelease in Kustomization ", "err", err, "clusterId", request.Config.ClusterId, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
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
			impl.logger.Errorw("Error in getting the tree response for all resources except HelmReleases in Kustomization ", "err", err, "clusterId", request.Config.ClusterId, "KsAppName", "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
			return nil, appStatus, err
		}
		if len(resourceTreeResponse.Nodes) > 0 {
			fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)
		}
	}
	return fluxAppTreeResponse, appStatus, err
}

// fetching response tree array for the FluxHr list found in the Kustomization inventory.
func (impl *FluxApplicationServiceImpl) getResponseTreeForKsChildrenHrList(request *FluxAppDetailRequest, fluxHrList []*FluxHr) ([]*bean.ResourceTreeResponse, error) {
	fluxAppTreeResponse := make([]*bean.ResourceTreeResponse, 0)
	var err error
	for _, fluxHr := range fluxHrList {
		releaseName := fluxHr.Name
		namespace := fluxHr.Namespace
		helmRelease, err := impl.common.GetHelmRelease(request.Config, namespace, releaseName)
		if err != nil {
			impl.logger.Errorw("error in getting helm release", "err", err, "clusterId", request.Config.ClusterId, "releaseName", releaseName, "namespace", namespace, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
			continue
		}
		appDetailRequest := &client.AppDetailRequest{
			ReleaseName:   releaseName,
			Namespace:     namespace,
			ClusterConfig: request.Config,
		}
		resourceTreeResponse, err := impl.common.BuildResourceTree(appDetailRequest, helmRelease)
		if err != nil {
			impl.logger.Errorw("error in building resource tree of helmRelease ", "err", err, "clusterId", request.Config.ClusterId, "helmRelease", helmRelease.Name, "namespace", helmRelease.Namespace, "fluxAppName", request.Name, "appNamespace", request.Namespace, "fluxAppType", FluxAppKustomizationKind)
			continue
		}
		fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)
	}
	return fluxAppTreeResponse, err
}

// getting list of FluxKustomizationApp k8s Resources List and  FluxHr list that contains the helmReleases found in inventory
func (impl *FluxApplicationServiceImpl) getKsAppResourceAndFluxHrList(app *FluxAppDetailRequest, inventory map[string]interface{}, isKubeConfigExist bool) ([]*client.ExternalResourceDetail, []*FluxHr, error) {
	var fluxK8sResourceList []*client.ExternalResourceDetail
	var fluxHrList []*FluxHr
	childResourcesList, err := fetchInventoryList(inventory)
	if err != nil {
		impl.logger.Errorw("error in getting inventory list of Kustomization Crd resource ", "err", err, "clusterId", app.Config.ClusterId, "fluxAppName", app.Name, "appNamespace", app.Namespace, "fluxAppType", FluxAppKustomizationKind)
		return nil, nil, err
	}
	for _, childResource := range childResourcesList {
		fluxK8sResourceList = append(fluxK8sResourceList, &client.ExternalResourceDetail{
			Namespace: childResource.Namespace,
			Name:      childResource.Name,
			Group:     childResource.Group,
			Kind:      childResource.Kind,
			Version:   childResource.Version,
		})
		//we are skipping if the resource is having parent ks attributes, its inventory it contains the parent ks too

		if childResource.Group == FluxKustomizationGroup && childResource.Kind == FluxAppKustomizationKind && childResource.Name == app.Name && childResource.Namespace == app.Namespace {
			continue
		}
		if childResource.Group == FluxHelmReleaseGroup && childResource.Kind == FluxAppHelmreleaseKind {
			var releaseName, nameSpace string
			releaseName, nameSpace, err = impl.getFluxReleaseNameNamespaceForKsChild(childResource.Name, childResource.Namespace, app.Config)
			if err != nil || releaseName == "" || nameSpace == "" {
				impl.logger.Errorw("error in parsing the namespace and releaseName from helmRelease Crd resource ", "err", err, "clusterId", app.Config.ClusterId, "helmRelease", childResource.Name, "namespace", childResource.Namespace, "fluxAppName", app.Name, "appNamespace", app.Namespace, "fluxAppType", FluxAppKustomizationKind)
				return nil, nil, err
			}
			fluxHrList = append(fluxHrList, &FluxHr{Name: releaseName, Namespace: nameSpace})
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
				impl.logger.Errorw("error in getting the k8s cluster config child kustomization resource ", "err", err, "clusterId", app.Config.ClusterId, "childKsName", childResource.Name, "namespace", childResource.Namespace)
				return nil, nil, err
			}
			resp, err := impl.k8sUtil.GetResource(context.Background(), childResource.Namespace, childResource.Name, GvkForKustomizationFluxApp, restConfig)
			if err != nil {
				impl.logger.Errorw("error in getting the k8s response of child kustomization resource", "err", err, "clusterId", app.Config.ClusterId, "childKsName", childResource.Name, "namespace", childResource.Namespace)
				return nil, nil, err
			}
			if resp == nil || resp.Manifest.Object == nil {
				impl.logger.Errorw("error in getting the k8s response of child kustomization resource ", "err", err, "clusterId", app.Config.ClusterId, "childKsName", childResource.Name, "namespace", childResource.Namespace)

				return nil, nil, nil
			}
			_, isKubeConfigExistOfChild, err := unstructured.NestedFieldCopy(resp.Manifest.Object, SpecField, kubeConfigKey)
			if err != nil {
				impl.logger.Errorw("error in getting the kubeconfig Key in the child Kustomization resource", "err", err, "childKsName", childResource.Name, "namespace", childResource.Namespace)
				return nil, nil, err
			}
			if childInventory, exists := fetchInventoryIfExists(resp.Manifest.Object); exists {
				childFluxK8sResourceList, childFluxHrList, err := impl.getKsAppResourceAndFluxHrList(fluxKsAppChild, childInventory, isKubeConfigExistOfChild)
				if err != nil {
					impl.logger.Errorw("error in getting the getting the KsResourceList and ksFluxHrList of child Kustomization resource ", "err", err, "clusterId", app.Config.ClusterId, "childKsName", childResource.Name, "namespace", childResource.Namespace)
					return nil, nil, err
				}
				fluxK8sResourceList = append(fluxK8sResourceList, childFluxK8sResourceList...)
				fluxHrList = append(fluxHrList, childFluxHrList...)
			}
		}
	}
	return fluxK8sResourceList, fluxHrList, nil
}

// extracting the releaseName and Namespace of the child component for resource type helmRelease Crd found in kustomization inventory
func (impl *FluxApplicationServiceImpl) getFluxReleaseNameNamespaceForKsChild(name string, namespace string, config *client.ClusterConfig) (string, string, error) {
	var releaseName, _namespace string

	k8sConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sConfig)
	resp, err := impl.k8sUtil.GetResource(context.Background(), namespace, name, GvkForHelmreleaseFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting manifest of flux helmRelease", "err", err, "clusterId", config.ClusterId, "HelmReleaseCrdName", name, "crdNamespace", namespace)
		return releaseName, _namespace, err
	}
	//var appStatus *FluxAppStatusDetail
	if resp != nil && resp.Manifest.Object != nil {
		releaseName, _namespace, err = getReleaseNameNamespace(resp.Manifest.Object, name)
		//appStatus, err = getFluxAppStatus(resp.Manifest.Object, GvkForHelmreleaseFluxApp)
		if err != nil || releaseName == "" || _namespace == "" {
			impl.logger.Errorw("error in releaseName and namespace from flux helmRelease response", "err", err, "clusterId", config.ClusterId, "HelmReleaseCrdName", name, "crdNamespace", namespace)
			return releaseName, _namespace, err
		}
	}
	return releaseName, _namespace, nil
}

//func (impl *FluxApplicationServiceImpl) getKsAppResourceAndFluxHrList(app *FluxAppDetailRequest, fluxK8sResourceList []*client.ExternalResourceDetail, fluxHrList []*FluxHr, obj map[string]interface{}) {
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
//			releaseName, nameSpace, _, err = impl.getFluxReleaseNameNamespaceForKsChild(childResource.Name, childResource.Namespace, app.Config)
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
//			if fetchInventoryIfExists(resp.Manifest.Object) {
//				impl.getKsAppResourceAndFluxHrList(fluxKsAppChild, fluxK8sResourceList, fluxHrList, resp.Manifest.Object)
//
//			}
//
//		}
//	}
//	return
//}
