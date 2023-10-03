package service

type HelmReleaseStatusConfig struct {
	InstallAppVersionHistoryId int
	Message                    string
	IsReleaseInstalled         bool
	ErrorInInstallation        bool
}
