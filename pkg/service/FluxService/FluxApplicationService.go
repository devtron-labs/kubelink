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
	"strings"
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
			kustomizationAppLists := GetApplicationListDtos(kustomizationResp.Resources, config.ClusterName, int(config.ClusterId), "")
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

	appListFinalDto := make([]*client.FluxApplicationDetail, 0)

	for _, appDetail := range appListFinal {
		fluxAppDetailDto := GetFluxAppDetailDto(appDetail)
		appListFinalDto = append(appListFinalDto, fluxAppDetailDto)
	}
	finalAppListDto := &client.FluxApplicationList{
		ClusterId:             config.ClusterId,
		FluxApplicationDetail: appListFinalDto,
	}
	return finalAppListDto
}
func (impl *FluxApplicationServiceImpl) BuildFluxAppDetail(request *client.FluxAppDetailRequest) (*FluxKsAppDetail, error) {
	var fluxAppTreeResponse []*bean.ResourceTreeResponse
	var err error
	var appStatus *FluxAppStatusDetail
	req := &FluxAppDetailRequest{
		Name:        request.Name,
		Config:      request.ClusterConfig,
		Namespace:   request.Namespace,
		IsKustomize: request.IsKustomizeApp,
	}
	if request.IsKustomizeApp {
		fluxAppTreeResponse, appStatus, err = impl.buildFluxAppDetailForKustomize(req)
	} else {
		fluxAppTreeResponse, appStatus, err = impl.buildFluxAppDetailForHelmRelease(req)
	}

	if err != nil || appStatus != nil {

		if appStatus != nil {
			return &FluxKsAppDetail{
				Name: request.Name,
				EnvironmentDetail: &EnvironmentDetail{
					ClusterId:   int(req.Config.ClusterId),
					ClusterName: req.Config.ClusterName,
					Namespace:   req.Namespace,
				},
				IsKustomize:  req.IsKustomize,
				AppStatusDto: appStatus,
			}, nil
		}
		return nil, err
	}

	fluxKsAppDetail := &FluxKsAppDetail{
		Name: request.Name,
		EnvironmentDetail: &EnvironmentDetail{
			ClusterId:   int(req.Config.ClusterId),
			ClusterName: req.Config.ClusterName,
			Namespace:   req.Namespace,
		},
		IsKustomize:  req.IsKustomize,
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
func getFluxSpecKubeConfig(obj map[string]interface{}) bool {
	if statusRawObj, ok := obj["spec"]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		if _, ok2 := statusObj["kubeConfig"]; ok2 {
			return true
		} else {
			return false
		}
	} else {
		return false
	}
}
func parseObjMetadata(s string) (FluxKsResourceDetail, error) {
	index := strings.Index(s, FieldSeparator)
	if index == -1 {
		return NilObjMetadata, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	namespace := s[:index]
	s = s[index+1:]
	// Next, parse last field kind
	index = strings.LastIndex(s, FieldSeparator)
	if index == -1 {
		return NilObjMetadata, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	kind := s[index+1:]
	s = s[:index]
	// Next, parse next to last field group
	index = strings.LastIndex(s, FieldSeparator)
	if index == -1 {
		return NilObjMetadata, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	group := s[index+1:]
	// Finally, second field name. Name may contain colon transcoded as double underscore.
	name := s[:index]
	name = strings.ReplaceAll(name, ColonTranscoded, ":")
	// Check that there are no extra fields by search for fieldSeparator.
	if strings.Contains(name, FieldSeparator) {
		return NilObjMetadata, fmt.Errorf("too many fields within: %s", s)
	}
	// Create the ObjMetadata object from the four parsed fields.
	id := FluxKsResourceDetail{
		Namespace: namespace,
		Name:      name,
		Group:     group,
		Kind:      kind,
	}
	return id, nil
}
func getInventoryMap(obj map[string]interface{}) (map[string]string, error) {
	fluxManagedResourcesMap := make(map[string]string)
	if statusRawObj, ok := obj[STATUS]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		if inventoryRawObj, ok2 := statusObj[INVENTORY]; ok2 {
			inventoryObj := inventoryRawObj.(map[string]interface{})
			if entriesRawObj, ok3 := inventoryObj[ENTRIES]; ok3 {
				entriesObj := entriesRawObj.([]interface{})
				for _, itemRawObj := range entriesObj {
					itemObj := itemRawObj.(map[string]interface{})
					var matadataCompact ObjectMetadataCompact
					if metadataRaw, ok4 := itemObj[ID]; ok4 {
						metadata := metadataRaw.(string)
						matadataCompact.Id = metadata
					}
					if metadataVersionRaw, ok5 := itemObj[VERSION]; ok5 {
						metadataVersion := metadataVersionRaw.(string)
						matadataCompact.Version = metadataVersion
					}
					fluxManagedResourcesMap[matadataCompact.Id] = matadataCompact.Version
				}
			}
		} else {
			return nil, fmt.Errorf("inventory not found for flux application service2 inventory")
		}

	}
	return fluxManagedResourcesMap, nil
}
func getReleaseNameNamespace(obj map[string]interface{}) (string, string) {

	var releaseName, storageNamespace string

	if statusRawObj, ok := obj[STATUS]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		if storagens, ok6 := statusObj["storageNamespace"]; ok6 {
			storageNamespace = storagens.(string)
		}
		if historyRawObj, ok1 := statusObj["history"]; ok1 {
			historyObj := historyRawObj.([]interface{})

			lastValOfHistoryRaw := historyObj[len(historyObj)-1]
			lastValOfHistory := lastValOfHistoryRaw.(map[string]interface{})

			if chartName, ok3 := lastValOfHistory["name"]; ok3 {
				releaseName = chartName.(string)
			}
			//if cVersion, ok4 := firstvalOfHistory["chartVersion"]; ok4 {
			//	version = cVersion.(string)
			//
			//}
			//if ns, ok7 := firstvalOfHistory["namespace"]; ok7 {
			//	storageNamespace = ns.(string)
			//}
		}

	}
	//releaseName = fmt.Sprintf("sh.helm.release.v1.%s", releaseName)

	return releaseName, storageNamespace
}
func getKsAppStatus(obj map[string]interface{}) (*FluxAppStatusDetail, error) {
	var status, reason, message string

	if statusRawObj, ok := obj[STATUS]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		if conditionsRawObj, ok2 := statusObj["conditions"]; ok2 {
			conditionsObj := conditionsRawObj.([]interface{})
			lastIndex := len(conditionsObj)
			itemRawObj := conditionsObj[lastIndex-1]
			itemObj := itemRawObj.(map[string]interface{})
			if statusValRaw, ok4 := itemObj["status"]; ok4 {
				status = statusValRaw.(string)
			}
			if reasonRawVal, ok5 := itemObj["reason"]; ok5 {
				reason = reasonRawVal.(string)
			}
			if messageRaw, ok6 := itemObj["message"]; ok6 {
				message = messageRaw.(string)
			}
		} else {
			return nil, fmt.Errorf("inventory not found for flux application service2 inventory")
		}

	}

	return &FluxAppStatusDetail{
		Status:  status,
		Reason:  reason,
		Message: message,
	}, nil
}
