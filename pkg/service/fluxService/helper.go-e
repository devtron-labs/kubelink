package fluxService

import (
	"errors"
	"fmt"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"k8s.io/apimachinery/pkg/runtime/schema"
	//"github.com/devtron-labs/kubelink/pkg/service/fluxService"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// fetchFluxAppFields Extracting fields from the rowData and populating the FluxApplicationDto fields of flux app
func fetchFluxAppFields(rowDataMap map[string]interface{}, columnDefinitions map[string]int, clusterId int, clusterName string, FluxAppType FluxAppType) *FluxApplicationDto {

	if FluxAppType == FluxAppHelmreleaseKind && isKsChildHelmRelease(rowDataMap) {
		return nil
	}
	rowCells, found, err := unstructured.NestedSlice(rowDataMap, k8sCommonBean.K8sClusterResourceCellKey)
	if err != nil || !found {
		return nil
	}
	name, syncStatus, healthStatus := extractValuesFromRowCells(rowCells, columnDefinitions)

	namespace, found, err := unstructured.NestedString(rowDataMap, k8sCommonBean.K8sClusterResourceObjectKey, k8sCommonBean.K8sClusterResourceMetadataKey, NamespaceKey)
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

// fetchInventoryIfExists extracting out the inventory field from kustomization response data if it is present or not, returning the inventory if exists
func fetchInventoryIfExists(obj map[string]interface{}) (map[string]interface{}, bool) {
	inventory, found, err := unstructured.NestedMap(obj, STATUS, INVENTORY)
	if err != nil || !found {
		return inventory, false
	}
	return inventory, true
}

// fetchInventoryList extracting k8s resources details from inventory of flux appType Kustomization
func fetchInventoryList(inventory map[string]interface{}) ([]*FluxKsResourceDetail, error) {
	var inventoryResources []*FluxKsResourceDetail
	childResourcesMap, err := getInventoryObjectsMap(inventory)
	if err != nil {
		return nil, err
	}
	for childResourceId, version := range childResourcesMap {
		fluxResource, err := decodeObjMetadata(childResourceId, version)
		if err != nil {
			err = fmt.Errorf("unable to parse stored object metadata: %s", childResourceId)
			return nil, err
		}
		inventoryResources = append(inventoryResources, fluxResource)
	}
	return inventoryResources, nil
}

// getInventoryObjectsMap extracting Inventory field and getting a map of its objects
func getInventoryObjectsMap(inventory map[string]interface{}) (map[string]string, error) {
	var fluxManagedResourcesMap map[string]string

	entries, found, err := unstructured.NestedSlice(inventory, ENTRIES)
	if err != nil || !found {
		return nil, fmt.Errorf("entries in inventory not found %s", err)
	}

	fluxManagedResourcesMap, err = parseInventoryObjects(entries)
	if err != nil {
		return nil, err

	}
	return fluxManagedResourcesMap, nil
}

// getInventoryObjectsMapFromResponseObj extracting the map of resoruces and Ids in the Kustomization
func getInventoryObjectsMapFromResponseObj(obj map[string]interface{}) (map[string]string, error) {
	fluxManagedResourcesMap := make(map[string]string)
	entries, found, err := unstructured.NestedSlice(obj, STATUS, INVENTORY, ENTRIES)
	if err != nil || !found {
		return nil, fmt.Errorf("inventory entries not found")
	}
	fluxManagedResourcesMap, err = parseInventoryObjects(entries)
	if err != nil {
		return nil, err
	}
	return fluxManagedResourcesMap, nil
}

// getReleaseNameNamespace extracting releaseName and namespace From Flux Helm Release Crd Response
func getReleaseNameNamespace(obj map[string]interface{}, name string) (string, string, error) {

	var storageNamespace, releaseName string
	var err error
	storageNamespaceVal, err := getStorageNamespaceForFluxHelmRelease(obj)
	if err != nil {
		return releaseName, storageNamespace, errors.New("error in getting storage namespace")
	}
	storageNamespace = storageNamespaceVal

	releaseNameVal, err := getReleaseNameForFluxHelmRelease(obj, name)
	if err != nil {
		return releaseName, storageNamespace, errors.New("error in getting releaseName")
	}
	releaseName = releaseNameVal
	return releaseName, storageNamespace, nil

}

// getFluxAppStatus extracting the status, reason and message fields from condition field if exists
func getFluxAppStatus(obj map[string]interface{}, gvk schema.GroupVersionKind) (*FluxAppStatusDetail, error) {
	var reason, message string
	status := StatusMissing
	if statusRawObj, ok := obj[STATUS]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		conditions, found, err := unstructured.NestedSlice(statusObj, "conditions")
		if err != nil {
			return nil, err
		}

		/* In case of HelmRelease, Conditions are having negative polarity (means only available when the status is true)
		   In HelmRelease, conditions field exhibits the negative polarity, So it has been handled here by default values as Missing Status and customReason with custom message.
		*/
		if !found && gvk.Kind == FluxAppHelmreleaseKind {
			return &FluxAppStatusDetail{
				Status:  status,
				Reason:  Reason,
				Message: ErrMessageForHelmRelease,
			}, nil
		} else if !found || len(conditions) == 0 && gvk.Kind == FluxAppKustomizationKind {
			/* In case of Kustomization, Conditions are only present if that crd has been applied (means only available when it has been applied and its conditions field available)
			   It handles the case when the conditions field are not available due to some unknown error but the object has been received.
			*/
			return &FluxAppStatusDetail{
				Status:  status,
				Reason:  Reason,
				Message: ErrMessageForKustomization,
			}, nil
		}

		lastIndex := len(conditions)
		itemRawObj := conditions[lastIndex-1]
		itemObj := itemRawObj.(map[string]interface{})
		if statusValRaw, ok := itemObj["status"]; ok {
			status = statusValRaw.(string)
		}
		if reasonRawVal, ok := itemObj["reason"]; ok {
			reason = reasonRawVal.(string)
		}
		if messageRaw, ok := itemObj["message"]; ok {
			message = messageRaw.(string)
		}
	}

	return &FluxAppStatusDetail{
		Status:  status,
		Reason:  reason,
		Message: message,
	}, nil
}
