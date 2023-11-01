package repository

import (
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/go-pg/pg"
	"go.uber.org/zap"
)

type Cluster struct {
	tableName              struct{}          `sql:"cluster" pg:",discard_unknown_columns"`
	Id                     int               `sql:"id,pk"`
	ClusterName            string            `sql:"cluster_name"`
	ServerUrl              string            `sql:"server_url"`
	PrometheusEndpoint     string            `sql:"prometheus_endpoint"`
	Active                 bool              `sql:"active,notnull"`
	CdArgoSetup            bool              `sql:"cd_argo_setup,notnull"`
	Config                 map[string]string `sql:"config"`
	PUserName              string            `sql:"p_username"`
	PPassword              string            `sql:"p_password"`
	PTlsClientCert         string            `sql:"p_tls_client_cert"`
	PTlsClientKey          string            `sql:"p_tls_client_key"`
	AgentInstallationStage int               `sql:"agent_installation_stage"`
	K8sVersion             string            `sql:"k8s_version"`
	ErrorInConnecting      string            `sql:"error_in_connecting"`
	InsecureSkipTlsVerify  bool              `sql:"insecure_skip_tls_verify"`
	sql.AuditLog
}

func NewClusterRepositoryImpl(dbConnection *pg.DB, logger *zap.SugaredLogger) *ClusterRepositoryImpl {
	return &ClusterRepositoryImpl{
		dbConnection: dbConnection,
		Logger:       logger,
	}
}

type ClusterRepositoryImpl struct {
	dbConnection *pg.DB
	Logger       *zap.SugaredLogger
}

type ClusterRepository interface {
	FindAllActive() ([]*Cluster, error)
	FindById(id int) (*Cluster, error)
	FindByIdWithActiveFalse(id int) (*Cluster, error)
	GetDBConnection() *pg.DB
	GetClusterConfigFromClientBean(config *client.ClusterConfig) *k8sUtils.ClusterConfig
}

func (impl ClusterRepositoryImpl) GetDBConnection() *pg.DB {
	return impl.dbConnection
}

func (cluster *Cluster) GetClusterInfo() *bean.ClusterInfo {
	clusterInfo := &bean.ClusterInfo{}
	if cluster != nil {
		config := cluster.Config
		bearerToken := config["bearer_token"]
		clusterInfo = &bean.ClusterInfo{
			ClusterId:             cluster.Id,
			ClusterName:           cluster.ClusterName,
			BearerToken:           bearerToken,
			ServerUrl:             cluster.ServerUrl,
			InsecureSkipTLSVerify: cluster.InsecureSkipTlsVerify,
		}
		if cluster.InsecureSkipTlsVerify == false {
			clusterInfo.KeyData = config[k8sUtils.TlsKey]
			clusterInfo.CertData = config[k8sUtils.CertData]
			clusterInfo.CAData = config[k8sUtils.CertificateAuthorityData]
		}
	}
	return clusterInfo
}

func (impl ClusterRepositoryImpl) GetClusterConfigFromClientBean(config *client.ClusterConfig) *k8sUtils.ClusterConfig {
	clusterConfig := &k8sUtils.ClusterConfig{}
	if config != nil {
		clusterConfig = &k8sUtils.ClusterConfig{
			ClusterName:           config.ClusterName,
			Host:                  config.ApiServerUrl,
			BearerToken:           config.Token,
			InsecureSkipTLSVerify: config.InsecureSkipTLSVerify,
		}
		if config.InsecureSkipTLSVerify == false {
			clusterConfig.KeyData = config.GetKeyData()
			clusterConfig.CertData = config.GetCertData()
			clusterConfig.CAData = config.GetCaData()
		}
	}
	return clusterConfig
}

func (impl ClusterRepositoryImpl) FindAllActive() ([]*Cluster, error) {
	var clusters []*Cluster
	err := impl.dbConnection.
		Model(&clusters).
		Where("active=?", true).
		Select()
	return clusters, err
}

func (impl ClusterRepositoryImpl) FindById(id int) (*Cluster, error) {
	var cluster Cluster
	err := impl.dbConnection.
		Model(&cluster).
		Where("id= ? ", id).
		Where("active =?", true).
		Select()
	return &cluster, err
}

func (impl ClusterRepositoryImpl) FindByIdWithActiveFalse(id int) (*Cluster, error) {
	var cluster Cluster
	err := impl.dbConnection.
		Model(&cluster).
		Where("id= ? ", id).
		Where("active =?", false).
		Select()
	return &cluster, err
}
