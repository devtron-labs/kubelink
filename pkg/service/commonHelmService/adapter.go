package commonHelmService

import (
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func GetObjectIdentifierFromHelmManifest(manifest unstructured.Unstructured, namespace string) *client.ObjectIdentifier {
	gvk := manifest.GroupVersionKind()
	namespaceManifest := manifest.GetNamespace()
	if namespaceManifest == "" {
		namespaceManifest = namespace
	}
	return &client.ObjectIdentifier{
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
		Name:      manifest.GetName(),
		Namespace: namespaceManifest,
	}
}
