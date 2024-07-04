package fluxService

import (
	"fmt"
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"strings"
)

// extractColumnDefinitions extracting the column Definitions from the column definition
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

// extractValuesFromRowCells extracting fields from the list of fluxAppList rowCells based on the columnDefinitions fields,
func extractValuesFromRowCells(rowCells []interface{}, columnDefinitions map[string]int) (string, string, string) {
	getValue := func(index int) string {
		valueAtIndex := "Unknown"
		if index < len(rowCells) {
			if value, ok := rowCells[index].(string); ok && value != "" {
				valueAtIndex = value
				return valueAtIndex
			}
		}
		return valueAtIndex
	}
	name := getValue(columnDefinitions[AppNameKey])
	syncStatus := getValue(columnDefinitions[StatusKey])
	healthStatus := getValue(columnDefinitions[ReadyKey])
	return name, syncStatus, healthStatus
}

// isKsChildHelmRelease checking for the helmRelease Dependency on any kustomization or not
func isKsChildHelmRelease(rowDataMap map[string]interface{}) bool {

	nameLabel, _, errName := unstructured.NestedString(rowDataMap, ObjectField, MetaDataField, FluxLabel, KustomizeNameLabel)
	namespaceLabel, _, errNamespace := unstructured.NestedString(rowDataMap, ObjectField, MetaDataField, FluxLabel, KustomizeNamespaceLabel)
	if errName != nil || errNamespace != nil || nameLabel == "" || namespaceLabel == "" {
		return false
	}
	return true
}

// getReleaseNameForFluxHelmRelease extracting the releaseName from object response  for flux HelmRelease type
func getReleaseNameForFluxHelmRelease(obj map[string]interface{}, name string) (string, error) {
	releaseNameByDefault := name
	var releaseName string
	//releaseName is also optional, but if not provided then releaseName is decided by combination of "<targetNamespace>-name" and targetNamespace is optional,in absence of optional targetNamespace, releaseName will be same as HelmRelease.

	releaseNameRaw, found, err := unstructured.NestedString(obj, SpecField, ReleaseNameKey)
	if err != nil {
		return releaseName, err
	} else if found {
		releaseName = releaseNameRaw
		return releaseName, nil
	}

	targetNamespace, found, err := unstructured.NestedString(obj, SpecField, TargetNamespaceKey)
	if err != nil {
		return releaseName, err
	} else if found {
		releaseName = targetNamespace + "-" + releaseNameByDefault
	} else {
		releaseName = releaseNameByDefault
	}
	return releaseName, nil
}

// getStorageNamespaceForFluxHelmRelease extracting the release Storage Namespace from object response for flux HelmRelease type
func getStorageNamespaceForFluxHelmRelease(obj map[string]interface{}) (string, error) {
	//storageNamespace is optional, if not found, then its value byb default is the same as the namespace of helmRelease
	var storageNamespace string

	storageNamespaceDefault, found, err := unstructured.NestedString(obj, MetaDataField, NamespaceKey)
	if err != nil {
		return storageNamespace, err
	} else if found {
		storageNamespace = storageNamespaceDefault
	}

	storageNamespaceKey, found, err := unstructured.NestedString(obj, STATUS, StorageNamespaceKey)
	if err != nil {
		return storageNamespace, err
	} else if found {
		storageNamespace = storageNamespaceKey
	}
	return storageNamespace, nil
}

/*
this part of code logic belongs to flux cd for parsing their gvk in a kustomization inventory
don't change the internal implementation of this parseObjMetadata function, before changing look into its implementation in fluxcd
https://github.com/fluxcd/cli-utils/blob/7fd1e873041120a71a4c0e9d0df4fe00cd441b40/pkg/object/objmetadata.go#L70
*/
func parseObjMetadata(s string) (*ObjMetadata, error) {
	index := strings.Index(s, FieldSeparator)
	if index == -1 {
		return nil, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	namespace := s[:index]
	s = s[index+1:]
	// Next, parse last field kind
	index = strings.LastIndex(s, FieldSeparator)
	if index == -1 {
		return nil, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	kind := s[index+1:]
	s = s[:index]
	// Next, parse next to last field group
	index = strings.LastIndex(s, FieldSeparator)
	if index == -1 {
		return nil, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	group := s[index+1:]
	// Finally, second field name. Name may contain colon transcoded as double underscore.
	name := s[:index]
	name = strings.ReplaceAll(name, ColonTranscoded, ":")
	// Check that there are no extra fields by search for fieldSeparator.
	if strings.Contains(name, FieldSeparator) {
		return nil, fmt.Errorf("too many fields within: %s", s)
	}

	return &ObjMetadata{
		Name:      name,
		Namespace: namespace,
		Group:     group,
		Kind:      kind,
	}, nil
}

// GetFluxAppDetailDto converting the AppDetail in Grpc format
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

// decodeObjMetadata gives the gvk name and namespace after decoding the string from inventory
func decodeObjMetadata(s, version string) (*FluxKsResourceDetail, error) {
	metadata, err := parseObjMetadata(s)
	if err != nil {
		return nil, err
	}
	// Create the ObjMetadata object from the four parsed fields.
	id := &FluxKsResourceDetail{
		Namespace: metadata.Namespace,
		Name:      metadata.Name,
		Group:     metadata.Group,
		Kind:      metadata.Kind,
		Version:   version,
	}
	return id, nil
}

// parseInventoryObjects parsing the entries into map
func parseInventoryObjects(entries []interface{}) (map[string]string, error) {
	fluxManagedResourcesMap := make(map[string]string)
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

// convertFluxAppDetailsToDtos converting the appList array into appDetailsDto for grpc
func convertFluxAppDetailsToDtos(appDetails []*FluxApplicationDto) []*client.FluxApplication {
	var appListFinalDto []*client.FluxApplication
	for _, appDetail := range appDetails {
		fluxAppDetailDto := GetFluxAppDetailDto(appDetail)
		appListFinalDto = append(appListFinalDto, fluxAppDetailDto)
	}
	return appListFinalDto
}
