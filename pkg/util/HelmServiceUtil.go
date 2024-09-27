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

package util

import (
	"errors"
	"fmt"
	"github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/common-lib/utils/k8s/health"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// GetAppId returns AppID by logic  cluster_id|namespace|release_name
func GetAppId(clusterId int32, release *release.Release) string {
	return fmt.Sprintf("%d|%s|%s", clusterId, release.Namespace, release.Name)
}

func GetMessageFromReleaseStatus(releaseStatus release.Status) string {
	switch releaseStatus {
	case release.StatusUnknown:
		return "The release is in an uncertain state"
	case release.StatusDeployed:
		return "The release has been pushed to Kubernetes"
	case release.StatusUninstalled:
		return "The release has been uninstalled from Kubernetes"
	case release.StatusSuperseded:
		return "The release object is outdated and a newer one exists"
	case release.StatusFailed:
		return "The release was not successfully deployed"
	case release.StatusUninstalling:
		return "The release uninstall operation is underway"
	case release.StatusPendingInstall:
		return "The release install operation is underway"
	case release.StatusPendingUpgrade:
		return "The release upgrade operation is underway"
	case release.StatusPendingRollback:
		return "The release rollback operation is underway"
	default:
		fmt.Println("un handled release status", releaseStatus)
	}

	return ""
}

// app health is worst of the nodes health
// or if app status is healthy then check for hibernation status
func BuildAppHealthStatus(nodes []*commonBean.ResourceNode) *commonBean.HealthStatusCode {
	appHealthStatus := commonBean.HealthStatusHealthy
	isAppFullyHibernated := true
	var isAppPartiallyHibernated bool
	var isAnyNodeCanByHibernated bool

	for _, node := range nodes {
		if node.IsHook {
			continue
		}
		nodeHealth := node.Health
		if node.CanBeHibernated {
			isAnyNodeCanByHibernated = true
			if !node.IsHibernated {
				isAppFullyHibernated = false
			} else {
				isAppPartiallyHibernated = true
			}
		}
		if nodeHealth == nil {
			continue
		}
		if health.IsWorseStatus(health.HealthStatusCode(appHealthStatus), health.HealthStatusCode(nodeHealth.Status)) {
			appHealthStatus = nodeHealth.Status
		}
	}

	// override hibernate status on app level if status is healthy and hibernation done
	if appHealthStatus == commonBean.HealthStatusHealthy && isAnyNodeCanByHibernated {
		if isAppFullyHibernated {
			appHealthStatus = commonBean.HealthStatusHibernated
		} else if isAppPartiallyHibernated {
			appHealthStatus = commonBean.HealthStatusPartiallyHibernated
		}
	}

	return &appHealthStatus
}

func GetAppStatusOnBasisOfHealthyNonHealthy(healthStatusArray []*commonBean.HealthStatus) *commonBean.HealthStatusCode {
	appHealthStatus := commonBean.HealthStatusHealthy
	for _, node := range healthStatusArray {
		nodeHealth := node
		if nodeHealth == nil {
			continue
		}
		//if any node's health is worse than healthy then we break the loop and return
		if health.IsWorseStatus(health.HealthStatusCode(appHealthStatus), health.HealthStatusCode(nodeHealth.Status)) {
			appHealthStatus = nodeHealth.Status
			break
		}
	}
	return &appHealthStatus
}

func IsReleaseNotFoundError(err error) bool {
	return errors.Is(err, driver.ErrReleaseNotFound) || errors.Is(err, driver.ErrNoDeployedReleases)
}
