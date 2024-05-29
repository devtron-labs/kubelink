package fluxInformer

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

type FluxAppDto struct {
	*unstructured.Unstructured
}

func (a *FluxAppDto) getUniqueAppIdentifier() string {
	return a.GetNamespace() + "_" + a.GetName()
}
