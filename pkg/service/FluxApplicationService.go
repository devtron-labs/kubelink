package service

import (
	"context"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/kubelink/converter"
	"github.com/devtron-labs/kubelink/fluxApplication/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	clusterRepository "github.com/devtron-labs/kubelink/pkg/cluster"
	"go.uber.org/zap"
)

type FluxApplicationService interface {
	GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList
}

type FluxApplicationServiceImpl struct {
	logger            *zap.SugaredLogger
	clusterRepository clusterRepository.ClusterRepository
	k8sUtil           k8sUtils.K8sService
	helmAppService    HelmAppService
	converter         converter.ClusterBeanConverter
}

func NewFluxApplicationServiceImpl(logger *zap.SugaredLogger,
	clusterRepository clusterRepository.ClusterRepository,
	k8sUtil k8sUtils.K8sService,
	helmAppService HelmAppService, converter converter.ClusterBeanConverter) *FluxApplicationServiceImpl {
	return &FluxApplicationServiceImpl{
		logger:            logger,
		clusterRepository: clusterRepository,
		k8sUtil:           k8sUtil,
		helmAppService:    helmAppService,
		converter:         converter,
	}

}

func (impl *FluxApplicationServiceImpl) GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList {
	impl.logger.Debugw("Fetching application list ", "clusterId", config.ClusterId, "clusterName", config.ClusterName)

	appListFinal := make([]*bean.FluxApplicationDto, 0)
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("Error in building rest config ", "clusterId", config.ClusterId, "err", err)
		return &client.FluxApplicationList{}
	}
	restConfig2, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting rest config ", "err", err, "clusterId", config.ClusterId)
		return &client.FluxApplicationList{}
	}

	kustomizationResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, bean.GvkForKustomizationFluxApp, bean.AllNamespaces, true, nil)
	if err == nil {
		kustomizationAppLists := getApplicationListDtos(kustomizationResp.Resources.Object, config.ClusterName, int(config.ClusterId), "")
		if len(kustomizationAppLists) > 0 {
			appListFinal = append(appListFinal, kustomizationAppLists...)
		}
	}

	helmReleaseResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig2, bean.GvkForHelmreleaseFluxApp, bean.AllNamespaces, true, nil)
	if err == nil {
		helmReleaseAppLists := getApplicationListDtos(helmReleaseResp.Resources.Object, config.ClusterName, int(config.ClusterId), "HelmRelease")
		if len(helmReleaseAppLists) > 0 {
			appListFinal = append(appListFinal, helmReleaseAppLists...)
		}

	}

	appListFinalDto := make([]*client.FluxApplicationDetail, 0)

	for _, appDetail := range appListFinal {
		appDetailDto := &client.FluxApplicationDetail{
			Name:           appDetail.Name,
			HealthStatus:   appDetail.HealthStatus,
			SyncStatus:     appDetail.SyncStatus,
			IsKustomizeApp: appDetail.IsKustomizeApp,
			EnvironmentDetail: &client.EnvironmentDetails{
				ClusterName: appDetail.EnvironmentDetails.ClusterName,
				ClusterId:   int32(appDetail.EnvironmentDetails.ClusterId),
				Namespace:   appDetail.EnvironmentDetails.Namespace,
			},
		}
		appListFinalDto = append(appListFinalDto, appDetailDto)
	}
	finalAppListDto := &client.FluxApplicationList{
		ClusterId:             config.ClusterId,
		FluxApplicationDetail: appListFinalDto,
	}
	return finalAppListDto
}
func getApplicationListDtos(manifestObj map[string]interface{}, clusterName string, clusterId int, FluxAppType string) []*bean.FluxApplicationDto {
	fluxAppDetailArray := make([]*bean.FluxApplicationDto, 0)
	// map of keys and index in row cells, initially set as 0 will be updated by object
	keysToBeFetchedFromColumnDefinitions := map[string]int{k8sCommonBean.K8sResourceColumnDefinitionName: 0, "Ready": 0,
		"Status": 0}
	keysToBeFetchedFromRawObject := []string{k8sCommonBean.K8sClusterResourceNamespaceKey}

	columnsDataRaw := manifestObj[k8sCommonBean.K8sClusterResourceColumnDefinitionKey]
	if columnsDataRaw != nil {
		columnsData := columnsDataRaw.([]interface{})
		for i, columnData := range columnsData {
			columnDataMap := columnData.(map[string]interface{})
			for key := range keysToBeFetchedFromColumnDefinitions {
				if columnDataMap[k8sCommonBean.K8sClusterResourceNameKey] == key {
					keysToBeFetchedFromColumnDefinitions[key] = i
				}
			}
		}
	}

	rowsDataRaw := manifestObj[k8sCommonBean.K8sClusterResourceRowsKey]
	if rowsDataRaw != nil {
		rowsData := rowsDataRaw.([]interface{})
		for _, rowData := range rowsData {
			var namespace, name, syncStatus, healthStatus string
			isKustomize := true

			rowDataMap := rowData.(map[string]interface{})
			rowObject := rowDataMap[k8sCommonBean.K8sClusterResourceObjectKey].(map[string]interface{})
			metadata := rowObject[k8sCommonBean.K8sClusterResourceMetadataKey].(map[string]interface{})

			if FluxAppType == "HelmRelease" {

				// Check for the existence and non-empty values of the required labels
				labels := metadata["labels"].(map[string]interface{})
				nameLabel, nameExists := labels["kustomize.toolkit.fluxcd.io/name"].(string)
				namespaceLabel, namespaceExists := labels["kustomize.toolkit.fluxcd.io/namespace"].(string)
				if nameExists && nameLabel != "" && namespaceExists && namespaceLabel != "" {
					continue
				}
				isKustomize = false
			}

			rowCells := rowDataMap[k8sCommonBean.K8sClusterResourceCellKey].([]interface{})
			for key, value := range keysToBeFetchedFromColumnDefinitions {
				resolvedValueFromRowCell := rowCells[value].(string)
				switch key {
				case k8sCommonBean.K8sResourceColumnDefinitionName:
					name = resolvedValueFromRowCell
				case "Status":
					syncStatus = resolvedValueFromRowCell
				case "Ready":
					healthStatus = resolvedValueFromRowCell
				}
			}

			for _, key := range keysToBeFetchedFromRawObject {
				switch key {
				case k8sCommonBean.K8sClusterResourceNamespaceKey:
					namespace = metadata[k8sCommonBean.K8sClusterResourceNamespaceKey].(string)
				}
			}

			appDto := &bean.FluxApplicationDto{
				Name:         name,
				HealthStatus: healthStatus,
				SyncStatus:   syncStatus,
				EnvironmentDetails: &bean.EnvironmentDetail{
					ClusterId:   clusterId,
					ClusterName: clusterName,
					Namespace:   namespace,
				},
				IsKustomizeApp: isKustomize,
			}

			fluxAppDetailArray = append(fluxAppDetailArray, appDto)
		}
	}
	return fluxAppDetailArray
}
