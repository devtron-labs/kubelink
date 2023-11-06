package clusterCache

import (
	"errors"
	"fmt"
	clustercache "github.com/argoproj/gitops-engine/pkg/cache"
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	"github.com/containerd/containerd/log"
	"github.com/devtron-labs/kubelink/bean"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
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

type liveStateCache struct {
	//clusterId to clusterCache mapping
	clustersCache map[int]clustercache.ClusterCache
}

func (l *liveStateCache) getCluster(clusterInfo bean.ClusterInfo, impl *ClusterCacheImpl) (clustercache.ClusterCache, error) {
	var cache clustercache.ClusterCache
	var ok bool
	cache, ok = l.clustersCache[clusterInfo.ClusterId]
	if ok {
		return cache, nil
	}
	clusterConfig := clusterInfo.GetClusterConfig()
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting rest config", "err", err, "clusterName", clusterConfig.ClusterName)
		return cache, err
	}
	cache = clustercache.NewClusterCache(restConfig, getClusterCacheOptions()...)
	// this part copied from argo cd need to be refactored
	_ = cache.OnResourceUpdated(func(newRes *clustercache.Resource, oldRes *clustercache.Resource, namespaceResources map[kube.ResourceKey]*clustercache.Resource) {
		toNotify := make(map[string]bool)
		var ref v1.ObjectReference
		if newRes != nil {
			ref = newRes.Ref
		} else {
			ref = oldRes.Ref
		}

		if cacheSettings.ignoreResourceUpdatesEnabled && oldRes != nil && newRes != nil && skipResourceUpdate(resInfo(oldRes), resInfo(newRes)) {
			// Additional check for debug level so we don't need to evaluate the
			// format string in case of non-debug scenarios
			if log.GetLevel() >= log.DebugLevel {
				namespace := ref.Namespace
				if ref.Namespace == "" {
					namespace = "(cluster-scoped)"
				}
				log.WithFields(log.Fields{
					"server":      cluster.Server,
					"namespace":   namespace,
					"name":        ref.Name,
					"api-version": ref.APIVersion,
					"kind":        ref.Kind,
				}).Debug("Ignoring change of object because none of the watched resource fields have changed")
			}
			return
		}

		for _, r := range []*clustercache.Resource{newRes, oldRes} {
			if r == nil {
				continue
			}
			app := getApp(r, namespaceResources)
			if app == "" || skipAppRequeuing(r.ResourceKey()) {
				continue
			}
			toNotify[app] = isRootAppNode(r) || toNotify[app]
		}
		l.onObjectUpdated(toNotify, ref)
	})

	l.clustersCache[clusterInfo.ClusterId] = cache
	return cache, nil
}

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

