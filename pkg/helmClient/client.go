package helmClient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	error2 "github.com/devtron-labs/kubelink/error"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"log"
	"os"
	"sigs.k8s.io/yaml"
	"strconv"
	"text/template"
	"time"
)

var storage = repo.File{}

const (
	CHART_WORKING_DIR_PATH      = "/tmp/charts/"
	defaultCachePath            = "/home/devtron/devtroncd/.helmcache"
	defaultRepositoryConfigPath = "/home/devtron/devtroncd/.helmrepo"
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
	c.ActionConfig.RegistryClient = updatedChartSpec.RegistryClient
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
	// if not dry-run, then only check if the release is installed or not
	// adding registry client in action config
	c.ActionConfig.RegistryClient = spec.RegistryClient
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

	listClient.StateMask = action.ListDeployed | action.ListPendingInstall | action.ListPendingRollback | action.ListPendingUpgrade | action.ListUnknown | action.ListFailed

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
	c.ActionConfig.RegistryClient = spec.RegistryClient
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
		if error2.IsValidationError(err) {
			err = status.New(codes.InvalidArgument, err.Error()).Err()
			return nil, err
		}
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
func (c *HelmClient) GetNotes(spec *ChartSpec, options *HelmTemplateOptions) ([]byte, error) {
	client := action.NewInstall(c.ActionConfig)
	mergeInstallOptions(spec, client)

	client.DryRun = true
	client.ReleaseName = spec.ReleaseName
	client.Replace = true // Skip the name check
	client.ClientOnly = true
	client.IncludeCRDs = true

	if options != nil {
		client.KubeVersion = options.KubeVersion
		client.APIVersions = options.APIVersions
	}

	// NameAndChart returns either the TemplateName if set,
	// the ReleaseName if set or the generatedName as the first return value.
	releaseName, _, err := client.NameAndChart([]string{spec.ChartName})
	if err != nil {
		return nil, err
	}
	client.ReleaseName = releaseName

	if client.Version == "" {
		client.Version = ">0.0.0-0"
	}
	ChartPathOptions := action.ChartPathOptions{
		RepoURL: spec.RepoURL,
		Version: spec.Version,
	}
	client.ChartPathOptions = ChartPathOptions
	helmChart, chartPath, err := c.getChart(spec.ChartName, &client.ChartPathOptions)
	if err != nil {
		fmt.Errorf("error in getting helm chart and chart path for chart %q and repo Url %q",
			spec.ChartName,
			spec.RepoURL,
		)
		return nil, err
	}

	if helmChart.Metadata.Type != "" && helmChart.Metadata.Type != "application" {
		return nil, fmt.Errorf(
			"chart %q has an unsupported type and is not installable: %q",
			helmChart.Metadata.Name,
			helmChart.Metadata.Type,
		)
	}
	helmChart, err = updateDependencies(helmChart, &client.ChartPathOptions, chartPath, c, client.DependencyUpdate, spec)
	if err != nil {
		fmt.Errorf("error in updating dependencies for helm chart %q",
			spec.ChartName,
		)
		return nil, err
	}

	values, err := getValuesMap(spec)
	if err != nil {
		return nil, err
	}

	out := new(bytes.Buffer)
	rel, err := client.Run(helmChart, values)
	if err != nil {
		if _, isExecError := err.(template.ExecError); isExecError {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				fmt.Sprintf("invalid template, err %s", err),
			)
		}
		fmt.Errorf("error in fetching release for helm chart %q and repo Url %q",
			spec.ChartName,
			spec.RepoURL,
		)
		return nil, err
	}
	fmt.Fprintf(out, "%s", rel.Info.Notes)

	return out.Bytes(), err

}

