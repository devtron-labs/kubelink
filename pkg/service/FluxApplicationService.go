package service

import (
	"context"
	"fmt"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/converter"
	client "github.com/devtron-labs/kubelink/grpc"
	clusterRepository "github.com/devtron-labs/kubelink/pkg/cluster"
	FluxApplicationService2 "github.com/devtron-labs/kubelink/pkg/service/FluxApplicationService"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"strings"
)

type FluxApplicationService interface {
	GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList
	//BuildFluxAppDetail(request FluxAppDetailRequest) (*bean.ResourceTreeResponse, error)
}

type FluxApplicationServiceImpl struct {
	logger            *zap.SugaredLogger
	clusterRepository clusterRepository.ClusterRepository
	k8sUtil           k8sUtils.K8sService
	converter         converter.ClusterBeanConverter
	common            CommonUtilsI
}

func NewFluxApplicationServiceImpl(logger *zap.SugaredLogger,
	clusterRepository clusterRepository.ClusterRepository,
	k8sUtil k8sUtils.K8sService,
	converter converter.ClusterBeanConverter, common CommonUtilsI) *FluxApplicationServiceImpl {
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

	appListFinal := make([]*FluxApplicationService2.FluxApplicationDto, 0)
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("Error in building rest config ", "clusterId", config.ClusterId, "err", err)
		return &client.FluxApplicationList{}
	}

	var restConfig2 rest.Config
	restConfig2 = *restConfig
	kustomizationResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, FluxApplicationService2.GvkForKustomizationFluxApp, FluxApplicationService2.AllNamespaces, true, nil)
	if err != nil {
		impl.logger.Errorw("Error in fetching kustomizationList Resource", "err", err)
		return &client.FluxApplicationList{}
	} else {
		if kustomizationResp != nil {
			kustomizationAppLists := FluxApplicationService2.getApplicationListDtos(kustomizationResp.Resources, config.ClusterName, int(config.ClusterId), "")
			if len(kustomizationAppLists) > 0 {
				appListFinal = append(appListFinal, kustomizationAppLists...)
			}
		}
	}

	helmReleaseResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), &restConfig2, FluxApplicationService2.GvkForHelmreleaseFluxApp, FluxApplicationService2.AllNamespaces, true, nil)
	if err != nil {
		impl.logger.Errorw("Error in fetching helmReleaseList Resources", "err", err)
		return &client.FluxApplicationList{}
	} else {
		if helmReleaseResp != nil {
			helmReleaseAppLists := FluxApplicationService2.getApplicationListDtos(helmReleaseResp.Resources, config.ClusterName, int(config.ClusterId), FluxApplicationService2.HelmReleaseFluxAppType)
			if len(helmReleaseAppLists) > 0 {
				appListFinal = append(appListFinal, helmReleaseAppLists...)
			}
		}
	}

	appListFinalDto := make([]*client.FluxApplicationDetail, 0)

	for _, appDetail := range appListFinal {
		fluxAppDetailDto := FluxApplicationService2.getFluxAppDetailDto(appDetail)
		appListFinalDto = append(appListFinalDto, fluxAppDetailDto)
	}
	finalAppListDto := &client.FluxApplicationList{
		ClusterId:             config.ClusterId,
		FluxApplicationDetail: appListFinalDto,
	}
	return finalAppListDto
}
func (impl *FluxApplicationServiceImpl) BuildFluxAppDetail(request FluxApplicationService2.FluxAppDetailRequest) ([]*bean.ResourceTreeResponse, error) {
	fluxAppTreeResponse := make([]*bean.ResourceTreeResponse, 0)
	if request.IsKustomize {
		fluxK8sResourceList := make([]*client.ExternalResourceDetail, 0)
		fluxHrList := make([]*FluxApplicationService2.FluxHr, 0)
		err := impl.getFluxResourceAndFluxHrList(&request, fluxK8sResourceList, fluxHrList)
		if err != nil {
			return fluxAppTreeResponse, err
		}

		if len(fluxHrList) > 0 {
			for _, fluxHr := range fluxHrList {
				releaseName, namespace, err := impl.getHelmReleaseInventory(fluxHr.Name, fluxHr.Namespace, request.Config)
				if err != nil {
					return fluxAppTreeResponse, err
				}

				helmRelease, err := impl.common.GetHelmRelease(request.Config, namespace, releaseName)
				if err != nil {
					return fluxAppTreeResponse, err
				}

				appDetailRequest := &client.AppDetailRequest{
					ReleaseName:   releaseName,
					Namespace:     namespace,
					ClusterConfig: request.Config,
				}

				resourceTreeResponse, err := impl.common.BuildResourceTree(appDetailRequest, helmRelease)
				if err != nil {
					return fluxAppTreeResponse, err

				}

				fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)
			}
		}

		if len(fluxK8sResourceList) > 0 {
			req := &client.ExternalResourceTreeRequest{
				ClusterConfig:          request.Config,
				ExternalResourceDetail: fluxK8sResourceList,
			}

			resourceTreeResponse, err := impl.common.GetResourceTreeForExternalResources(req)
			if err != nil {
				return fluxAppTreeResponse, err
			}

			fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)
		}

	} else {
		releaseName, namespace, err := impl.getHelmReleaseInventory(request.Name, request.Namespace, request.Config)
		if err != nil {
			return fluxAppTreeResponse, err
		}

		helmRelease, err := impl.common.GetHelmRelease(request.Config, namespace, releaseName)
		if err != nil {
			return fluxAppTreeResponse, err
		}

		appDetailRequest := &client.AppDetailRequest{
			ReleaseName:   releaseName,
			Namespace:     namespace,
			ClusterConfig: request.Config,
		}

		resourceTreeResponse, err := impl.common.BuildResourceTree(appDetailRequest, helmRelease)
		if err != nil {
			return fluxAppTreeResponse, err

		}
		fluxAppTreeResponse = append(fluxAppTreeResponse, resourceTreeResponse)

		return fluxAppTreeResponse, nil

	}
	return fluxAppTreeResponse, nil

}
func (impl *FluxApplicationServiceImpl) getFluxResourceAndFluxHrList(app *FluxApplicationService2.FluxAppDetailRequest, fluxK8sResourceList []*client.ExternalResourceDetail, fluxHrList []*FluxApplicationService2.FluxHr) error {

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(app.Config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		return err
	}
	resp, err := impl.k8sUtil.GetResource(context.Background(), app.Namespace, app.Name, FluxApplicationService2.GvkForKustomizationFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting resource list", "err", err)
		return err
	}
	if resp == nil && resp.Manifest.Object == nil {
		return err

	}
	obj := resp.Manifest.Object

	inventoryMap := getInventoryMap(obj)
	fluxKsSpecKubeConfig := getFluxKsSpecKubeConfig(obj)

	for id, version := range inventoryMap {
		var fluxResource FluxApplicationService2.FluxKsAppDetail
		fluxResource, err = parseObjMetadata(id)
		if err != nil {
			fmt.Println("issue is here for some reason , r", err)
		}
		fluxResource.Version = version
		if fluxResource.Group == FluxApplicationService2.FluxKustomizationGroup && fluxResource.Kind == FluxApplicationService2.FluxAppKustomizationKind && fluxResource.Name == app.Name && fluxResource.Namespace == app.Namespace {
			continue
		}

		if fluxResource.Kind != FluxApplicationService2.FluxAppHelmreleaseKind && fluxResource.Kind != FluxApplicationService2.FluxAppKustomizationKind {
			//fluxK8sResources = append(fluxK8sResources, &fluxResource)
			fluxK8sResourceList = append(fluxK8sResourceList, &client.ExternalResourceDetail{
				Namespace: fluxResource.Namespace,
				Name:      fluxResource.Name,
				Group:     fluxResource.Group,
				Version:   fluxResource.Version,
				Kind:      fluxResource.Kind,
			})
		}

		if fluxResource.Group == FluxApplicationService2.FluxHelmReleaseGroup &&
			fluxResource.Kind == FluxApplicationService2.FluxAppHelmreleaseKind {
			var releaseName, nameSpace string
			releaseName, nameSpace, err = impl.getHelmReleaseInventory(fluxResource.Name, fluxResource.Namespace, app.Config)
			if err != nil {
				fmt.Println(err)
			}
			fluxK8sResourceList = append(fluxK8sResourceList, &client.ExternalResourceDetail{
				Namespace: fluxResource.Namespace,
				Name:      fluxResource.Name,
				Group:     fluxResource.Group,
				Kind:      fluxResource.Kind,
				Version:   fluxResource.Version,
			})

			fluxHrList = append(fluxHrList, &FluxApplicationService2.FluxHr{Name: releaseName, Namespace: nameSpace})
		}

		if fluxResource.Group == FluxApplicationService2.FluxKustomizationGroup &&
			fluxResource.Kind == FluxApplicationService2.FluxAppKustomizationKind &&
			// skip kustomization if it targets a remote clusters
			fluxKsSpecKubeConfig == false {
			fluxK8sResourceList = append(fluxK8sResourceList, &client.ExternalResourceDetail{
				Namespace: fluxResource.Namespace,
				Name:      fluxResource.Name,
				Group:     fluxResource.Group,
				Kind:      fluxResource.Kind,
				Version:   fluxResource.Version,
			})
			fluxKsAppChild := &FluxApplicationService2.FluxAppDetailRequest{
				Name:        fluxResource.Name,
				Namespace:   fluxResource.Namespace,
				IsKustomize: true,
				Config:      app.Config,
			}
			err = impl.getFluxResourceAndFluxHrList(fluxKsAppChild, fluxK8sResourceList, fluxHrList)
			if err != nil {
				fmt.Println(err)
			}
		}

	}

	return nil
}
func (impl *FluxApplicationServiceImpl) getHelmReleaseInventory(name string, namespace string, config *client.ClusterConfig) (string, string, error) {
	k8sConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sConfig)
	resp, err := impl.k8sUtil.GetResource(context.Background(), namespace, name, FluxApplicationService2.GvkForHelmreleaseFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting resource list", "err", err)
		return "", "", err
	}
	var releaseName, _namespace string
	if resp != nil && resp.Manifest.Object != nil {
		releaseName, _namespace = getReleaseNameNamespace(resp.Manifest.Object)
	}
	return releaseName, _namespace, nil

}
func getFluxKsSpecKubeConfig(obj map[string]interface{}) bool {
	return false
}
func parseObjMetadata(s string) (FluxApplicationService2.FluxKsAppDetail, error) {
	index := strings.Index(s, FluxApplicationService2.FieldSeparator)
	if index == -1 {
		return FluxApplicationService2.NilObjMetadata, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	namespace := s[:index]
	s = s[index+1:]
	// Next, parse last field kind
	index = strings.LastIndex(s, FluxApplicationService2.FieldSeparator)
	if index == -1 {
		return FluxApplicationService2.NilObjMetadata, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	kind := s[index+1:]
	s = s[:index]
	// Next, parse next to last field group
	index = strings.LastIndex(s, FluxApplicationService2.FieldSeparator)
	if index == -1 {
		return FluxApplicationService2.NilObjMetadata, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	group := s[index+1:]
	// Finally, second field name. Name may contain colon transcoded as double underscore.
	name := s[:index]
	name = strings.ReplaceAll(name, FluxApplicationService2.ColonTranscoded, ":")
	// Check that there are no extra fields by search for fieldSeparator.
	if strings.Contains(name, FluxApplicationService2.FieldSeparator) {
		return FluxApplicationService2.NilObjMetadata, fmt.Errorf("too many fields within: %s", s)
	}
	// Create the ObjMetadata object from the four parsed fields.
	id := FluxApplicationService2.FluxKsAppDetail{
		Namespace: namespace,
		Name:      name,
		Group:     group,
		Kind:      kind,
	}
	return id, nil
}
func getInventoryMap(obj map[string]interface{}) map[string]string {
	fluxManagedResourcesMap := make(map[string]string)
	if statusRawObj, ok := obj[FluxApplicationService2.STATUS]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		if inventoryRawObj, ok2 := statusObj[FluxApplicationService2.INVENTORY]; ok2 {
			inventoryObj := inventoryRawObj.(map[string]interface{})
			if entriesRawObj, ok3 := inventoryObj[FluxApplicationService2.ENTRIES]; ok3 {
				entriesObj := entriesRawObj.([]interface{})
				for _, itemRawObj := range entriesObj {
					itemObj := itemRawObj.(map[string]interface{})
					var matadataCompact FluxApplicationService2.ObjectMetadataCompact
					if metadataRaw, ok4 := itemObj[FluxApplicationService2.ID]; ok4 {
						metadata := metadataRaw.(string)
						matadataCompact.Id = metadata
					}
					if metadataVersionRaw, ok5 := itemObj[FluxApplicationService2.VERSION]; ok5 {
						metadataVersion := metadataVersionRaw.(string)
						matadataCompact.Version = metadataVersion
					}
					fluxManagedResourcesMap[matadataCompact.Id] = matadataCompact.Version
				}
			}
		}
	}
	return fluxManagedResourcesMap
}
func getReleaseNameNamespace(obj map[string]interface{}) (string, string) {

	var releaseName, storageNamespace string

	if statusRawObj, ok := obj[FluxApplicationService2.STATUS]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		if storagens, ok6 := statusObj["storageNamespace"]; ok6 {
			storageNamespace = storagens.(string)
		}
		if historyRawObj, ok1 := statusObj["history"]; ok1 {
			historyObj := historyRawObj.([]interface{})

			firstValOfHistoryRaw := historyObj[0]
			firstValOfHistory := firstValOfHistoryRaw.(map[string]interface{})

			if chartName, ok3 := firstValOfHistory["name"]; ok3 {
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
	releaseName = fmt.Sprintf("sh.helm.release.v1.%s", releaseName)

	return releaseName, storageNamespace
}
