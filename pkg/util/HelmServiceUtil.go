package util

import (
	"encoding/binary"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/devtron-labs/common-lib/utils/k8s/health"
	"github.com/devtron-labs/kubelink/bean"
	"hash"
	"hash/fnv"
	"helm.sh/helm/v3/pkg/release"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GetAppId returns AppID by logic  cluster_id|namespace|release_name
func GetAppId(clusterId int32, release *release.Release) string {
	return fmt.Sprintf("%d|%s|%s", clusterId, release.Namespace, release.Name)
}

func GetMessageFromReleaseStatus(releaseStatus release.Status) string {
	switch releaseStatus {
	case release.StatusUnknown:
		return "The release is in an uncertain state"
	case release.StatusDeployed:
		return "The release has been pushed to Kubernetes"
	case release.StatusUninstalled:
		return "The release has been uninstalled from Kubernetes"
	case release.StatusSuperseded:
		return "The release object is outdated and a newer one exists"
	case release.StatusFailed:
		return "The release was not successfully deployed"
	case release.StatusUninstalling:
		return "The release uninstall operation is underway"
	case release.StatusPendingInstall:
		return "The release install operation is underway"
	case release.StatusPendingUpgrade:
		return "The release upgrade operation is underway"
	case release.StatusPendingRollback:
		return "The release rollback operation is underway"
	default:
		fmt.Println("un handled release status", releaseStatus)
	}

	return ""
}

// app health is worst of the nodes health
// or if app status is healthy then check for hibernation status
func BuildAppHealthStatus(nodes []*bean.ResourceNode) *bean.HealthStatusCode {
	appHealthStatus := bean.HealthStatusHealthy
	isAppFullyHibernated := true
	var isAppPartiallyHibernated bool
	var isAnyNodeCanByHibernated bool

	for _, node := range nodes {
		nodeHealth := node.Health
		if node.CanBeHibernated {
			isAnyNodeCanByHibernated = true
			if !node.IsHibernated {
				isAppFullyHibernated = false
			} else {
				isAppPartiallyHibernated = true
			}
		}
		if nodeHealth == nil {
			continue
		}
		if health.IsWorseStatus(health.HealthStatusCode(appHealthStatus), health.HealthStatusCode(nodeHealth.Status)) {
			appHealthStatus = nodeHealth.Status
		}
	}

	// override hibernate status on app level if status is healthy and hibernation done
	if appHealthStatus == bean.HealthStatusHealthy && isAnyNodeCanByHibernated {
		if isAppFullyHibernated {
			appHealthStatus = bean.HealthStatusHibernated
		} else if isAppPartiallyHibernated {
			appHealthStatus = bean.HealthStatusPartiallyHibernated
		}
	}

	return &appHealthStatus
}

func GetAppStatusOnBasisOfHealthyNonHealthy(healthStatusArray []*bean.HealthStatus) *bean.HealthStatusCode {
	appHealthStatus := bean.HealthStatusHealthy
	for _, node := range healthStatusArray {
		nodeHealth := node
		if nodeHealth == nil {
			continue
		}
		//if any node's health is worse than healthy then we break the loop and return
		if health.IsWorseStatus(health.HealthStatusCode(appHealthStatus), health.HealthStatusCode(nodeHealth.Status)) {
			appHealthStatus = nodeHealth.Status
			break
		}
	}
	return &appHealthStatus
}

// We omit vowels from the set of available characters to reduce the chances
// of "bad words" being formed.
const alphanums = "bcdfghjklmnpqrstvwxz2456789"

// SafeEncodeString encodes s using the same characters as rand.String. This reduces the chances of bad words and
// ensures that strings generated from hash functions appear consistent throughout the API.
func SafeEncodeString(s string) string {
	r := make([]byte, len(s))
	for i, b := range []rune(s) {
		r[i] = alphanums[(int(b) % len(alphanums))]
	}
	return string(r)
}

// DeepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func DeepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	_, err := printer.Fprintf(hasher, "%#v", objectToWrite)
	if err != nil {
		fmt.Println(err)
	}
}

func ComputePodHash(template *coreV1.PodTemplateSpec, collisionCount *int32) string {
	podTemplateSpecHasher := fnv.New32a()
	DeepHashObject(podTemplateSpecHasher, *template)

	// Add collisionCount in the hash if it exists.
	if collisionCount != nil {
		collisionCountBytes := make([]byte, 8)
		binary.LittleEndian.PutUint32(collisionCountBytes, uint32(*collisionCount))
		_, err := podTemplateSpecHasher.Write(collisionCountBytes)
		if err != nil {
			fmt.Println(err)
		}
	}
	return SafeEncodeString(fmt.Sprint(podTemplateSpecHasher.Sum32()))
}

func ConvertToV1Deployment(node *bean.ResourceNode) (*v1beta1.Deployment, error) {
	deploymentObj := v1beta1.Deployment{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(node.Manifest.UnstructuredContent(), &deploymentObj)
	if err != nil {
		return nil, err
	}
	return &deploymentObj, nil
}
func ConvertToV1ReplicaSet(node *bean.ResourceNode) (*v1beta1.ReplicaSet, error) {
	replicaSetObj := v1beta1.ReplicaSet{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(node.Manifest.UnstructuredContent(), &replicaSetObj)
	if err != nil {
		return nil, err
	}
	return &replicaSetObj, nil
}

func GetReplicaSetPodHash(replicasetObj *v1beta1.ReplicaSet, deploymentObj *v1beta1.Deployment) string {
	labels := make(map[string]string)
	for k, v := range replicasetObj.Spec.Template.Labels {
		if k != "pod-template-hash" {
			labels[k] = v
		}
	}
	replicasetObj.Spec.Template.Labels = labels
	podHash := ComputePodHash(&replicasetObj.Spec.Template, deploymentObj.Status.CollisionCount)
	return podHash
}

func GetRolloutPodTemplateHash(replicasetObj *v1beta1.ReplicaSet) string {
	if rolloutPodTemplateHash, ok := replicasetObj.Labels["rollouts-pod-template-hash"]; ok {
		return rolloutPodTemplateHash
	}
	return ""
}

func GetRolloutPodHash(rollout map[string]interface{}) string {
	if s, ok := rollout["status"]; ok {
		if sm, ok := s.(map[string]interface{}); ok {
			if cph, ok := sm["currentPodHash"]; ok {
				if cphs, ok := cph.(string); ok {
					return cphs
				}
			}
		}
	}
	return ""
}
