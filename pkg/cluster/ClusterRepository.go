/*
 * Copyright (c) 2024. Devtron Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package repository

import (
	"github.com/devtron-labs/kubelink/pkg/remoteConnection"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
	"go.uber.org/zap"
)

type Cluster struct {
	tableName                struct{}          `sql:"cluster" pg:",discard_unknown_columns"`
	Id                       int               `sql:"id,pk"`
	ClusterName              string            `sql:"cluster_name"`
	ServerUrl                string            `sql:"server_url"`
	RemoteConnectionConfigId int               `sql:"remote_connection_config_id"`
	PrometheusEndpoint       string            `sql:"prometheus_endpoint"`
	Active                   bool              `sql:"active,notnull"`
	CdArgoSetup              bool              `sql:"cd_argo_setup,notnull"`
	Config                   map[string]string `sql:"config"`
	PUserName                string            `sql:"p_username"`
	PPassword                string            `sql:"p_password"`
	PTlsClientCert           string            `sql:"p_tls_client_cert"`
	PTlsClientKey            string            `sql:"p_tls_client_key"`
	AgentInstallationStage   int               `sql:"agent_installation_stage"`
	K8sVersion               string            `sql:"k8s_version"`
	ErrorInConnecting        string            `sql:"error_in_connecting"`
	IsVirtualCluster         bool              `sql:"is_virtual_cluster"`
	InsecureSkipTlsVerify    bool              `sql:"insecure_skip_tls_verify"`
	ProxyUrl                 string            `sql:"proxy_url"`
	ToConnectWithSSHTunnel   bool              `sql:"to_connect_with_ssh_tunnel"`
	SSHTunnelUser            string            `sql:"ssh_tunnel_user"`
	SSHTunnelPassword        string            `sql:"ssh_tunnel_password"`
	SSHTunnelAuthKey         string            `sql:"ssh_tunnel_auth_key"`
	SSHTunnelServerAddress   string            `sql:"ssh_tunnel_server_address"`
	RemoteConnectionConfig   *remoteConnection.RemoteConnectionConfig
	sql.AuditLog
}

func NewClusterRepositoryImpl(dbConnection *pg.DB, logger *zap.SugaredLogger) *ClusterRepositoryImpl {
	return &ClusterRepositoryImpl{
		dbConnection: dbConnection,
		logger:       logger,
	}
}

type ClusterRepositoryImpl struct {
	dbConnection *pg.DB
	logger       *zap.SugaredLogger
}

type ClusterRepository interface {
	FindAllActive() ([]*Cluster, error)
	FindById(id int) (*Cluster, error)
	FindByIdWithActiveFalse(id int) (*Cluster, error)
}

func (impl ClusterRepositoryImpl) FindAllActive() ([]*Cluster, error) {
	var clusters []*Cluster
	err := impl.dbConnection.
		Model(&clusters).
		Column("cluster.*", "RemoteConnectionConfig").
		Where("cluster.active = ?", true).
		WhereGroup(func(q *orm.Query) (*orm.Query, error) {
			q = q.WhereOr("cluster.is_virtual_cluster = ?", false).
				WhereOr("cluster.is_virtual_cluster IS NULL")
			return q, nil
		}).
		Select()
	return clusters, err
}

func (impl ClusterRepositoryImpl) FindById(id int) (*Cluster, error) {
	var cluster Cluster
	err := impl.dbConnection.
		Model(&cluster).
		Column("cluster.*", "RemoteConnectionConfig").
		Where("cluster.id= ? ", id).
		Where("cluster.active =?", true).
		Select()
	return &cluster, err
}

func (impl ClusterRepositoryImpl) FindByIdWithActiveFalse(id int) (*Cluster, error) {
	var cluster Cluster
	err := impl.dbConnection.
		Model(&cluster).
		Column("cluster.*", "RemoteConnectionConfig").
		Where("cluster.id= ? ", id).
		Where("cluster.active =?", false).
		Select()
	return &cluster, err
}
