package clusterCache

import clustercache "github.com/argoproj/gitops-engine/pkg/cache"

func SetListSemaphore(port int) clustercache.UpdateSettingsFunc {
	return func(cache *clusterCache) {
		cache.listSemaphore = listSemaphore
	}
}
