package clusterCache

import (
	clustercache "github.com/argoproj/gitops-engine/pkg/cache"
	"github.com/caarlos0/env"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"strconv"
)

type ClusterCache interface {
	syncClusterCache(clusterInfo bean.ClusterInfo) error
}

type ClusterCacheConfig struct {
	ClusterIdList []string `env:"CLUSTER_ID_LIST" envSeparator:","`
}

func GetClusterCacheConfig() (*ClusterCacheConfig, error) {
	cfg := &ClusterCacheConfig{}
	err := env.Parse(cfg)
	return cfg, err
}

type ClusterCacheImpl struct {
	logger             *zap.SugaredLogger
	clusterCacheConfig *ClusterCacheConfig
	clusterRepository  repository.ClusterRepository
	k8sUtil            *k8sUtils.K8sUtil
}

func NewClusterCacheImpl(logger *zap.SugaredLogger, clusterCacheConfig *ClusterCacheConfig,
	clusterRepository repository.ClusterRepository, k8sUtil *k8sUtils.K8sUtil) *ClusterCacheImpl {
	clusterCacheImpl := &ClusterCacheImpl{
		logger:             logger,
		clusterCacheConfig: clusterCacheConfig,
		clusterRepository:  clusterRepository,
		k8sUtil:            k8sUtil,
	}

	if len(clusterCacheConfig.ClusterIdList) > 0 {
		go clusterCacheImpl.SyncCache()
	}
	return clusterCacheImpl
}

func parseClusterIdList(clusterIdList []string) []int {
	clusterIds := make([]int, 0)
	for _, clusterId := range clusterIdList {
		intValue, _ := strconv.Atoi(clusterId)
		clusterIds = append(clusterIds, intValue)
	}
	return clusterIds
}

func (impl *ClusterCacheImpl) SyncCache() error {
	clusterIdList := parseClusterIdList(impl.clusterCacheConfig.ClusterIdList)
	clusterIdToCache := make(map[int]clustercache.ClusterCache)
	for _, clusterId := range clusterIdList {
		var cache clustercache.ClusterCache
		clusterIdToCache[clusterId] = cache
	}
	liveState := liveStateCache{clusterIdToCache}
	for _, clusterId := range clusterIdList {
		model, err := impl.clusterRepository.FindById(clusterId)
		if err != nil {
			impl.logger.Errorw("error in getting cluster from db by cluster id", "clusterId", clusterId)
		}
		clusterInfo := k8sInformer.GetClusterInfo(model)

		err = impl.syncClusterCache(*clusterInfo, liveState)
		if err != nil {
			impl.logger.Error("error in cluster cache sync for cluster ", "cluster-name ", clusterInfo.ClusterName, "err", err)
			return err
		}
	}
	return nil
}

func (impl *ClusterCacheImpl) syncClusterCache(clusterInfo bean.ClusterInfo, liveStageCache liveStateCache) error {
	c, err := liveStageCache.getCluster(clusterInfo, impl)
	if err != nil {
		impl.logger.Errorw("failed to get cluster info for", "clusterId", clusterInfo.ClusterId, "error", err)
		return err
	}
	err = c.EnsureSynced()
	if err != nil {
		impl.logger.Errorw("error in syncing cluster cache", "sync-error", err)
		return err
	}
	return nil
}

func getClusterCacheOptions() []clustercache.UpdateSettingsFunc {
	clusterCacheOpts := []clustercache.UpdateSettingsFunc{
		clustercache.SetListSemaphore(semaphore.NewWeighted(clusterCacheListSemaphoreSize)),
		clustercache.SetListPageSize(clusterCacheListPageSize),
		clustercache.SetListPageBufferSize(clusterCacheListPageBufferSize),
		clustercache.SetWatchResyncTimeout(clusterCacheWatchResyncDuration),
		clustercache.SetClusterSyncRetryTimeout(clusterSyncRetryTimeoutDuration),
		clustercache.SetResyncTimeout(clusterCacheResyncDuration),
		//clustercache.SetSettings(cacheSettings.clusterSettings),
		//drop this for now,
		clustercache.SetNamespaces(cluster.Namespaces),
		//clustercache.SetClusterResources(cluster.ClusterResources),
		clustercache.SetPopulateResourceInfoHandler(func(un *unstructured.Unstructured, isRoot bool) (interface{}, bool) {
			//fmt.Println("resource updated")
			//res := &ResourceInfo{}
			//	populateNodeInfo(un, res, resourceCustomLabels)
			//	c.lock.RLock()
			//	cacheSettings := c.cacheSettings
			//	c.lock.RUnlock()
			//
			//	res.Health, _ = health.GetResourceHealth(un, cacheSettings.clusterSettings.ResourceHealthOverride)
			//
			//	appName := c.resourceTracking.GetAppName(un, cacheSettings.appInstanceLabelKey, cacheSettings.trackingMethod)
			//	if isRoot && appName != "" {
			//		res.AppName = appName
			//	}
			//
			//	gvk := un.GroupVersionKind()
			//
			//	if cacheSettings.ignoreResourceUpdatesEnabled && shouldHashManifest(appName, gvk) {
			//		hash, err := generateManifestHash(un, nil, cacheSettings.resourceOverrides)
			//		if err != nil {
			//			log.Errorf("Failed to generate manifest hash: %v", err)
			//		} else {
			//			res.manifestHash = hash
			//		}
			//	}
			//
			//	// edge case. we do not label CRDs, so they miss the tracking label we inject. But we still
			//	// want the full resource to be available in our cache (to diff), so we store all CRDs
			//	return res, res.AppName != "" || gvk.Kind == kube.CustomResourceDefinitionKind
			return nil, true
		}),
		clustercache.SetRetryOptions(clusterCacheAttemptLimit, clusterCacheRetryUseBackoff, isRetryableError),
	}
	return clusterCacheOpts
}
