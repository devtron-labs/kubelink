package k8sInformer

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
	"helm.sh/helm/v3/pkg/release"
	"io/ioutil"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	HELM_RELEASE_SECRET_TYPE = "helm.sh/release.v1"
	CLUSTER_ADD_REQ          = "CLUSTER_ADD_REQUEST"
	CLUSTER_UPDATE_REQ       = "CLUSTER_UPDATE_REQUEST"
	DEFAULT_CLUSTER          = "default_cluster"
)

type K8sInformer interface {
	BuildInformer(clusterInfo []*bean.ClusterInfo) error
	StartInformer(clusterInfo bean.ClusterInfo, config *rest.Config, mutex *sync.Mutex) error
	SyncInformer(clusterId int) error
	StopInformer(clusterName string)
	//StartInformerAndPopulateCache(clusterId int) error
	GetAllReleaseByClusterId(clusterId int) []*client.DeployedAppDetail
}

type K8sInformerImpl struct {
	logger             *zap.SugaredLogger
	HelmListClusterMap map[string]*client.DeployedAppDetail
	mutex              sync.Mutex
	informerStopper    map[string]chan struct{}
	clusterRepository  repository.ClusterRepository
}

func Newk8sInformerImpl(logger *zap.SugaredLogger, clusterRepository repository.ClusterRepository) *K8sInformerImpl {
	informerFactory := &K8sInformerImpl{
		logger:            logger,
		clusterRepository: clusterRepository,
	}
	informerFactory.HelmListClusterMap = make(map[string]*client.DeployedAppDetail)
	informerFactory.informerStopper = make(map[string]chan struct{})
	go informerFactory.BuildInformerForAllClusters()
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
	var clusterInfo []*bean.ClusterInfo
	for _, model := range models {
		bearerToken := model.Config["bearer_token"]
		clusterInfo = append(clusterInfo, &bean.ClusterInfo{
			ClusterId:   model.Id,
			ClusterName: model.ClusterName,
			BearerToken: bearerToken,
			ServerUrl:   model.ServerUrl,
		})
	}
	err = impl.BuildInformer(clusterInfo)
	if err != nil {
		impl.logger.Errorw("error in starting informer for cluster ")
		return err
	}
	return nil
}

