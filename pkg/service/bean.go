package service

import "k8s.io/apimachinery/pkg/api/meta"

type HelmReleaseStatusConfig struct {
	InstallAppVersionHistoryId int
	Message                    string
	IsReleaseInstalled         bool
	ErrorInInstallation        bool
}

type ParentChildGvkMapping struct {
	Group        string         `json:"group"`
	Version      string         `json:"version"`
	Kind         string         `json:"kind"`
	ChildObjects []ChildObjects `json:"childObjects"`
}

type ChildObjects struct {
	Group    string             `json:"group"`
	Version  string             `json:"version"`
	Resource string             `json:"resource"`
	Scope    meta.RESTScopeName `json:"scope"`
}
