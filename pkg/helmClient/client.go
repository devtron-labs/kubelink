package helmClient

import (
	"context"
	"errors"
	"fmt"
	"github.com/devtron-labs/kubelink/pkg/util"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/helmpath"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"io/ioutil"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"strings"
)

var storage = repo.File{}

const (
	defaultCachePath            = "/devtroncd/.helmcache"
	defaultRepositoryConfigPath = "/devtroncd/.helmrepo"
)

// NewClientFromRestConf returns a new Helm client constructed with the provided REST config options
func NewClientFromRestConf(options *RestConfClientOptions) (Client, error) {
	settings := cli.New()

	clientGetter := NewRESTClientGetter(options.Namespace, nil, options.RestConfig)

	err := setEnvSettings(options.Options, settings)
	if err != nil {
		return nil, err
	}

	return newClient(options.Options, clientGetter, settings)
}

// newClient returns a new Helm client via the provided options and REST config
func newClient(options *Options, clientGetter genericclioptions.RESTClientGetter, settings *cli.EnvSettings) (Client, error) {
	err := setEnvSettings(options, settings)
	if err != nil {
		return nil, err
	}

	debugLog := options.DebugLog
	if debugLog == nil {
		debugLog = func(format string, v ...interface{}) {
			log.Printf(format, v...)
		}
	}

	actionConfig := new(action.Configuration)
	err = actionConfig.Init(
		clientGetter,
		options.Namespace,
		os.Getenv("HELM_DRIVER"),
		debugLog,
	)
	if err != nil {
		return nil, err
	}

	return &HelmClient{
		Settings:     settings,
		Providers:    getter.All(settings),
		storage:      &storage,
		ActionConfig: actionConfig,
		linting:      options.Linting,
		DebugLog:     debugLog,
	}, nil
}

// setEnvSettings sets the client's environment settings based on the provided client configuration
func setEnvSettings(options *Options, settings *cli.EnvSettings) error {
	if options == nil {
		options = &Options{
			RepositoryConfig: defaultRepositoryConfigPath,
			RepositoryCache:  defaultCachePath,
			Linting:          true,
		}
	}

	// set the namespace with this ugly workaround because cli.EnvSettings.namespace is private
	// thank you helm!
	/*if options.Namespace != "" {
		pflags := pflag.NewFlagSet("", pflag.ContinueOnError)
		settings.AddFlags(pflags)
		err := pflags.Parse([]string{"-n", options.Namespace})
		if err != nil {
			return err
		}
	}*/

	if options.RepositoryConfig == "" {
		options.RepositoryConfig = defaultRepositoryConfigPath
	}

	if options.RepositoryCache == "" {
		options.RepositoryCache = defaultCachePath
	}

	settings.RepositoryCache = options.RepositoryCache
	settings.RepositoryConfig = options.RepositoryConfig
	settings.Debug = options.Debug

	return nil
}

// ListDeployedReleases lists all deployed releases.
// Namespace and other context is provided via the Options struct when instantiating a client.
func (c *HelmClient) ListDeployedReleases() ([]*release.Release, error) {
	return c.listDeployedReleases()
}

// GetRelease returns a release specified by name.
func (c *HelmClient) GetRelease(name string) (*release.Release, error) {
	return c.getRelease(name)
}

// ListReleaseHistory lists the last 'max' number of entries
// in the history of the release identified by 'name'.
func (c *HelmClient) ListReleaseHistory(name string, max int) ([]*release.Release, error) {
	client := action.NewHistory(c.ActionConfig)

	client.Max = max

	return client.Run(name)
}

func (c *HelmClient) ListAllReleases() ([]*release.Release, error) {
	return c.listAllReleases()
}

// UninstallReleaseByName uninstalls a release identified by the provided 'name'.
func (c *HelmClient) UninstallReleaseByName(name string) error {
	return c.uninstallReleaseByName(name)
}

// UpgradeRelease upgrades the provided chart and returns the corresponding release.
// Namespace and other context is provided via the helmclient.Options struct when instantiating a client.
func (c *HelmClient) UpgradeRelease(ctx context.Context, chart *chart.Chart, updatedChartSpec *ChartSpec) (*release.Release, error) {
	return c.upgrade(ctx, chart, updatedChartSpec)
}

