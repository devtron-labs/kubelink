package service

type HelmInstallNatsMessage struct {
	InstallAppVersionHistoryId int
	Message                    string
	IsReleaseInstalled         bool
}
