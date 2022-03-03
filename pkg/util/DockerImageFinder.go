package util

import (
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	appsV1 "k8s.io/api/apps/v1"
	batchV1 "k8s.io/api/batch/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func ExtractAllDockerImages(manifests []unstructured.Unstructured) ([]string, error) {
	var dockerImages []string
	for _, manifest := range manifests {
		switch manifest.GroupVersionKind() {
		case schema.GroupVersionKind{Group: "", Version: "v1", Kind: kube.PodKind}:
			var pod coreV1.Pod
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &pod)
			if err != nil {
				return nil, err
			}
			dockerImages = append(dockerImages, extractImagesFromPodTemplate(pod.Spec)...)
		case schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kube.DeploymentKind}:
			var deployment appsV1.Deployment
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &deployment)
			if err != nil {
				return nil, err
			}
			dockerImages = append(dockerImages, extractImagesFromPodTemplate(deployment.Spec.Template.Spec)...)
		case schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kube.ReplicaSetKind}:
			var replicaSet appsV1.ReplicaSet
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &replicaSet)
			if err != nil {
				return nil, err
			}
			dockerImages = append(dockerImages, extractImagesFromPodTemplate(replicaSet.Spec.Template.Spec)...)
		case schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kube.StatefulSetKind}:
			var statefulSet appsV1.StatefulSet
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &statefulSet)
			if err != nil {
				return nil, err
			}
			dockerImages = append(dockerImages, extractImagesFromPodTemplate(statefulSet.Spec.Template.Spec)...)
		case schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kube.DaemonSetKind}:
			var daemonSet appsV1.DaemonSet
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &daemonSet)
			if err != nil {
				return nil, err
			}
			dockerImages = append(dockerImages, extractImagesFromPodTemplate(daemonSet.Spec.Template.Spec)...)
		case schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: kube.JobKind}:
			var job batchV1.Job
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &job)
			if err != nil {
				return nil, err
			}
			dockerImages = append(dockerImages, extractImagesFromPodTemplate(job.Spec.Template.Spec)...)
		case schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}:
			var cronJob batchV1.CronJob
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &cronJob)
			if err != nil {
				return nil, err
			}
			dockerImages = append(dockerImages, extractImagesFromPodTemplate(cronJob.Spec.JobTemplate.Spec.Template.Spec)...)
		case schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ReplicationController"}:
			var replicationController coreV1.ReplicationController
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &replicationController)
			if err != nil {
				return nil, err
			}
			dockerImages = append(dockerImages, extractImagesFromPodTemplate(replicationController.Spec.Template.Spec)...)
		}
	}

	return dockerImages, nil
}

func extractImagesFromPodTemplate(podSpec coreV1.PodSpec) []string {
	var dockerImages []string
	for _, container := range podSpec.Containers {
		dockerImages = append(dockerImages, container.Image)
	}
	for _, initContainer := range podSpec.InitContainers {
		dockerImages = append(dockerImages, initContainer.Image)
	}
	return dockerImages
}