// AddOrUpdateChartRepo adds or updates the provided helm chart repository.
func (c *HelmClient) AddOrUpdateChartRepo(entry repo.Entry) error {
	chartRepo, err := repo.NewChartRepository(&entry, c.Providers)
	if err != nil {
		return err
	}

	chartRepo.CachePath = c.Settings.RepositoryCache

	_, err = DownloadIndexFile(chartRepo)
	if err != nil {
		return err
	}

	/*if c.storage.Has(entry.Name) {
		// repository name already exists
		return nil
	}*/

	c.storage.Update(&entry)
	err = c.storage.WriteFile(c.Settings.RepositoryConfig, 0o644)
	if err != nil {
		return err
	}

	return nil
}

// InstallChart installs the provided chart and returns the corresponding release.
// Namespace and other context is provided via the helmclient.Options struct when instantiating a client.
func (c *HelmClient) InstallChart(ctx context.Context, spec *ChartSpec) (*release.Release, error) {
	// if not dry-run, then only check if the release is installed or not
	if !spec.DryRun {
		installed, err := c.chartIsInstalled(spec.ReleaseName, spec.Namespace)
		if err != nil {
			return nil, err
		}

		if installed {
			errorMessage := fmt.Sprintf("release already exists with releaseName : %s in namespace : %s", spec.ReleaseName, spec.Namespace)
			return nil, errors.New(errorMessage)
		}
	}
	return c.install(ctx, spec)
}

// UpgradeReleaseWithChartInfo installs the provided chart and returns the corresponding release.
// Namespace and other context is provided via the helmclient.Options struct when instantiating a client.
func (c *HelmClient) UpgradeReleaseWithChartInfo(ctx context.Context, spec *ChartSpec) (*release.Release, error) {
	return c.upgradeWithChartInfo(ctx, spec)
}

func (c *HelmClient) IsReleaseInstalled(ctx context.Context, releaseName string, releaseNamespace string) (bool, error) {
	installed, err := c.chartIsInstalled(releaseName, releaseNamespace)
	return installed, err
}

// RollbackRelease rollbacks a release to a specific version.
// Specifying '0' as the value for 'version' will result in a rollback to the previous release version.
func (c *HelmClient) RollbackRelease(spec *ChartSpec, version int) error {
	return c.rollbackRelease(spec, version)
}

// listDeployedReleases lists all deployed helm releases.
func (c *HelmClient) listDeployedReleases() ([]*release.Release, error) {
	listClient := action.NewList(c.ActionConfig)

	listClient.StateMask = action.ListDeployed

	return listClient.Run()
}

// getRelease returns a release matching the provided 'name'.
func (c *HelmClient) getRelease(name string) (*release.Release, error) {
	getReleaseClient := action.NewGet(c.ActionConfig)

	return getReleaseClient.Run(name)
}

func (c *HelmClient) listAllReleases() ([]*release.Release, error) {
	listClient := action.NewList(c.ActionConfig)
	listClient.StateMask = action.ListAll
	listClient.AllNamespaces = true
	listClient.All = true
	return listClient.Run()
}

// uninstallReleaseByName uninstalls a release identified by the provided 'name'.
func (c *HelmClient) uninstallReleaseByName(name string) error {
	client := action.NewUninstall(c.ActionConfig)

	_, err := client.Run(name)
	if err != nil {
		return err
	}

	return nil
}

// upgrade upgrades a chart and CRDs.
// Optionally lints the chart if the linting flag is set.
func (c *HelmClient) upgrade(ctx context.Context, helmChart *chart.Chart, updatedChartSpec *ChartSpec) (*release.Release, error) {
	client := action.NewUpgrade(c.ActionConfig)
	copyUpgradeOptions(updatedChartSpec, client)

	if client.Version == "" {
		client.Version = ">0.0.0-0"
	}

	if req := helmChart.Metadata.Dependencies; req != nil {
		if err := action.CheckDependencies(helmChart, req); err != nil {
			return nil, err
		}
	}

	values, err := getValuesMap(updatedChartSpec)
	if err != nil {
		return nil, err
	}

	release, err := client.RunWithContext(ctx, updatedChartSpec.ReleaseName, helmChart, values)
	if err != nil {
		return nil, err
	}

	return release, nil
}

// upgradeWithChartInfo upgrades a chart and CRDs.
// Optionally lints the chart if the linting flag is set.
func (c *HelmClient) upgradeWithChartInfo(ctx context.Context, spec *ChartSpec) (*release.Release, error) {
	client := action.NewUpgrade(c.ActionConfig)
	copyUpgradeOptions(spec, client)

	if client.Version == "" {
		client.Version = ">0.0.0-0"
	}

	helmChart, _, err := c.getChart(spec.ChartName, &client.ChartPathOptions)
	if err != nil {
		return nil, err
	}

	if req := helmChart.Metadata.Dependencies; req != nil {
		if err := action.CheckDependencies(helmChart, req); err != nil {
			return nil, err
		}
	}

	values, err := getValuesMap(spec)
	if err != nil {
		return nil, err
	}

	release, err := client.RunWithContext(ctx, spec.ReleaseName, helmChart, values)
	if err != nil {
		return nil, err
	}

	return release, nil
}

