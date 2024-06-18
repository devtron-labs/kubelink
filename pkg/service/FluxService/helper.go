package FluxService

import (
	"fmt"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	client "github.com/devtron-labs/kubelink/grpc"
	"strings"

	//"github.com/devtron-labs/kubelink/pkg/service/FluxService"
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
		FluxAppDeploymentType: FluxAppType,
	}
}

func GetApplicationListDtos(resources unstructured.UnstructuredList, clusterName string, clusterId int, FluxAppType FluxAppType) []*FluxApplicationDto {
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

func GetFluxAppDetailDto(appDetail *FluxApplicationDto) *client.FluxApplication {
	return &client.FluxApplication{
		Name:                  appDetail.Name,
		HealthStatus:          appDetail.HealthStatus,
		SyncStatus:            appDetail.SyncStatus,
		FluxAppDeploymentType: string(appDetail.FluxAppDeploymentType),
		EnvironmentDetail: &client.EnvironmentDetails{
			ClusterName: appDetail.EnvironmentDetails.ClusterName,
			ClusterId:   int32(appDetail.EnvironmentDetails.ClusterId),
			Namespace:   appDetail.EnvironmentDetails.Namespace,
		},
	}
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