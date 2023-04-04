package k8sInformer

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/caarlos0/env"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
	"helm.sh/helm/v3/pkg/release"
	"io/ioutil"
	coreV1 "k8s.io/api/core/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"strconv"
	"sync"
	"time"
)

const (
	HELM_RELEASE_SECRET_TYPE         = "helm.sh/release.v1"
	CLUSTER_MODIFY_EVENT_SECRET_TYPE = "cluster.request/modify"
	DEFAULT_CLUSTER                  = "default_cluster"
	INFORMER_ALREADY_EXIST_MESSAGE   = "INFORMER_ALREADY_EXIST"
)

type K8sInformer interface {
	StartInformer(clusterInfo bean.ClusterInfo) error
	SyncInformer(clusterId int) error
	StopInformer(clusterName string, clusterId int)
	StartInformerAndPopulateCache(clusterId int) error
	GetAllReleaseByClusterId(clusterId int) []*client.DeployedAppDetail
}

type HelmReleaseConfig struct {
	EnableHelmReleaseCache bool `env:"ENABLE_HELM_RELEASE_CACHE" envDefault:"true"`
}

func GetHelmReleaseConfig() (*HelmReleaseConfig, error) {
	cfg := &HelmReleaseConfig{}
	err := env.Parse(cfg)
	return cfg, err
}

type K8sInformerImpl struct {
	logger             *zap.SugaredLogger
	HelmListClusterMap map[int]map[string]*client.DeployedAppDetail
	mutex              sync.Mutex
	informerStopper    map[string]chan struct{}
	clusterRepository  repository.ClusterRepository
	helmReleaseConfig  *HelmReleaseConfig
}

func Newk8sInformerImpl(logger *zap.SugaredLogger, clusterRepository repository.ClusterRepository, helmReleaseConfig *HelmReleaseConfig) *K8sInformerImpl {
	informerFactory := &K8sInformerImpl{
		logger:            logger,
		clusterRepository: clusterRepository,
		helmReleaseConfig: helmReleaseConfig,
	}
	informerFactory.HelmListClusterMap = make(map[int]map[string]*client.DeployedAppDetail)
	informerFactory.informerStopper = make(map[string]chan struct{})
	if helmReleaseConfig.EnableHelmReleaseCache {
		go informerFactory.BuildInformerForAllClusters()
	}
	return informerFactory
}

func decodeRelease(data string) (*release.Release, error) {
	// base64 decode string
	b64 := base64.StdEncoding
	b, err := b64.DecodeString(data)
	if err != nil {
		return nil, err
	}

	var magicGzip = []byte{0x1f, 0x8b, 0x08}

	// For backwards compatibility with releases that were stored before
	// compression was introduced we skip decompression if the
	// gzip magic header is not found
	if len(b) > 3 && bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b2, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}

	var rls release.Release
	// unmarshal release object bytes
	if err := json.Unmarshal(b, &rls); err != nil {
		return nil, err
	}
	return &rls, nil
}

func (impl *K8sInformerImpl) BuildInformerForAllClusters() error {
	models, err := impl.clusterRepository.FindAllActive()
	if err != nil {
		impl.logger.Errorw("error in fetching clusters", "err", err)
		return err
	}
	for _, model := range models {

		bearerToken := model.Config["bearer_token"]

		clusterInfo := &bean.ClusterInfo{
			ClusterId:   model.Id,
			ClusterName: model.ClusterName,
			BearerToken: bearerToken,
			ServerUrl:   model.ServerUrl,
		}
		err := impl.StartInformer(*clusterInfo)
		if err != nil {
			impl.logger.Errorw("error in starting informer for cluster ", "cluster-name ", clusterInfo.ClusterName, "err", err)
			return err
		}

	}

	return nil
}

