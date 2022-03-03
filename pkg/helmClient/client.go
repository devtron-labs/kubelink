package helmClient

import (
	"context"
	"errors"
	"fmt"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"log"
	"os"
	"sigs.k8s.io/yaml"
)

var storage = repo.File{}

const (
	defaultCachePath            = "/tmp/.helmcache"
	defaultRepositoryConfigPath = "/tmp/.helmrepo"
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

	_, err = chartRepo.DownloadIndexFile()
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
	installed, err := c.chartIsInstalled(spec)
	if err != nil {
		return nil, err
	}

	if installed {
		errorMessage := fmt.Sprintf("release already exists with releaseName : %s in namespace : %s", spec.ReleaseName, spec.Namespace)
		return nil, errors.New(errorMessage)
	}

	return c.install(ctx, spec)
}

// UpgradeReleaseWithChartInfo installs the provided chart and returns the corresponding release.
// Namespace and other context is provided via the helmclient.Options struct when instantiating a client.
func (c *HelmClient) UpgradeReleaseWithChartInfo(ctx context.Context, spec *ChartSpec) (*release.Release, error) {
	return c.upgradeWithChartInfo(ctx, spec)
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
	mergeUpgradeOptions(updatedChartSpec, client)

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
	mergeUpgradeOptions(spec, client)

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

// mergeUpgradeOptions merges values of the provided chart to helm upgrade options used by the client.
func mergeUpgradeOptions(chartSpec *ChartSpec, upgradeOptions *action.Upgrade) {
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
}

// mergeInstallOptions merges values of the provided chart to helm install options used by the client.
func mergeInstallOptions(chartSpec *ChartSpec, installOptions *action.Install) {
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
func (c *HelmClient) chartIsInstalled(spec *ChartSpec) (bool, error) {
	releases, err := c.listDeployedReleases()
	if err != nil {
		return false, err
	}

	for _, r := range releases {
		if r.Name == spec.ReleaseName && r.Namespace == spec.Namespace {
			return true, nil
		}
	}

	return false, nil
}

// install installs the provided chart.
// Optionally lints the chart if the linting flag is set.
func (c *HelmClient) install(ctx context.Context, spec *ChartSpec) (*release.Release, error) {
	client := action.NewInstall(c.ActionConfig)
	mergeInstallOptions(spec, client)

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

	values, err := getValuesMap(spec)
	if err != nil {
		return nil, err
	}

	rel, err := client.RunWithContext(ctx, helmChart, values)
	if err != nil {
		return rel, err
	}

	return rel, nil
}