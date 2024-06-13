package FluxApplicationService

import (
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func getRowsData(manifestObj map[string]interface{}, key string) []interface{} {
	if rowsDataRaw, exists := manifestObj[key]; exists {
		return rowsDataRaw.([]interface{})
	}
	return nil
}

func extractColumnDefinitions(columnsDataRaw interface{}) map[string]int {
	columnDefinitions := make(map[string]int)
	if columns, ok := columnsDataRaw.([]interface{}); ok {
		for index, column := range columns {
			if columnMap, ok := column.(map[string]interface{}); ok {
				if name, exists := columnMap[ColumnNameKey].(string); exists {
					columnDefinitions[name] = index
				}
			}
		}
	}
	return columnDefinitions
}

func extractValuesFromRowCells(rowCells []interface{}, columnDefinitions map[string]int) (string, string, string) {
	var name, syncStatus, healthStatus string
	for key, index := range columnDefinitions {
		resolvedValueFromRowCell := rowCells[index].(string)
		switch key {
		case NameKey:
			name = resolvedValueFromRowCell
		case StatusKey:
			syncStatus = resolvedValueFromRowCell
		case ReadyKey:
			healthStatus = resolvedValueFromRowCell
		}
	}
	return name, syncStatus, healthStatus
}

func extractNamespace(metadata map[string]interface{}) string {
	return metadata[k8sCommonBean.K8sClusterResourceNamespaceKey].(string)
}

func shouldProcessApp(FluxAppType FluxAppType, metadata map[string]interface{}) bool {
	if FluxAppType == FluxAppHelmreleaseKind {
		if labels, exists := metadata[FluxLabel].(map[string]interface{}); exists {
			nameLabel, nameExists := labels[KustomizeNameLabel].(string)
			namespaceLabel, namespaceExists := labels[KustomizeNamespaceLabel].(string)
			if nameExists && nameLabel != "" && namespaceExists && namespaceLabel != "" {
				return false
			}
		}
	}
	return true
}
func createFluxApplicationDto(rowDataMap map[string]interface{}, columnDefinitions map[string]int, clusterId int, clusterName string, FluxAppType FluxAppType) *FluxApplicationDto {
	rowObject := rowDataMap[k8sCommonBean.K8sClusterResourceObjectKey].(map[string]interface{})
	metadata := rowObject[k8sCommonBean.K8sClusterResourceMetadataKey].(map[string]interface{})

	if !shouldProcessApp(FluxAppType, metadata) {
		return nil
	}
	rowCells := rowDataMap[k8sCommonBean.K8sClusterResourceCellKey].([]interface{})
	name, syncStatus, healthStatus := extractValuesFromRowCells(rowCells, columnDefinitions)

	namespace := extractNamespace(metadata)

	return &FluxApplicationDto{
		Name:         name,
		HealthStatus: healthStatus,
		SyncStatus:   syncStatus,
		EnvironmentDetails: &EnvironmentDetail{
			ClusterId:   clusterId,
			ClusterName: clusterName,
			Namespace:   namespace,
		},
		AppType: string(FluxAppType),
	}
}

func getApplicationListDtos(resources unstructured.UnstructuredList, clusterName string, clusterId int, FluxAppType FluxAppType) []*FluxApplicationDto {
	manifestObj := resources.Object
	fluxAppDetailArray := make([]*FluxApplicationDto, 0)
	columnDefinitions := extractColumnDefinitions(manifestObj[k8sCommonBean.K8sClusterResourceColumnDefinitionKey])

	rowsData := getRowsData(manifestObj, k8sCommonBean.K8sClusterResourceRowsKey)
	if rowsData != nil {
		for _, rowData := range rowsData {
			rowDataMap := rowData.(map[string]interface{})
			appDto := createFluxApplicationDto(rowDataMap, columnDefinitions, clusterId, clusterName, FluxAppType)
			if appDto != nil {
				fluxAppDetailArray = append(fluxAppDetailArray, appDto)
			}
		}
	}

	return fluxAppDetailArray
}

func getFluxAppDetailDto(appDetail *FluxApplicationDto) *client.FluxApplication {
	return &client.FluxApplication{
		Name:         appDetail.Name,
		HealthStatus: appDetail.HealthStatus,
		SyncStatus:   appDetail.SyncStatus,
		AppType:      appDetail.AppType,
		EnvironmentDetail: &client.EnvironmentDetails{
			ClusterName: appDetail.EnvironmentDetails.ClusterName,
			ClusterId:   int32(appDetail.EnvironmentDetails.ClusterId),
			Namespace:   appDetail.EnvironmentDetails.Namespace,
		},
	}
}