// copyUpgradeOptions merges values of the provided chart to helm upgrade options used by the client.
func copyUpgradeOptions(chartSpec *ChartSpec, upgradeOptions *action.Upgrade) {
	upgradeOptions.Version = chartSpec.Version
	upgradeOptions.Namespace = chartSpec.Namespace
	upgradeOptions.Timeout = chartSpec.Timeout
	upgradeOptions.Wait = chartSpec.Wait
	upgradeOptions.DisableHooks = chartSpec.DisableHooks
	upgradeOptions.Force = chartSpec.Force
	upgradeOptions.ResetValues = chartSpec.ResetValues
	upgradeOptions.ReuseValues = chartSpec.ReuseValues
	upgradeOptions.Recreate = chartSpec.Recreate
	upgradeOptions.MaxHistory = chartSpec.MaxHistory
	upgradeOptions.Atomic = chartSpec.Atomic
	upgradeOptions.CleanupOnFail = chartSpec.CleanupOnFail
	upgradeOptions.DryRun = chartSpec.DryRun
	upgradeOptions.SubNotes = chartSpec.SubNotes
	upgradeOptions.DisableOpenAPIValidation = true
}

// copyInstallOptions merges values of the provided chart to helm install options used by the client.
func copyInstallOptions(chartSpec *ChartSpec, installOptions *action.Install) {
	installOptions.CreateNamespace = chartSpec.CreateNamespace
	installOptions.DisableHooks = chartSpec.DisableHooks
	installOptions.Replace = chartSpec.Replace
	installOptions.Wait = chartSpec.Wait
	installOptions.DependencyUpdate = chartSpec.DependencyUpdate
	installOptions.Timeout = chartSpec.Timeout
	installOptions.Namespace = chartSpec.Namespace
	installOptions.ReleaseName = chartSpec.ReleaseName
	installOptions.Version = chartSpec.Version
	installOptions.GenerateName = chartSpec.GenerateName
	installOptions.NameTemplate = chartSpec.NameTemplate
	installOptions.Atomic = chartSpec.Atomic
	installOptions.SkipCRDs = chartSpec.SkipCRDs
	installOptions.DryRun = chartSpec.DryRun
	installOptions.SubNotes = chartSpec.SubNotes
	installOptions.DisableOpenAPIValidation = true
}

// getChart returns a chart matching the provided chart name and options.
func (c *HelmClient) getChart(chartName string, chartPathOptions *action.ChartPathOptions) (*chart.Chart, string, error) {
	chartPath, err := chartPathOptions.LocateChart(chartName, c.Settings)
	if err != nil {
		return nil, "", err
	}

	helmChart, err := loader.Load(chartPath)
	if err != nil {
		return nil, "", err
	}

	return helmChart, chartPath, err
}

// lint lints a chart's values.
func (c *HelmClient) lint(chartPath string, values map[string]interface{}) error {
	client := action.NewLint()

	result := client.Run([]string{chartPath}, values)

	for _, err := range result.Errors {
		c.DebugLog("Error %s", err)
	}

	if len(result.Errors) > 0 {
		return fmt.Errorf("linting for chartpath %q failed", chartPath)
	}

	return nil
}

func getValuesMap(spec *ChartSpec) (map[string]interface{}, error) {
	var values map[string]interface{}

	err := yaml.Unmarshal([]byte(spec.ValuesYaml), &values)
	if err != nil {
		return nil, err
	}

	return values, nil
}

// chartIsInstalled checks whether a chart is already installed
// in a namespace or not based on the provided chart spec.
// Note that this function only considers the contained chart name and namespace.
func (c *HelmClient) chartIsInstalled(releaseName string, releaseNamespace string) (bool, error) {
	releases, err := c.listDeployedReleases()
	if err != nil {
		return false, err
	}

	for _, r := range releases {
		if r.Name == releaseName && r.Namespace == releaseNamespace {
			return true, nil
		}
	}

	return false, nil
}

