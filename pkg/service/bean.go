package service

import (
	"github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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

func (r ParentChildGvkMapping) GetParentGvk() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   r.Group,
		Version: r.Version,
		Kind:    r.Kind,
	}
}

type ChildObjects struct {
	Group    string             `json:"group"`
	Version  string             `json:"version"`
	Resource string             `json:"resource"`
	Scope    meta.RESTScopeName `json:"scope"`
}

func (r ChildObjects) GetGvrAndScopeForChildObject() commonBean.GvrAndScope {
	return commonBean.GvrAndScope{
		Gvr: schema.GroupVersionResource{
			Group:    r.Group,
			Version:  r.Version,
			Resource: r.Resource,
		},
		Scope: r.Scope,
	}
}