func (impl *K8sInformerImpl) BuildInformer(clusterInfo []*bean.ClusterInfo) error {

	restConfig := &rest.Config{}
	for _, cluster := range clusterInfo {

		if cluster.ClusterId == 0 {
			usr, err := user.Current()
			if err != nil {
				impl.logger.Errorw("Error while getting user current env details", "error", err)
			}
			kubeconfig := flag.String("build-informer", filepath.Join(usr.HomeDir, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
			flag.Parse()
			restConfig, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
			if err != nil {
				impl.logger.Errorw("Error while building config from flags", "error", err)
			}
		} else {
			restConfig.BearerToken = cluster.BearerToken
			restConfig.Host = cluster.ServerUrl
		}

		err := impl.StartInformer(*cluster, restConfig, &impl.mutex)
		if err != nil {
			impl.logger.Errorw("error in starting informer for cluster ", "cluster-name ", cluster.ClusterName, "err", err)
			return err
		}
	}
	return nil

}

func (impl *K8sInformerImpl) deleteSecret(namespace string, name string, client *v1.CoreV1Client) error {
	err := client.Secrets(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (impl *K8sInformerImpl) StartInformer(clusterInfo bean.ClusterInfo, config *rest.Config, mutex *sync.Mutex) error {

	config.Host = "https://20.232.55.111:16443"
	config.BearerToken = "YlA4SlRZOXBkM0tmMzljTklFUjQ1RGtkN2J1OGxBaG12YzBqcld2Rmc5cz0K"
	config.Insecure = true
	httpClientFor, err := rest.HTTPClientFor(config)
	if err != nil {
		fmt.Println("error occurred while overriding k8s client", "reason", err)
		return err
	}
	clusterClient, err := kubernetes.NewForConfigAndClient(config, httpClientFor)
	if err != nil {
		impl.logger.Errorw("error in create k8s config", "err", err)
		return err
	}

	if clusterInfo.ClusterName == DEFAULT_CLUSTER {
		informerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(clusterClient, time.Minute)
		stopper := make(chan struct{})
		secretInformer := informerFactory.Core().V1().Secrets()
		secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if secretObject, ok := obj.(*coreV1.Secret); ok {
					if secretObject.Type != CLUSTER_ADD_REQ && secretObject.Type != CLUSTER_UPDATE_REQ {
						return
					}
					if secretObject.Type == CLUSTER_ADD_REQ {
						data := secretObject.Data
						id := data["cluster_id"][0]
						id_int, err := strconv.Atoi(string(id))
						err = impl.StartInformerAndPopulateCache(id_int)
						impl.logger.Errorw("error in adding informer for cluster", "id", clusterInfo.ClusterId, "name", clusterInfo.ClusterName, "err", err)
						return
					}
					if secretObject.Type == CLUSTER_UPDATE_REQ {
						data := secretObject.Data
						id := data["cluster_id"][0]
						id_int, err := strconv.Atoi(string(id))
						err = impl.SyncInformer(id_int)
						impl.logger.Errorw("error in updating informer for cluster", "id", clusterInfo.ClusterId, "name", clusterInfo.ClusterName, "err", err)
						return
					}
					k8sClient, err := v1.NewForConfigAndClient(config, httpClientFor)
					if err != nil {
						impl.logger.Errorw("error creating k8s client", "error", err)
						return
					}
					err = impl.deleteSecret(secretObject.Namespace, secretObject.Name, k8sClient)
					if err != nil {
						impl.logger.Errorw("error in deleting Cluster create namespace", "err", err)
						return
					}
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
			},
			DeleteFunc: func(obj interface{}) {
			},
		})
		informerFactory.Start(stopper)
		impl.informerStopper[clusterInfo.ClusterName] = stopper

	}
	// these informers will be used to populate cache
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
	impl.StopInformer(clusterInfo.ClusterName)
	//create new informer for cluster with new config
	err = impl.StartInformerAndPopulateCache(clusterId)
	if err != nil {
		impl.logger.Errorw("error in starting informer for ", "cluster name", clusterInfo.ClusterName)
		return err
	}
	return nil
}

func (impl *K8sInformerImpl) StopInformer(clusterName string) {
	stopper := impl.informerStopper[clusterName]
	if stopper != nil {
		close(stopper)
		delete(impl.informerStopper, clusterName)
	}
	return
}

func (impl *K8sInformerImpl) StartInformerAndPopulateCache(clusterId int) error {

	clusterInfo, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Errorw("error in fetching cluster by cluster ids")
		return err
	}
	restConfig := rest.Config{}
	restConfig.Host = clusterInfo.ServerUrl
	restConfig.BearerToken = clusterInfo.Config["bearer_token"]

	restConfig.Host = "https://20.232.55.111:16443"
	restConfig.BearerToken = "YlA4SlRZOXBkM0tmMzljTklFUjQ1RGtkN2J1OGxBaG12YzBqcld2Rmc5cz0K"
	restConfig.Insecure = true

	httpClientFor, err := rest.HTTPClientFor(&restConfig)
	if err != nil {
		fmt.Println("error occurred while overriding k8s client", "reason", err)
		return err
	}
	clusterClient, err := kubernetes.NewForConfigAndClient(&restConfig, httpClientFor)
	if err != nil {
		impl.logger.Errorw("error in create k8s config", "err", err)
		return err
	}

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
				impl.HelmListClusterMap[releaseDTO.Name] = appDetail
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if secretObject, ok := oldObj.(*coreV1.Secret); ok {
				if secretObject.Type != "HELM_RELEASE_SECRET_TYPE" {
					return
				}
				releaseDTO, err := decodeRelease(string(secretObject.Data["release"]))
				if err != nil {
					impl.logger.Errorw("error in decoding release")
				}
				appDetail := &client.DeployedAppDetail{
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
				impl.HelmListClusterMap[releaseDTO.Name] = appDetail
			}
		},
	})
	informerFactory.Start(stopper)
	impl.informerStopper[clusterInfo.ClusterName] = stopper
	return nil
}

func (impl *K8sInformerImpl) GetAllReleaseByClusterId(clusterId int) []*client.DeployedAppDetail {

	var deployedAppDetailList []*client.DeployedAppDetail
	deployedAppDetailMap := impl.HelmListClusterMap

	for _, v := range deployedAppDetailMap {
		if int(v.EnvironmentDetail.ClusterId) == clusterId {
			deployedAppDetailList = append(deployedAppDetailList, v)
		}
	}
	return deployedAppDetailList
}