// install installs the provided chart.
// Optionally lints the chart if the linting flag is set.
func (c *HelmClient) install(ctx context.Context, spec *ChartSpec) (*release.Release, error) {
	client := action.NewInstall(c.ActionConfig)
	copyInstallOptions(spec, client)

	if client.Version == "" {
		client.Version = ">0.0.0-0"
	}

	helmChart, chartPath, err := c.getChart(spec.ChartName, &client.ChartPathOptions)
	if err != nil {
		return nil, err
	}

	if helmChart.Metadata.Type != "" && helmChart.Metadata.Type != "application" {
		return nil, fmt.Errorf(
			"chart %q has an unsupported type and is not installable: %q",
			helmChart.Metadata.Name,
			helmChart.Metadata.Type,
		)
	}

	if req := helmChart.Metadata.Dependencies; req != nil {
		if err := action.CheckDependencies(helmChart, req); err != nil {
			if client.DependencyUpdate {
				man := &downloader.Manager{
					ChartPath:        chartPath,
					Keyring:          client.ChartPathOptions.Keyring,
					SkipUpdate:       false,
					Getters:          c.Providers,
					RepositoryConfig: c.Settings.RepositoryConfig,
					RepositoryCache:  c.Settings.RepositoryCache,
				}
				if err := man.Update(); err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
	}

	_, err = getValuesMap(spec)
	if err != nil {
		return nil, err
	}

	/*rel, err := client.RunWithContext(ctx, helmChart, values)
	if err != nil {
		return rel, err
	}*/

	fmt.Println("ignoring")

	return nil, nil
}

// rollbackRelease rolls back a release matching the ChartSpec 'spec' to a specific version.
// Specifying version = 0 will roll back a release to the latest revision.
func (c *HelmClient) rollbackRelease(spec *ChartSpec, version int) error {
	client := action.NewRollback(c.ActionConfig)

	copyRollbackOptions(spec, client)

	client.Version = version

	return client.Run(spec.ReleaseName)
}

// copyRollbackOptions merges values of the provided chart to helm rollback options used by the client.
func copyRollbackOptions(chartSpec *ChartSpec, rollbackOptions *action.Rollback) {
	rollbackOptions.DisableHooks = chartSpec.DisableHooks
	rollbackOptions.DryRun = chartSpec.DryRun
	rollbackOptions.Timeout = chartSpec.Timeout
	rollbackOptions.CleanupOnFail = chartSpec.CleanupOnFail
	rollbackOptions.Force = chartSpec.Force
	rollbackOptions.MaxHistory = chartSpec.MaxHistory
	rollbackOptions.Recreate = chartSpec.Recreate
	rollbackOptions.Wait = chartSpec.Wait
}

// DownloadIndexFile fetches the index from a repository.
func DownloadIndexFile(chartRepo *repo.ChartRepository) (string, error) {
	parsedURL, err := url.Parse(chartRepo.Config.URL)
	if err != nil {
		return "", err
	}
	parsedURL.RawPath = path.Join(parsedURL.RawPath, "index.yaml")
	parsedURL.Path = path.Join(parsedURL.Path, "index.yaml")

	indexURL := parsedURL.String()

	index, err := util.GetFromUrlWithRetry(indexURL)

	if err != nil {
		return "", err
	}

	indexFile, err := loadIndex(index, chartRepo.Config.URL)
	if err != nil {
		return "", err
	}

	// Create the chart list file in the cache directory
	var charts strings.Builder
	for name := range indexFile.Entries {
		fmt.Fprintln(&charts, name)
	}
	chartsFile := filepath.Join(chartRepo.CachePath, helmpath.CacheChartsFile(chartRepo.Config.Name))
	os.MkdirAll(filepath.Dir(chartsFile), 0755)
	ioutil.WriteFile(chartsFile, []byte(charts.String()), 0644)

	// Create the index file in the cache directory
	fname := filepath.Join(chartRepo.CachePath, helmpath.CacheIndexFile(chartRepo.Config.Name))
	os.MkdirAll(filepath.Dir(fname), 0755)

	err = ioutil.WriteFile(fname, index, 0644)
	if err != nil {
		return "", err
	}

	// cleanup
	index = nil
	indexFile = nil

	return fname, nil
}

// loadIndex loads an index file and does minimal validity checking.
//
// The source parameter is only used for logging.
// This will fail if API Version is not set (ErrNoAPIVersion) or if the unmarshal fails.
func loadIndex(data []byte, source string) (*repo.IndexFile, error) {
	i := &repo.IndexFile{}

	if len(data) == 0 {
		return i, repo.ErrEmptyIndexYaml
	}

	if err := yaml.UnmarshalStrict(data, i); err != nil {
		return i, err
	}

	return i, nil
}
