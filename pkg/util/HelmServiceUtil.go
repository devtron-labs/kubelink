package util

import (
	"fmt"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/devtron-labs/kubelink/bean"
	"helm.sh/helm/v3/pkg/release"
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
func BuildAppHealthStatus(nodes []*bean.ResourceNode) *bean.HealthStatusCode {
	appHealthStatus := bean.HealthStatusHealthy
	isAppFullyHibernated := true
	var isAppPartiallyHibernated bool
	var isAnyNodeCanByHibernated bool

	for _, node := range nodes {
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
		if health.IsWorse(health.HealthStatusCode(appHealthStatus), health.HealthStatusCode(nodeHealth.Status)) {
			appHealthStatus = nodeHealth.Status
		}
	}

	// override hibernate status on app level if status is healthy and hibernation done
	if appHealthStatus == bean.HealthStatusHealthy && isAnyNodeCanByHibernated {
		if isAppFullyHibernated {
			appHealthStatus = bean.HealthStatusHibernated
		} else if isAppPartiallyHibernated {
			appHealthStatus = bean.HealthStatusPartiallyHibernated
		}
	}

	return &appHealthStatus
}