func (c *HelmClient) TemplateChart(spec *ChartSpec, options *HelmTemplateOptions, chartData []byte, returnChartBytes bool) ([]byte, []byte, error) {
	var helmChart *chart.Chart
	var chartPath string
	var chartBytes []byte
	var err error
	if chartData != nil {
		helmChart, err = loader.LoadArchive(bytes.NewReader(chartData))
		if err != nil {
			return nil, chartBytes, err
		}
	}
	c.ActionConfig.RegistryClient = spec.RegistryClient
	client := action.NewInstall(c.ActionConfig)
	mergeInstallOptions(spec, client)

	client.DryRun = true
	client.ReleaseName = spec.ReleaseName
	client.Replace = true // Skip the name check
	client.ClientOnly = true
	client.IncludeCRDs = true

	if options != nil {
		client.KubeVersion = options.KubeVersion
		client.APIVersions = options.APIVersions
	}

	// NameAndChart returns either the TemplateName if set,
	// the ReleaseName if set or the generatedName as the first return value.
	releaseName, _, err := client.NameAndChart([]string{spec.ChartName})
	if err != nil {
		return nil, chartBytes, err
	}
	client.ReleaseName = releaseName

	if client.Version == "" {
		client.Version = ">0.0.0-0"
	}

	// if we are not sending chart from orchestrator, then we have to fetch the chart from helm
	if chartData == nil {
		helmChart, err = c.getChartFromHelm(spec, client, helmChart, chartPath, err)
		if err != nil {
			return nil, chartBytes, err
		}

		if returnChartBytes {
			chartBytes, err = GetChartBytes(helmChart)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	values, err := getValuesMap(spec)
	if err != nil {
		return nil, chartBytes, err
	}

	out := new(bytes.Buffer)
	rel, err := client.Run(helmChart, values)
	if err != nil {
		if _, isExecError := err.(template.ExecError); isExecError {
			return nil, chartBytes, status.Errorf(
				codes.FailedPrecondition,
				fmt.Sprintf("invalid template, err %s", err),
			)
		}
		fmt.Errorf("error in fetching release for helm chart %q and repo Url %q",
			spec.ChartName,
			spec.RepoURL,
		)
		return nil, chartBytes, err
	}

	// We ignore a potential error here because, when the --debug flag was specified,
	// we always want to print the YAML, even if it is not valid. The error is still returned afterwards.
	//if rel != nil {
	//	var manifests bytes.Buffer
	//	fmt.Fprintln(&manifests, strings.TrimSpace(rel.Manifest))
	//	if !client.DisableHooks {
	//		for _, m := range rel.Hooks {
	//			fmt.Fprintf(&manifests, "---\n# Source: %s\n%s\n", m.Path, m.Manifest)
	//		}
	//	}
	//
	//	// if we have a list of files to render, then check that each of the
	//	// provided files exists in the chart.
	//	fmt.Fprintf(out, "%s", manifests.String())
	//}
	fmt.Fprintf(out, "%s", rel.Manifest)

	return out.Bytes(), chartBytes, err
}

func (c *HelmClient) getChartFromHelm(spec *ChartSpec, client *action.Install, helmChart *chart.Chart, chartPath string, err error) (*chart.Chart, error) {
	client.ChartPathOptions.RepoURL = spec.RepoURL
	client.ChartPathOptions.Version = spec.Version
	helmChart, chartPath, err = c.getChart(spec.ChartName, &client.ChartPathOptions)
	if err != nil {
		fmt.Errorf("error in getting helm chart and chart path for chart %q and repo Url %q",
			spec.ChartName,
			spec.RepoURL,
		)
		return nil, err
	}

	if helmChart.Metadata.Type != "" && helmChart.Metadata.Type != "application" {
		return nil, fmt.Errorf(
			"chart %q has an unsupported type and is not installable: %q",
			helmChart.Metadata.Name,
			helmChart.Metadata.Type,
		)
	}
	helmChart, err = updateDependencies(helmChart, &client.ChartPathOptions, chartPath, c, client.DependencyUpdate, spec)
	if err != nil {
		fmt.Errorf("error in updating dependencies for helm chart %q",
			spec.ChartName,
		)
		return nil, err
	}
	return helmChart, nil
}

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
	installOptions.WaitForJobs = chartSpec.WaitForJobs
	installOptions.ChartPathOptions.RepoURL = chartSpec.RepoURL
	installOptions.ChartPathOptions.Version = chartSpec.Version
}

// updateDependencies checks dependencies for given helmChart and updates dependencies with metadata if dependencyUpdate is true. returns updated HelmChart
func updateDependencies(helmChart *chart.Chart, chartPathOptions *action.ChartPathOptions, chartPath string, c *HelmClient, dependencyUpdate bool, spec *ChartSpec) (*chart.Chart, error) {
	if req := helmChart.Metadata.Dependencies; req != nil {
		if err := action.CheckDependencies(helmChart, req); err != nil {
			if dependencyUpdate {
				man := &downloader.Manager{
					ChartPath:        chartPath,
					Keyring:          chartPathOptions.Keyring,
					SkipUpdate:       false,
					Getters:          c.Providers,
					RepositoryConfig: c.Settings.RepositoryConfig,
					RepositoryCache:  c.Settings.RepositoryCache,
					Out:              c.output,
				}
				if err := man.Update(); err != nil {
					return nil, err
				}

				helmChart, _, err = c.getChart(spec.ChartName, chartPathOptions)
				if err != nil {
					return nil, err
				}

			} else {
				return nil, err
			}
		}
	}
	return helmChart, nil
}

func GetChartBytes(helmChart *chart.Chart) ([]byte, error) {
	dirPath := CHART_WORKING_DIR_PATH
	outputChartPathDir := fmt.Sprintf("%s/%s", dirPath, strconv.FormatInt(time.Now().UnixNano(), 16))
	err := os.MkdirAll(outputChartPathDir, os.ModePerm)
	if err != nil {
		return nil, err
	}

	defer func() {
		err := os.RemoveAll(outputChartPathDir)
		if err != nil {
			fmt.Println("error in deleting dir", " dir: ", outputChartPathDir, " err: ", err)
		}
	}()
	absFilePath, err := chartutil.Save(helmChart, outputChartPathDir)
	if err != nil {
		fmt.Println("error in saving chartdata in the destination dir ", " dir : ", outputChartPathDir, " err : ", err)
		return nil, err
	}

	chartBytes, err := os.ReadFile(absFilePath)
	if err != nil {
		fmt.Println("error in reading chartdata from the file ", " filePath : ", absFilePath, " err : ", err)
	}

	return chartBytes, nil
}
