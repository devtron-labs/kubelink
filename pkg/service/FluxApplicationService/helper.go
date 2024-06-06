package FluxApplicationService

import (
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	client "github.com/devtron-labs/kubelink/grpc"
)

func getApplicationListDtos(manifestObj map[string]interface{}, clusterName string, clusterId int, FluxAppType FluxAppType) []*FluxApplicationDto {
	fluxAppDetailArray := make([]*FluxApplicationDto, 0)
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

			appDto := &FluxApplicationDto{
				Name:         name,
				HealthStatus: healthStatus,
				SyncStatus:   syncStatus,
				EnvironmentDetails: &EnvironmentDetail{
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

func getFluxAppDetailDto(appDetail *FluxApplicationDto) *client.FluxApplicationDetail {
	return &client.FluxApplicationDetail{
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
}