func (impl *K8sInformerImpl) StartInformer(clusterInfo bean.ClusterInfo) error {

	restConfig := &rest.Config{}
	if clusterInfo.ClusterName == DEFAULT_CLUSTER {
		config, err := rest.InClusterConfig()
		if err != nil {
			impl.logger.Errorw("error in fetch default cluster config", "err", err, "servername", restConfig.ServerName)
			return err
		}
		restConfig = config
	} else {
		restConfig.BearerToken = clusterInfo.BearerToken
		restConfig.Host = clusterInfo.ServerUrl
		restConfig.Insecure = true
	}

	httpClientFor, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		impl.logger.Errorw("error occurred while overriding k8s client", "reason", err)
		return err
	}
	clusterClient, err := kubernetes.NewForConfigAndClient(restConfig, httpClientFor)
	if err != nil {
		impl.logger.Errorw("error in create k8s config", "err", err)
		return err
	}

	// for default cluster adding an extra informer, this informer will add informer on new clusters
	if clusterInfo.ClusterName == DEFAULT_CLUSTER {
		impl.logger.Debugw("Starting informer, reading new cluster request for default cluster")
		informerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(clusterClient, time.Minute)
		stopper := make(chan struct{})
		secretInformer := informerFactory.Core().V1().Secrets()
		secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if secretObject, ok := obj.(*coreV1.Secret); ok {
					if secretObject.Type != CLUSTER_MODIFY_EVENT_SECRET_TYPE {
						return
					}
					data := secretObject.Data
					action := data["action"]
					id := string(data["cluster_id"])
					id_int, _ := strconv.Atoi(id)

					if string(action) == "add" {
						err = impl.StartInformerAndPopulateCache(id_int)
						if err != nil && err != errors.New(INFORMER_ALREADY_EXIST_MESSAGE) {
							impl.logger.Debugw("error in adding informer for cluster", "id", id_int, "err", err)
							return
						}
					}
					if string(action) == "update" {
						err = impl.SyncInformer(id_int)
						if err != nil && err != errors.New(INFORMER_ALREADY_EXIST_MESSAGE) {
							impl.logger.Debugw("error in updating informer for cluster", "id", clusterInfo.ClusterId, "name", clusterInfo.ClusterName, "err", err)
							return
						}
					}
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				if secretObject, ok := newObj.(*coreV1.Secret); ok {

					if secretObject.Type != CLUSTER_MODIFY_EVENT_SECRET_TYPE {
						return
					}
					data := secretObject.Data
					action := data["action"]
					id := string(data["cluster_id"])
					id_int, _ := strconv.Atoi(id)

					if string(action) == "add" {
						err = impl.StartInformerAndPopulateCache(clusterInfo.ClusterId)
						if err != nil && err != errors.New(INFORMER_ALREADY_EXIST_MESSAGE) {
							impl.logger.Errorw("error in adding informer for cluster", "id", id_int, "err", err)
							return
						}
					}
					if string(action) == "update" {
						err := impl.SyncInformer(id_int)
						if err != nil && err != errors.New(INFORMER_ALREADY_EXIST_MESSAGE) {
							impl.logger.Errorw("error in updating informer for cluster", "id", clusterInfo.ClusterId, "name", clusterInfo.ClusterName, "err", err)
							return
						}
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				if secretObject, ok := obj.(*coreV1.Secret); ok {
					if secretObject.Type != CLUSTER_MODIFY_EVENT_SECRET_TYPE {
						return
					}
					data := secretObject.Data
					action := data["action"]
					id := string(data["cluster_id"])
					id_int, _ := strconv.Atoi(id)

					if string(action) == "delete" {
						deleteClusterInfo, err := impl.clusterRepository.FindByIdWithActiveFalse(id_int)
						if err != nil {
							impl.logger.Errorw("Error in fetching cluster by id", "cluster-id ", id_int)
							return
						}
						impl.StopInformer(deleteClusterInfo.ClusterName, deleteClusterInfo.Id)
						if err != nil {
							impl.logger.Errorw("error in updating informer for cluster", "id", clusterInfo.ClusterId, "name", clusterInfo.ClusterName, "err", err)
							return
						}
					}
				}
			},
		})
		informerFactory.Start(stopper)
		impl.informerStopper[clusterInfo.ClusterName+"_second_informer"] = stopper

	}
	// these informers will be used to populate helm release cache

	err = impl.StartInformerAndPopulateCache(clusterInfo.ClusterId)
	if err != nil {
		impl.logger.Errorw("error in creating informer for new cluster", "err", err)
		return err
	}

	return nil
}

func (impl *K8sInformerImpl) SyncInformer(clusterId int) error {

	clusterInfo, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Errorw("error in fetching cluster info by id", "err", err)
		return err
	}
	//before creating new informer for cluster, close existing one
	impl.logger.Debugw("stopping informer for cluster - ", "cluster-name", clusterInfo.ClusterName, "cluster-id", clusterInfo.Id)
	impl.StopInformer(clusterInfo.ClusterName, clusterInfo.Id)
	impl.logger.Debugw("informer stopped", "cluster-name", clusterInfo.ClusterName, "cluster-id", clusterInfo.Id)
	//create new informer for cluster with new config
	err = impl.StartInformerAndPopulateCache(clusterId)
	if err != nil {
		impl.logger.Errorw("error in starting informer for ", "cluster name", clusterInfo.ClusterName)
		return err
	}
	return nil
}

func (impl *K8sInformerImpl) StopInformer(clusterName string, clusterId int) {
	stopper := impl.informerStopper[clusterName+string(rune(clusterId))]
	if stopper != nil {
		close(stopper)
		delete(impl.informerStopper, clusterName+string(rune(clusterId)))
	}
	return
}

