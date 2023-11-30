package cache

import (
	"errors"
	"fmt"
	clustercache "github.com/argoproj/gitops-engine/pkg/cache"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/common-lib/utils/k8s/health"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/pkg/util"
	"github.com/devtron-labs/kubelink/pkg/util/argo"
	"golang.org/x/sync/semaphore"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// GitOps engine cluster cache tuning options
var (
	// clusterCacheResyncDuration controls the duration of cluster cache refresh.
	// NOTE: this differs from gitops-engine default of 24h
	clusterCacheResyncDuration = 12 * time.Hour

	// clusterCacheWatchResyncDuration controls the maximum duration that group/kind watches are allowed to run
	// for before relisting & restarting the watch
	clusterCacheWatchResyncDuration = 10 * time.Minute

	// clusterSyncRetryTimeoutDuration controls the sync retry duration when cluster sync error happens
	clusterSyncRetryTimeoutDuration = 10 * time.Second

	// The default limit of 50 is chosen based on experiments.
	clusterCacheListSemaphoreSize int64 = 50

	// clusterCacheListPageSize is the page size when performing K8s list requests.
	// 500 is equal to kubectl's size
	clusterCacheListPageSize int64 = 500

	// clusterCacheListPageBufferSize is the number of pages to buffer when performing K8s list requests
	clusterCacheListPageBufferSize int32 = 1

	// clusterCacheRetryLimit sets a retry limit for failed requests during cluster cache sync
	// If set to 1, retries are disabled.
	clusterCacheAttemptLimit int32 = 1

	// clusterCacheRetryUseBackoff specifies whether to use a backoff strategy on cluster cache sync, if retry is enabled
	clusterCacheRetryUseBackoff bool = false
)

const (
	hibernateReplicaAnnotation = "hibernator.devtron.ai/replicas"
)

// isRetryableError is a helper method to see whether an error
// returned from the dynamic client is potentially retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	return kerrors.IsInternalError(err) ||
		kerrors.IsInvalid(err) ||
		kerrors.IsTooManyRequests(err) ||
		kerrors.IsServerTimeout(err) ||
		kerrors.IsServiceUnavailable(err) ||
		kerrors.IsTimeout(err) ||
		kerrors.IsUnexpectedObjectError(err) ||
		kerrors.IsUnexpectedServerError(err) ||
		isResourceQuotaConflictErr(err) ||
		isTransientNetworkErr(err) ||
		isExceededQuotaErr(err) ||
		errors.Is(err, syscall.ECONNRESET)
}

func isExceededQuotaErr(err error) bool {
	return kerrors.IsForbidden(err) && strings.Contains(err.Error(), "exceeded quota")
}

func isResourceQuotaConflictErr(err error) bool {
	return kerrors.IsConflict(err) && strings.Contains(err.Error(), "Operation cannot be fulfilled on resourcequota")
}

func isTransientNetworkErr(err error) bool {
	var error net.Error
	switch {
	case errors.As(err, &error):
		var DNSError *net.DNSError
		var opError *net.OpError
		var unknownNetworkError net.UnknownNetworkError
		var error *url.Error
		switch {
		case errors.As(err, &DNSError), errors.As(err, &opError), errors.As(err, &unknownNetworkError):
			return true
		case errors.As(err, &error):
			// For a URL error, where it replies "connection closed"
			// retry again.
			return strings.Contains(err.Error(), "Connection closed by foreign host")
		}
	}

	errorString := err.Error()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		errorString = fmt.Sprintf("%s %s", errorString, exitErr.Stderr)
	}
	if strings.Contains(errorString, "net/http: TLS handshake timeout") ||
		strings.Contains(errorString, "i/o timeout") ||
		strings.Contains(errorString, "connection timed out") ||
		strings.Contains(errorString, "connection reset by peer") {
		return true
	}
	return false
}

