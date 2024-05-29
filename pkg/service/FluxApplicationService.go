package service

import (
	"context"

	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/kubelink/converter"
	"github.com/devtron-labs/kubelink/fluxApplication/bean"
	clusterRepository "github.com/devtron-labs/kubelink/pkg/cluster"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
)

type FluxApplicationService interface {
	ListApplications(clusterIds []int) ([]*bean.FluxApplicationListDto, error)
	//GetAppDetail(resourceName, resourceNamespace string, clusterId int) (*bean.FluxApplicationListDto, error)
	//GetServerConfigIfClusterIsNotAddedOnDevtron(resourceResp *k8s.ManifestResponse, restConfig *rest.Config,
	//	clusterWithApplicationObject clusterRepository.Cluster, clusterServerUrlIdMap map[string]int) (*rest.Config, error)
	//GetClusterConfigFromAllClusters(clusterId int) (*k8s.ClusterConfig, clusterRepository.Cluster, map[string]int, error)
	//GetRestConfigForExternalArgo(ctx context.Context, clusterId int, externalFluxApplicationName string) (*rest.Config, error)
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

func (impl *FluxApplicationServiceImpl) ListApplications(clusterIds []int) ([]*bean.FluxApplicationListDto, error) {
	var clusters []*clusterRepository.Cluster
	var err error
	if len(clusterIds) > 0 {
		// getting cluster details by ids
		clusters, err = impl.clusterRepository.FindByIds(clusterIds)
		if err != nil {
			impl.logger.Errorw("error in getting clusters by ids", "err", err, "clusterIds", clusterIds)
			return nil, err
		}
	} else {
		clusters, err = impl.clusterRepository.FindAllActive()
		if err != nil {
			impl.logger.Errorw("error in getting all active clusters", "err", err)
			return nil, err
		}
	}

	// TODO: make goroutine and channel for optimization
	appListFinal := make([]*bean.FluxApplicationListDto, 0)
	for _, cluster := range clusters {
		clusterObj := cluster
		if clusterObj.IsVirtualCluster || len(clusterObj.ErrorInConnecting) != 0 {
			continue
		}

		restConfig := &rest.Config{}
		clusterInfo := impl.converter.GetClusterInfo(cluster)
		clusterConfig := impl.converter.GetClusterConfig(clusterInfo)
		restConfig, err = impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
		if err != nil {
			impl.logger.Errorw("error in getting rest config ", "err", err, "clusterId", clusterObj.Id)
			return nil, err
		}

		kustomizationResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, bean.GvkForKustomizationFluxApp, bean.AllNamespaces, true, nil)
		if err != nil {
			if errStatus, ok := err.(*errors.StatusError); ok {
				if errStatus.Status().Code == 404 {
					// no flux kustomization apps found, not sending error
					impl.logger.Warnw("error in getting external flux kustomization  app list, no kustomization apps found", "err", err, "clusterId", clusterObj.Id)
					continue
				}
			}
			impl.logger.Errorw("error in getting resource list", "err", err)
			return nil, err
		}

		helmReleaseResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, bean.GvkForhelmreleaseFluxApp, bean.AllNamespaces, true, nil)
		if err != nil {
			if errStatus, ok := err.(*errors.StatusError); ok {
				if errStatus.Status().Code == 404 {
					// no flux apps found, not sending error
					impl.logger.Warnw("error in getting external argo app list, no apps found", "err", err, "clusterId", clusterObj.Id)
					continue
				}
			}
			impl.logger.Errorw("error in getting resource list", "err", err)
			return nil, err
		}
		kustomizationAppLists := getApplicationListDtos(kustomizationResp.Resources.Object, clusterObj.ClusterName, clusterObj.Id, "")
		helmReleaseAppLists := getApplicationListDtos(helmReleaseResp.Resources.Object, clusterObj.ClusterName, clusterObj.Id, "HelmRelease")
		appListFinal = append(appListFinal, kustomizationAppLists...)

		appListFinal = append(appListFinal, helmReleaseAppLists...)
	}
	return appListFinal, nil
}

func getApplicationListDtos(manifestObj map[string]interface{}, clusterName string, clusterId int, FluxAppType string) []*bean.FluxApplicationListDto {
	appLists := make([]*bean.FluxApplicationListDto, 0)
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
			appListDto := &bean.FluxApplicationListDto{
				ClusterId:   clusterId,
				ClusterName: clusterName,
			}
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
			}

			rowCells := rowDataMap[k8sCommonBean.K8sClusterResourceCellKey].([]interface{})
			for key, value := range keysToBeFetchedFromColumnDefinitions {
				resolvedValueFromRowCell := rowCells[value].(string)
				switch key {
				case k8sCommonBean.K8sResourceColumnDefinitionName:
					appListDto.Name = resolvedValueFromRowCell
				case "Status":
					appListDto.SyncStatus = resolvedValueFromRowCell
				case "Ready":
					appListDto.HealthStatus = resolvedValueFromRowCell
				}
			}
			for _, key := range keysToBeFetchedFromRawObject {
				switch key {
				case k8sCommonBean.K8sClusterResourceNamespaceKey:
					appListDto.Namespace = metadata[k8sCommonBean.K8sClusterResourceNamespaceKey].(string)
				}
			}
			appLists = append(appLists, appListDto)
		}
	}
	return appLists
}