func (impl *K8sInformerImpl) StartInformerAndPopulateCache(clusterId int) error {

	clusterInfo, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Errorw("error in fetching cluster by cluster ids")
		return err
	}

	if _, ok := impl.informerStopper[clusterInfo.ClusterName+string(rune(clusterId))]; ok {
		impl.logger.Debugw(fmt.Sprintf("informer for %s already exist", clusterInfo.ClusterName))
		return errors.New(INFORMER_ALREADY_EXIST_MESSAGE)
	}

	impl.logger.Infow("starting informer for cluster - ", "cluster-id", clusterInfo.Id, "cluster-name", clusterInfo.ClusterName)

	restConfig := &rest.Config{}

	if clusterInfo.ClusterName == DEFAULT_CLUSTER {
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			impl.logger.Errorw("error in fetch default cluster config", "err", err, "clusterName", clusterInfo.ClusterName)
			return err
		}
	} else {
		restConfig = &rest.Config{
			Host:            clusterInfo.ServerUrl,
			BearerToken:     clusterInfo.Config["bearer_token"],
			TLSClientConfig: rest.TLSClientConfig{Insecure: true},
		}
	}

	httpClientFor, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		fmt.Println("error occurred while overriding k8s client", "reason", err)
		return err
	}
	clusterClient, err := kubernetes.NewForConfigAndClient(restConfig, httpClientFor)
	if err != nil {
		impl.logger.Errorw("error in create k8s config", "err", err)
		return err
	}

	impl.mutex.Lock()
	impl.HelmListClusterMap[clusterId] = make(map[string]*client.DeployedAppDetail)
	impl.mutex.Unlock()

	informerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(clusterClient, time.Minute)
	stopper := make(chan struct{})
	secretInformer := informerFactory.Core().V1().Secrets()
	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if secretObject, ok := obj.(*coreV1.Secret); ok {

				if secretObject.Type != HELM_RELEASE_SECRET_TYPE {
					return
				}
				releaseDTO, err := decodeRelease(string(secretObject.Data["release"]))
				if err != nil {
					impl.logger.Errorw("error in decoding release")
				}
				appDetail := &client.DeployedAppDetail{
					AppId:        util.GetAppId(int32(clusterInfo.Id), releaseDTO),
					AppName:      releaseDTO.Name,
					ChartName:    releaseDTO.Chart.Name(),
					ChartAvatar:  releaseDTO.Chart.Metadata.Icon,
					LastDeployed: timestamppb.New(releaseDTO.Info.LastDeployed.Time),
					EnvironmentDetail: &client.EnvironmentDetails{
						ClusterId:   int32(clusterInfo.Id),
						ClusterName: clusterInfo.ClusterName,
						Namespace:   releaseDTO.Namespace,
					},
				}
				impl.mutex.Lock()
				defer impl.mutex.Unlock()
				// adding cluster id with release name because there can be case when two cluster have release with same name
				impl.HelmListClusterMap[clusterId][releaseDTO.Name+string(rune(clusterInfo.Id))] = appDetail
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if secretObject, ok := oldObj.(*coreV1.Secret); ok {
				if secretObject.Type != HELM_RELEASE_SECRET_TYPE {
					return
				}
				releaseDTO, err := decodeRelease(string(secretObject.Data["release"]))
				if err != nil {
					impl.logger.Errorw("error in decoding release")
				}
				appDetail := &client.DeployedAppDetail{
					AppId:        util.GetAppId(int32(clusterInfo.Id), releaseDTO),
					AppName:      releaseDTO.Name,
					ChartName:    releaseDTO.Chart.Name(),
					ChartAvatar:  releaseDTO.Chart.Metadata.Icon,
					LastDeployed: timestamppb.New(releaseDTO.Info.LastDeployed.Time),
					EnvironmentDetail: &client.EnvironmentDetails{
						ClusterId:   int32(clusterInfo.Id),
						ClusterName: clusterInfo.ClusterName,
						Namespace:   releaseDTO.Namespace,
					},
				}
				impl.mutex.Lock()
				defer impl.mutex.Unlock()
				// adding cluster id with release name because there can be case when two cluster have release with same name
				impl.HelmListClusterMap[clusterId][releaseDTO.Name+string(rune(clusterInfo.Id))] = appDetail
			}
		},
		DeleteFunc: func(obj interface{}) {
			if secretObject, ok := obj.(*coreV1.Secret); ok {
				if secretObject.Type != HELM_RELEASE_SECRET_TYPE {
					return
				}
				releaseDTO, err := decodeRelease(string(secretObject.Data["release"]))
				if err != nil {
					impl.logger.Errorw("error in decoding release")
				}
				impl.mutex.Lock()
				defer impl.mutex.Unlock()
				delete(impl.HelmListClusterMap[clusterId], releaseDTO.Name+string(rune(clusterInfo.Id)))
			}
		},
	})
	informerFactory.Start(stopper)
	impl.logger.Infow("informer started for cluster: ", "cluster_id", clusterInfo.Id, "cluster_name", clusterInfo.ClusterName)
	impl.informerStopper[clusterInfo.ClusterName+string(rune(clusterInfo.Id))] = stopper
	return nil
}

func (impl *K8sInformerImpl) GetAllReleaseByClusterId(clusterId int) []*client.DeployedAppDetail {

	var deployedAppDetailList []*client.DeployedAppDetail

	impl.mutex.Lock()
	releaseMap := impl.HelmListClusterMap[clusterId]
	impl.mutex.Unlock()

	for _, v := range releaseMap {
		if int(v.EnvironmentDetail.ClusterId) == clusterId {
			deployedAppDetailList = append(deployedAppDetailList, v)
		}
	}
	return deployedAppDetailList
}
