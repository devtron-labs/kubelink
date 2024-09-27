package commonHelmService

import (
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func GetObjectIdentifierFromHelmManifest(manifest unstructured.Unstructured) *client.ObjectIdentifier {
	gvk := manifest.GroupVersionKind()
	return &client.ObjectIdentifier{
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
		Name:      manifest.GetName(),
		Namespace: manifest.GetNamespace(),
	}
}
