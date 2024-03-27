package k8sInformer

import "helm.sh/helm/v3/pkg/release"

type ReleaseDto struct {
	*release.Release
}

func (r *ReleaseDto) getUniqueReleaseIdentifier() string {
	return r.Namespace + "_" + r.Name
}
