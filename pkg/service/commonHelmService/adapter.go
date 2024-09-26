package commonHelmService

import (
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func GetClientGVKFromSchemaGVK(gvk schema.GroupVersionKind) *client.Gvk {
	return &client.Gvk{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind,
	}
}
