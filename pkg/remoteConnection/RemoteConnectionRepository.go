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

package remoteConnection

import (
	"github.com/devtron-labs/kubelink/pkg/model"
	"github.com/go-pg/pg"
	"go.uber.org/zap"
)

type RemoteConnectionRepository interface {
	GetById(id int) (*model.RemoteConnectionConfig, error)
}

type RemoteConnectionRepositoryImpl struct {
	logger       *zap.SugaredLogger
	dbConnection *pg.DB
}

func NewRemoteConnectionRepositoryImpl(dbConnection *pg.DB, logger *zap.SugaredLogger) *RemoteConnectionRepositoryImpl {
	return &RemoteConnectionRepositoryImpl{
		logger:       logger,
		dbConnection: dbConnection,
	}
}

func (repo *RemoteConnectionRepositoryImpl) GetById(id int) (*model.RemoteConnectionConfig, error) {
	model := &model.RemoteConnectionConfig{}
	err := repo.dbConnection.Model(model).
		Where("id = ?", id).
		Where("deleted = ?", false).
		Select()
	if err != nil {
		repo.logger.Errorw("error in getting server connection config", "err", err, "id", id)
		return nil, err
	}
	return model, nil
}
