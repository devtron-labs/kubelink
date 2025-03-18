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
	"github.com/devtron-labs/kubelink/pkg/model"
	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
	"go.uber.org/zap"
)

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
	FindAllActive() ([]*model.Cluster, error)
	FindById(id int) (*model.Cluster, error)
	FindByIdWithActiveFalse(id int) (*model.Cluster, error)
}

func (impl ClusterRepositoryImpl) FindAllActive() ([]*model.Cluster, error) {
	var clusters []*model.Cluster
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

func (impl ClusterRepositoryImpl) FindById(id int) (*model.Cluster, error) {
	var cluster model.Cluster
	err := impl.dbConnection.
		Model(&cluster).
		Column("cluster.*", "RemoteConnectionConfig").
		Where("cluster.id= ? ", id).
		Where("cluster.active =?", true).
		Select()
	return &cluster, err
}

func (impl ClusterRepositoryImpl) FindByIdWithActiveFalse(id int) (*model.Cluster, error) {
	var cluster model.Cluster
	err := impl.dbConnection.
		Model(&cluster).
		Column("cluster.*", "RemoteConnectionConfig").
		Where("cluster.id= ? ", id).
		Where("cluster.active =?", false).
		Select()
	return &cluster, err
}
