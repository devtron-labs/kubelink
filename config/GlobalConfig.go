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

package config

import "github.com/caarlos0/env"

type HelmReleaseConfig struct {
	EnableHelmReleaseCache    bool   `env:"ENABLE_HELM_RELEASE_CACHE" envDefault:"true"`
	MaxCountForHelmRelease    int    `env:"MAX_COUNT_FOR_HELM_RELEASE" envDefault:"20"`
	ManifestFetchBatchSize    int    `env:"MANIFEST_FETCH_BATCH_SIZE" envDefault:"2"`
	RunHelmInstallInAsyncMode bool   `env:"RUN_HELM_INSTALL_IN_ASYNC_MODE" envDefault:"false"`
	ParentChildGvkMapping     string `env:"PARENT_CHILD_GVK_MAPPING" envDefault:""`
	ChartWorkingDirectory     string `env:"CHART_WORKING_DIRECTORY" envDefault:"/home/devtron/devtroncd/charts/"`
}

func GetHelmReleaseConfig() (*HelmReleaseConfig, error) {
	cfg := &HelmReleaseConfig{}
	err := env.Parse(cfg)
	return cfg, err
}

func (c *HelmReleaseConfig) IsHelmReleaseCachingEnabled() bool {
	return c.EnableHelmReleaseCache
}