//
//func (c *liveStateCache) getCluster(server string) (clustercache.ClusterCache, error) {
//	c.lock.RLock()
//	clusterCache, ok := c.clusters[server]
//	cacheSettings := c.cacheSettings
//	c.lock.RUnlock()
//
//	if ok {
//		return clusterCache, nil
//	}
//
//	c.lock.Lock()
//	defer c.lock.Unlock()
//
//	clusterCache, ok = c.clusters[server]
//	if ok {
//		return clusterCache, nil
//	}
//
//	cluster, err := c.db.GetCluster(context.Background(), server)
//	if err != nil {
//		return nil, fmt.Errorf("error getting cluster: %w", err)
//	}
//
//	if !c.canHandleCluster(cluster) {
//		return nil, fmt.Errorf("controller is configured to ignore cluster %s", cluster.Server)
//	}
//
//	resourceCustomLabels, err := c.settingsMgr.GetResourceCustomLabels()
//	if err != nil {
//		return nil, fmt.Errorf("error getting custom label: %w", err)
//	}
//
//	respectRBAC, err := c.settingsMgr.RespectRBAC()
//	if err != nil {
//		return nil, fmt.Errorf("error getting value for %v: %w", settings.RespectRBAC, err)
//	}
//
//	clusterCacheConfig := cluster.RESTConfig()
//	// Controller dynamically fetches all resource types available on the cluster
//	// using a discovery API that may contain deprecated APIs.
//	// This causes log flooding when managing a large number of clusters.
//	// https://github.com/argoproj/argo-cd/issues/11973
//	// However, we can safely suppress deprecation warnings
//	// because we do not rely on resources with a particular API group or version.
//	// https://kubernetes.io/blog/2020/09/03/warnings/#customize-client-handling
//	//
//	// Completely suppress warning logs only for log levels that are less than Debug.
//	if log.GetLevel() < log.DebugLevel {
//		clusterCacheConfig.WarningHandler = rest.NoWarnings{}
//	}
//
//	clusterCacheOpts := []clustercache.UpdateSettingsFunc{
//		clustercache.SetListSemaphore(semaphore.NewWeighted(clusterCacheListSemaphoreSize)),
//		clustercache.SetListPageSize(clusterCacheListPageSize),
//		clustercache.SetListPageBufferSize(clusterCacheListPageBufferSize),
//		clustercache.SetWatchResyncTimeout(clusterCacheWatchResyncDuration),
//		clustercache.SetClusterSyncRetryTimeout(clusterSyncRetryTimeoutDuration),
//		clustercache.SetResyncTimeout(clusterCacheResyncDuration),
//		clustercache.SetSettings(cacheSettings.clusterSettings),
//		clustercache.SetNamespaces(cluster.Namespaces),
//		clustercache.SetClusterResources(cluster.ClusterResources),
//		clustercache.SetPopulateResourceInfoHandler(func(un *unstructured.Unstructured, isRoot bool) (interface{}, bool) {
//			res := &ResourceInfo{}
//			populateNodeInfo(un, res, resourceCustomLabels)
//			c.lock.RLock()
//			cacheSettings := c.cacheSettings
//			c.lock.RUnlock()
//
//			res.Health, _ = health.GetResourceHealth(un, cacheSettings.clusterSettings.ResourceHealthOverride)
//
//			appName := c.resourceTracking.GetAppName(un, cacheSettings.appInstanceLabelKey, cacheSettings.trackingMethod)
//			if isRoot && appName != "" {
//				res.AppName = appName
//			}
//
//			gvk := un.GroupVersionKind()
//
//			if cacheSettings.ignoreResourceUpdatesEnabled && shouldHashManifest(appName, gvk) {
//				hash, err := generateManifestHash(un, nil, cacheSettings.resourceOverrides)
//				if err != nil {
//					log.Errorf("Failed to generate manifest hash: %v", err)
//				} else {
//					res.manifestHash = hash
//				}
//			}
//
//			// edge case. we do not label CRDs, so they miss the tracking label we inject. But we still
//			// want the full resource to be available in our cache (to diff), so we store all CRDs
//			return res, res.AppName != "" || gvk.Kind == kube.CustomResourceDefinitionKind
//		}),
//		clustercache.SetLogr(logutils.NewLogrusLogger(log.WithField("server", cluster.Server))),
//		clustercache.SetRetryOptions(clusterCacheAttemptLimit, clusterCacheRetryUseBackoff, isRetryableError),
//		clustercache.SetRespectRBAC(respectRBAC),
//	}
//
//	clusterCache = clustercache.NewClusterCache(clusterCacheConfig, clusterCacheOpts...)
//
//	_ = clusterCache.OnResourceUpdated(func(newRes *clustercache.Resource, oldRes *clustercache.Resource, namespaceResources map[kube.ResourceKey]*clustercache.Resource) {
//		toNotify := make(map[string]bool)
//		var ref v1.ObjectReference
//		if newRes != nil {
//			ref = newRes.Ref
//		} else {
//			ref = oldRes.Ref
//		}
//
//		c.lock.RLock()
//		cacheSettings := c.cacheSettings
//		c.lock.RUnlock()
//
//		if cacheSettings.ignoreResourceUpdatesEnabled && oldRes != nil && newRes != nil && skipResourceUpdate(resInfo(oldRes), resInfo(newRes)) {
//			// Additional check for debug level so we don't need to evaluate the
//			// format string in case of non-debug scenarios
//			if log.GetLevel() >= log.DebugLevel {
//				namespace := ref.Namespace
//				if ref.Namespace == "" {
//					namespace = "(cluster-scoped)"
//				}
//				log.WithFields(log.Fields{
//					"server":      cluster.Server,
//					"namespace":   namespace,
//					"name":        ref.Name,
//					"api-version": ref.APIVersion,
//					"kind":        ref.Kind,
//				}).Debug("Ignoring change of object because none of the watched resource fields have changed")
//			}
//			return
//		}
//
//		for _, r := range []*clustercache.Resource{newRes, oldRes} {
//			if r == nil {
//				continue
//			}
//			app := getApp(r, namespaceResources)
//			if app == "" || skipAppRequeuing(r.ResourceKey()) {
//				continue
//			}
//			toNotify[app] = isRootAppNode(r) || toNotify[app]
//		}
//		c.onObjectUpdated(toNotify, ref)
//	})
//
//	_ = clusterCache.OnEvent(func(event watch.EventType, un *unstructured.Unstructured) {
//		gvk := un.GroupVersionKind()
//		c.metricsServer.IncClusterEventsCount(cluster.Server, gvk.Group, gvk.Kind)
//	})
//
//	c.clusters[server] = clusterCache
//
//	return clusterCache, nil
//}
