/*
 * Copyright (c) 2020 Devtron Labs
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package internal

import (
	"go.uber.org/zap"
	"sync"
)

type ChartRepositoryLocker struct {
	logger  *zap.SugaredLogger
	locks   map[string]*sync.Mutex // repoName vs lock
	mapLock sync.Mutex             // to make the map safe concurrently
}

func NewChartRepositoryLocker(logger *zap.SugaredLogger) *ChartRepositoryLocker {
	return &ChartRepositoryLocker{
		logger: logger,
		locks:  make(map[string]*sync.Mutex),
	}
}

func (l *ChartRepositoryLocker) Lock(chartRepoName string) {
	l.logger.Infow("locking", "chartRepoName", chartRepoName)
	l.getLockBy(chartRepoName).Lock()
}

func (l *ChartRepositoryLocker) Unlock(chartRepoName string) {
	l.logger.Infow("unlocking", "chartRepoName", chartRepoName)
	l.getLockBy(chartRepoName).Unlock()
}

func (l *ChartRepositoryLocker) getLockBy(chartRepoName string) *sync.Mutex {
	l.mapLock.Lock()
	defer l.mapLock.Unlock()

	ret, found := l.locks[chartRepoName]
	if found {
		return ret
	}

	ret = &sync.Mutex{}
	l.locks[chartRepoName] = ret
	return ret
}
