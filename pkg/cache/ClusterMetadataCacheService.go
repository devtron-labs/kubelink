package cache

import (
	"errors"
	clustercache "github.com/argoproj/gitops-engine/pkg/cache"
	"github.com/caarlos0/env"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"go.uber.org/zap"
	"sync"
)

type ClusterCache interface {
	k8sInformer.ClusterSecretUpdateListener
	GetClusterCacheByClusterId(clusterId int) (clustercache.ClusterCache, error)
}

type ClusterCacheConfig struct {
	ClusterIdList []int `env:"CLUSTER_ID_LIST" envSeparator:","`
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
	clustersCache      map[int]clustercache.ClusterCache
	rwMutex            sync.RWMutex
	k8sInformer        k8sInformer.K8sInformer
}

func NewClusterCacheImpl(logger *zap.SugaredLogger, clusterCacheConfig *ClusterCacheConfig,
	clusterRepository repository.ClusterRepository, k8sUtil *k8sUtils.K8sUtil, k8sInformer k8sInformer.K8sInformer) *ClusterCacheImpl {

	clustersCache := make(map[int]clustercache.ClusterCache)
	clusterCacheImpl := &ClusterCacheImpl{
		logger:             logger,
		clusterCacheConfig: clusterCacheConfig,
		clusterRepository:  clusterRepository,
		k8sUtil:            k8sUtil,
		clustersCache:      clustersCache,
		k8sInformer:        k8sInformer,
	}
	k8sInformer.RegisterListener(clusterCacheImpl)

	if len(clusterCacheConfig.ClusterIdList) > 0 {
		err := clusterCacheImpl.SyncCache()
		if err != nil {
			return nil
		}
	}
	return clusterCacheImpl
}

func (impl *ClusterCacheImpl) getClusterInfoByClusterId(clusterId int) (*bean.ClusterInfo, error) {
	model, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Errorw("error in getting cluster from db by cluster id", "clusterId", clusterId)
		return nil, err
	}
	clusterInfo := k8sInformer.GetClusterInfo(model)
	return clusterInfo, nil
}

func (impl *ClusterCacheImpl) OnStateChange(clusterId int, action string) {
	switch action {
	case k8sInformer.UPDATE:
		clusterInfo, err := impl.getClusterInfoByClusterId(clusterId)
		if err != nil {
			impl.logger.Errorw("error in getting clusterInfo by cluster id", "clusterId", clusterId)
			return
		}
		impl.logger.Infow("syncing cluster cache on cluster config update", "clusterId", clusterId)
		go impl.SyncClusterCache(clusterInfo)
	case k8sInformer.DELETE:
		impl.logger.Infow("invalidating cluster cache on cluster config delete", "clusterId", clusterId)
		impl.rwMutex.Lock()
		impl.clustersCache[clusterId].Invalidate()
		impl.rwMutex.Unlock()

		delete(impl.clustersCache, clusterId)
	}
}

func (impl *ClusterCacheImpl) SyncCache() error {
	for _, clusterId := range impl.clusterCacheConfig.ClusterIdList {
		clusterInfo, err := impl.getClusterInfoByClusterId(clusterId)
		if err != nil {
			impl.logger.Errorw("error in getting clusterInfo by cluster id", "clusterId", clusterId)
			continue
		}

		go impl.SyncClusterCache(clusterInfo)
	}
	return nil
}

func (impl *ClusterCacheImpl) SyncClusterCache(clusterInfo *bean.ClusterInfo) (clustercache.ClusterCache, error) {
	impl.logger.Infow("cluster cache sync started..", "clusterId", clusterInfo.ClusterId)
	cache, err := impl.getClusterCache(clusterInfo)
	if err != nil {
		impl.logger.Errorw("failed to get cluster info for", "clusterId", clusterInfo.ClusterId, "error", err)
		return cache, err
	}
	err = cache.EnsureSynced()
	if err != nil {
		impl.logger.Errorw("error in syncing cluster cache", "clusterId", clusterInfo.ClusterId, "sync-error", err)
		return cache, err
	}
	impl.rwMutex.Lock()
	impl.clustersCache[clusterInfo.ClusterId] = cache
	impl.rwMutex.Unlock()
	return cache, nil
}

func (impl *ClusterCacheImpl) getClusterCache(clusterInfo *bean.ClusterInfo) (clustercache.ClusterCache, error) {
	cache, err := impl.GetClusterCacheByClusterId(clusterInfo.ClusterId)
	if err == nil {
		return cache, nil
	}
	clusterConfig := clusterInfo.GetClusterConfig()
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting rest config", "err", err, "clusterName", clusterConfig.ClusterName)
		return cache, err
	}
	cache = clustercache.NewClusterCache(restConfig, getClusterCacheOptions()...)
	return cache, nil
}

func (impl *ClusterCacheImpl) GetClusterCacheByClusterId(clusterId int) (clustercache.ClusterCache, error) {
	impl.rwMutex.RLock()
	defer impl.rwMutex.RUnlock()
	if clusterCache, found := impl.clustersCache[clusterId]; found {
		return clusterCache, nil
	}
	return nil, errors.New("cluster cache not yet synced for this cluster id")
}