func getClusterCacheOptions(clusterCacheConfig *ClusterCacheConfig) []clustercache.UpdateSettingsFunc {
	clusterCacheOpts := []clustercache.UpdateSettingsFunc{
		clustercache.SetListSemaphore(semaphore.NewWeighted(clusterCacheConfig.ClusterCacheListSemaphoreSize)),
		clustercache.SetListPageSize(clusterCacheConfig.ClusterCacheListPageSize),
		clustercache.SetListPageBufferSize(clusterCacheListPageBufferSize),
		clustercache.SetWatchResyncTimeout(clusterCacheWatchResyncDuration),
		clustercache.SetClusterSyncRetryTimeout(clusterSyncRetryTimeoutDuration),
		clustercache.SetResyncTimeout(clusterCacheResyncDuration),
		clustercache.SetRetryOptions(clusterCacheAttemptLimit, clusterCacheRetryUseBackoff, isRetryableError),
		clustercache.SetPopulateResourceInfoHandler(func(un *unstructured.Unstructured, isRoot bool) (interface{}, bool) {
			gvk := un.GroupVersionKind()
			res := getResourceNodeFromManifest(un, gvk)
			setHealthStatusForNode(res, un, gvk)

			if k8sUtils.IsPod(gvk) {
				infoItems, _ := argo.PopulatePodInfo(un)
				res.Info = infoItems
			}
			setHibernationRules(res, un)
			return res, false
		}),
	}
	return clusterCacheOpts
}

func getResourceNodeFromManifest(un *unstructured.Unstructured, gvk schema.GroupVersionKind) *bean.ResourceNode {
	resourceNode := &bean.ResourceNode{
		Port:            util.GetPorts(un, gvk),
		ResourceVersion: un.GetResourceVersion(),
		NetworkingInfo: &bean.ResourceNetworkingInfo{
			Labels: un.GetLabels(),
		},
		CreatedAt: un.GetCreationTimestamp().String(),
		ResourceRef: &bean.ResourceRef{
			Group:     gvk.Group,
			Version:   gvk.Version,
			Kind:      gvk.Kind,
			Namespace: un.GetNamespace(),
			Name:      un.GetName(),
			UID:       string(un.GetUID()),
		},
	}
	util.AddSelectiveInfoInResourceNode(resourceNode, gvk, un.UnstructuredContent())

	return resourceNode
}

func setHealthStatusForNode(res *bean.ResourceNode, un *unstructured.Unstructured, gvk schema.GroupVersionKind) {
	if k8sUtils.IsService(gvk) && un.GetName() == k8sUtils.DEVTRON_SERVICE_NAME && k8sUtils.IsDevtronApp(res.NetworkingInfo.Labels) {
		res.Health = &bean.HealthStatus{
			Status: bean.HealthStatusHealthy,
		}
	} else {
		if healthCheck := health.GetHealthCheckFunc(gvk); healthCheck != nil {
			health, err := healthCheck(un)
			if err != nil {
				res.Health = &bean.HealthStatus{
					Status:  bean.HealthStatusUnknown,
					Message: err.Error(),
				}
			} else if health != nil {
				res.Health = &bean.HealthStatus{
					Status:  string(health.Status),
					Message: health.Message,
				}
			}
		}
	}
}
func setHibernationRules(res *bean.ResourceNode, un *unstructured.Unstructured) {
	if un.GetOwnerReferences() == nil {
		// set CanBeHibernated
		replicas, found, _ := unstructured.NestedInt64(un.UnstructuredContent(), "spec", "replicas")
		if found {
			res.CanBeHibernated = true
		}

		// set IsHibernated
		annotations := un.GetAnnotations()
		if annotations != nil {
			if val, ok := annotations[hibernateReplicaAnnotation]; ok {
				if val != "0" && replicas == 0 {
					res.IsHibernated = true
				}
			}
		}
	}
}

func isInClusterIdList(clusterId int, clusterIdList []int) bool {
	for _, id := range clusterIdList {
		if id == clusterId {
			return true
		}
	}
	return false
}
