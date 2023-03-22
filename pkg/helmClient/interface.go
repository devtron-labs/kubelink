package helmClient

import (
	"context"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
)

//go:generate mockgen -source=interface.go -package mockhelmclient -destination=./mock/interface.go -self_package=. Client

// Client holds the method signatures for a Helm client.
type Client interface {
	ListDeployedReleases() ([]*release.Release, error)
	GetRelease(name string) (*release.Release, error)
	ListReleaseHistory(name string, max int) ([]*release.Release, error)
	ListAllReleases() ([]*release.Release, error)
	UninstallReleaseByName(name string) error
	UpgradeRelease(ctx context.Context, chart *chart.Chart, updatedChartSpec *ChartSpec) (*release.Release, error)
	AddOrUpdateChartRepo(entry repo.Entry) error
	InstallChart(ctx context.Context, spec *ChartSpec) (*release.Release, error)
	UpgradeReleaseWithChartInfo(ctx context.Context, spec *ChartSpec) (*release.Release, error)
	IsReleaseInstalled(ctx context.Context, releaseName string, releaseNamespace string) (bool, error)
	RollbackRelease(spec *ChartSpec, version int) error
	TemplateChart(spec *ChartSpec, options *HelmTemplateOptions) ([]byte, error)
	GetNotes(spec *ChartSpec, options *HelmTemplateOptions) ([]byte, error)
}
