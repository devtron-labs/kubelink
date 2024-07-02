package FluxService

import (
	"fmt"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"strings"

	//"github.com/devtron-labs/kubelink/pkg/service/FluxService"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func extractValuesFromRowCells(rowCells []interface{}, columnDefinitions map[string]int) (string, string, string) {
	getValue := func(index int) string {
		if index < len(rowCells) {
			if value, ok := rowCells[index].(string); ok && value != "" {
				return value
			}
		}
		return "Unknown"
	}
	name := getValue(columnDefinitions[NameKey])
	syncStatus := getValue(columnDefinitions[StatusKey])
	healthStatus := getValue(columnDefinitions[ReadyKey])

	return name, syncStatus, healthStatus
}
func shouldProcessApp(fluxAppType FluxAppType, metadata map[string]interface{}) bool {
	if fluxAppType == FluxAppHelmreleaseKind {
		labels, found, err := unstructured.NestedMap(metadata, FluxLabel)
		if err != nil || !found {
			return true
		}
		nameLabel, _, errName := unstructured.NestedString(labels, KustomizeNameLabel)
		namespaceLabel, _, errNamespace := unstructured.NestedString(labels, KustomizeNamespaceLabel)
		if errName != nil || errNamespace != nil || nameLabel == "" || namespaceLabel == "" {
			return true
		}
		return false
	}
	return true
}
func createFluxApplicationDto(rowDataMap map[string]interface{}, columnDefinitions map[string]int, clusterId int, clusterName string, FluxAppType FluxAppType) *FluxApplicationDto {
	rowObject, _, err := unstructured.NestedMap(rowDataMap, k8sCommonBean.K8sClusterResourceObjectKey)
	if err != nil {
		return nil
	}
	metadata, _, err := unstructured.NestedMap(rowObject, k8sCommonBean.K8sClusterResourceMetadataKey)
	if err != nil {
		return nil
	}
	if !shouldProcessApp(FluxAppType, metadata) {
		return nil
	}
	rowCells, _, err := unstructured.NestedSlice(rowDataMap, k8sCommonBean.K8sClusterResourceCellKey)
	if err != nil {
		return nil
	}
	name, syncStatus, healthStatus := extractValuesFromRowCells(rowCells, columnDefinitions)

	namespace, found, err := unstructured.NestedString(metadata, "namespace") // Replace "namespace" with actual key
	if err != nil || !found {
		return nil
	}
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

	columnDefinitions, found, err := unstructured.NestedSlice(manifestObj, k8sCommonBean.K8sClusterResourceColumnDefinitionKey)
	if err != nil || !found {
		return fluxAppDetailArray
	}

	columnDefinitionMap := extractColumnDefinitions(columnDefinitions)

	rowsData, found, err := unstructured.NestedSlice(manifestObj, k8sCommonBean.K8sClusterResourceRowsKey)
	if err != nil || !found {
		return fluxAppDetailArray
	}

	for _, rowData := range rowsData {
		rowDataMap, ok := rowData.(map[string]interface{})
		if !ok {
			continue
		}
		appDto := createFluxApplicationDto(rowDataMap, columnDefinitionMap, clusterId, clusterName, FluxAppType)
		if appDto != nil {
			fluxAppDetailArray = append(fluxAppDetailArray, appDto)
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
func getFluxSpecKubeConfig(obj map[string]interface{}) (bool, error) {
	spec, found, err := unstructured.NestedMap(obj, "spec")
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	_, found, err = unstructured.NestedFieldCopy(spec, "kubeConfig")
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return true, nil
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
func inventoryExists(obj map[string]interface{}) bool {
	statusObj, found, err := unstructured.NestedMap(obj, STATUS)
	if err != nil || !found {
		return false
	}
	_, found, err = unstructured.NestedMap(statusObj, INVENTORY)
	if err != nil || !found {
		return false
	}
	return true
}
func fetchInventoryList(obj map[string]interface{}) ([]FluxKsResourceDetail, error) {
	var inventoryResources []FluxKsResourceDetail
	childResourcesMap, err := getInventoryMap(obj)
	if err != nil {
		return nil, err
	}
	for childResourceId, version := range childResourcesMap {
		fluxResource, err := parseObjMetadata(childResourceId)
		if err != nil {
			err = fmt.Errorf("unable to parse stored object metadata: %s", childResourceId)
			return nil, err
		}
		fluxResource.Version = version
		inventoryResources = append(inventoryResources, fluxResource)
	}
	return inventoryResources, nil
}
func convertFluxAppDetailsToDtos(appDetails []*FluxApplicationDto) []*client.FluxApplication {
	var appListFinalDto []*client.FluxApplication
	for _, appDetail := range appDetails {
		fluxAppDetailDto := GetFluxAppDetailDto(appDetail)
		appListFinalDto = append(appListFinalDto, fluxAppDetailDto)
	}
	return appListFinalDto
}
func getInventoryMap(obj map[string]interface{}) (map[string]string, error) {
	fluxManagedResourcesMap := make(map[string]string)

	status, found, err := unstructured.NestedMap(obj, STATUS)
	if err != nil || !found {
		return nil, fmt.Errorf("status not found")
	}

	inventory, found, err := unstructured.NestedMap(status, INVENTORY)
	if err != nil || !found {
		return nil, fmt.Errorf("inventory not found")
	}

	entries, found, err := unstructured.NestedSlice(inventory, ENTRIES)
	if err != nil || !found {
		return nil, fmt.Errorf("entries not found")
	}

	for _, item := range entries {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid item format")
		}

		metadataCompact := ObjectMetadataCompact{}
		if id, found, err := unstructured.NestedString(itemMap, ID); found && err == nil {
			metadataCompact.Id = id
		} else if err != nil {
			return nil, err
		}

		if version, found, err := unstructured.NestedString(itemMap, VERSION); found && err == nil {
			metadataCompact.Version = version
		} else if err != nil {
			return nil, err
		}
		fluxManagedResourcesMap[metadataCompact.Id] = metadataCompact.Version
	}
	return fluxManagedResourcesMap, nil
}
func getReleaseNameNamespace(obj map[string]interface{}, name string) (string, string, error) {
	//storageNamespace is optional, if not found, then its value byb default is the same as the namespace of helmRelease
	var storageNamespace string

	if statusRawObj, ok := obj[STATUS]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		if storageNamespaceRaw, ok6 := statusObj["storageNamespace"]; ok6 {
			storageNamespace = storageNamespaceRaw.(string)
		} else {
			metadata, found, err := unstructured.NestedMap(obj, "metadata")
			if err != nil {
				return "", "", err
			}
			if !found {
				return "", "", fmt.Errorf("entries not found")
			}
			namespaceRaw, found, err := unstructured.NestedFieldNoCopy(metadata, "namespace")
			if err != nil {
				return "", "", err
			}
			if !found {
				return "", "", fmt.Errorf("entries not found")
			}
			storageNamespace = namespaceRaw.(string)
		}
	}

	//releaseName is also optional, but if not provided then releaseName is decided by combination of "<targetNamespace>-name" and targetNamespace is optional,in absence of optional targetNamespace, releaseName will be same as HelmRelease.

	spec, found, err := unstructured.NestedMap(obj, "spec")
	if err != nil {
		return "", "", err
	}
	if !found {
		return "", "", fmt.Errorf("entries not found")
	}

	releaseNameRaw, found, err := unstructured.NestedFieldCopy(spec, "releaseName")
	if err != nil {
		return "", "", err
	}
	if found {
		return releaseNameRaw.(string), storageNamespace, nil
	}

	targetNamespaceRaw, found, err := unstructured.NestedFieldNoCopy(spec, "targetNamespace")
	if err != nil {
		return "", "", err
	}
	if found {
		return targetNamespaceRaw.(string) + "-" + name, storageNamespace, nil
	}

	return name, storageNamespace, nil

}
func getKsAppStatus(obj map[string]interface{}, gvk schema.GroupVersionKind) (*FluxAppStatusDetail, error) {
	var status, reason, message string
	if statusRawObj, ok := obj[STATUS]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		conditions, found, err := unstructured.NestedSlice(statusObj, "conditions")
		if err != nil {
			return nil, err
		}

		// In case of HelmRelease, Conditions are having negative polarity (means only available when the status is true)
		if !found && gvk.Kind == FluxAppHelmreleaseKind {
			status = "Not true"
			return &FluxAppStatusDetail{
				Status:  status,
				Reason:  "StatusNotReady",
				Message: "Status is not true for this helmRelease",
			}, nil
		} else if !found {
			return nil, fmt.Errorf("conditions not found for the requested  flux app")
		}

		lastIndex := len(conditions)
		itemRawObj := conditions[lastIndex-1]
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
	}

	return &FluxAppStatusDetail{
		Status:  status,
		Reason:  reason,
		Message: message,
	}, nil
}
func extractColumnDefinitions(columnsDataRaw interface{}) map[string]int {
	columnDefinitions := make(map[string]int)
	columns, found, err := unstructured.NestedSlice(map[string]interface{}{"data": columnsDataRaw}, "data")
	if err != nil || !found {
		return columnDefinitions
	}
	for index, column := range columns {
		columnMap, ok := column.(map[string]interface{})
		if !ok {
			continue
		}
		name, found, err := unstructured.NestedString(columnMap, ColumnNameKey)
		if err != nil || !found {
			continue
		}
		columnDefinitions[name] = index
	}
	return columnDefinitions
}
