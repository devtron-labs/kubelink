package argo

import (
	"fmt"
	"github.com/devtron-labs/kubelink/bean"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func PopulatePodInfo(un *unstructured.Unstructured) ([]bean.InfoItem, error) {
	var infoItems []bean.InfoItem

	pod := v1.Pod{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, &pod)
	if err != nil {
		return nil, err
	}
	restarts := 0
	totalContainers := len(pod.Spec.Containers)
	readyContainers := 0

	reason := string(pod.Status.Phase)
	if pod.Status.Reason != "" {
		reason = pod.Status.Reason
	}

	initializing := false
	for i := range pod.Status.InitContainerStatuses {
		container := pod.Status.InitContainerStatuses[i]
		restarts += int(container.RestartCount)
		switch {
		case container.State.Terminated != nil && container.State.Terminated.ExitCode == 0:
			continue
		case container.State.Terminated != nil:
			// initialization is failed
			if len(container.State.Terminated.Reason) == 0 {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Init:Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("Init:ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else {
				reason = "Init:" + container.State.Terminated.Reason
			}
			initializing = true
		case container.State.Waiting != nil && len(container.State.Waiting.Reason) > 0 && container.State.Waiting.Reason != "PodInitializing":
			reason = "Init:" + container.State.Waiting.Reason
			initializing = true
		default:
			reason = fmt.Sprintf("Init:%d/%d", i, len(pod.Spec.InitContainers))
			initializing = true
		}
		break
	}
	if !initializing {
		restarts = 0
		hasRunning := false
		for i := len(pod.Status.ContainerStatuses) - 1; i >= 0; i-- {
			container := pod.Status.ContainerStatuses[i]

			restarts += int(container.RestartCount)
			if container.State.Waiting != nil && container.State.Waiting.Reason != "" {
				reason = container.State.Waiting.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason != "" {
				reason = container.State.Terminated.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason == "" {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else if container.Ready && container.State.Running != nil {
				hasRunning = true
				readyContainers++
			}
		}

		// change pod status back to "Running" if there is at least one container still reporting as "Running" status
		if reason == "Completed" && hasRunning {
			reason = "Running"
		}
	}

	// "NodeLost" = https://github.com/kubernetes/kubernetes/blob/cb8ad64243d48d9a3c26b11b2e0945c098457282/pkg/util/node/node.go#L46
	// But depending on the k8s.io/kubernetes package just for a constant
	// is not worth it.
	// See https://github.com/argoproj/argo-cd/issues/5173
	// and https://github.com/kubernetes/kubernetes/issues/90358#issuecomment-617859364
	if pod.DeletionTimestamp != nil && pod.Status.Reason == "NodeLost" {
		reason = "Unknown"
	} else if pod.DeletionTimestamp != nil {
		reason = "Terminating"
	}
	infoItems = getAllInfoItems(infoItems, reason, restarts, readyContainers, totalContainers, pod)
	return infoItems, nil
}

func getAllInfoItems(infoItems []bean.InfoItem, reason string, restarts int, readyContainers int, totalContainers int, pod v1.Pod) []bean.InfoItem {
	if reason != "" {
		infoItems = append(infoItems, bean.InfoItem{Name: "Status Reason", Value: reason})
	}
	infoItems = append(infoItems, bean.InfoItem{Name: "Node", Value: pod.Spec.NodeName})

	containerNames, initContainerNames, ephemeralContainersInfo, ephemeralContainerStatus := getContainersInfo(pod)

	infoItems = append(infoItems, bean.InfoItem{Name: bean.ContainersType, Value: fmt.Sprintf("%d/%d", readyContainers, totalContainers), ContainerNames: containerNames})
	infoItems = append(infoItems, bean.InfoItem{Name: bean.InitContainersType, InitContainerNames: initContainerNames})
	infoItems = append(infoItems, bean.InfoItem{Name: bean.EphemeralContainersType, EphemeralContainersInfo: ephemeralContainersInfo, EphemeralContainerStatuses: ephemeralContainerStatus})
	if restarts > 0 {
		infoItems = append(infoItems, bean.InfoItem{Name: "Restart Count", Value: fmt.Sprintf("%d", restarts)})
	}
	return infoItems
}

func getContainersInfo(pod v1.Pod) ([]string, []string, []bean.EphemeralContainerInfo, []bean.EphemeralContainerStatusesInfo) {
	containerNames := make([]string, 0, len(pod.Spec.Containers))
	initContainerNames := make([]string, 0, len(pod.Spec.InitContainers))
	ephemeralContainers := make([]bean.EphemeralContainerInfo, 0, len(pod.Spec.EphemeralContainers))
	ephemeralContainerStatus := make([]bean.EphemeralContainerStatusesInfo, 0, len(pod.Status.EphemeralContainerStatuses))
	for _, container := range pod.Spec.Containers {
		containerNames = append(containerNames, container.Name)
	}
	for _, initContainer := range pod.Spec.InitContainers {
		initContainerNames = append(initContainerNames, initContainer.Name)
	}
	for _, ec := range pod.Spec.EphemeralContainers {
		ecData := bean.EphemeralContainerInfo{
			Name:    ec.Name,
			Command: ec.Command,
		}
		ephemeralContainers = append(ephemeralContainers, ecData)
	}
	for _, ecStatus := range pod.Status.EphemeralContainerStatuses {
		status := bean.EphemeralContainerStatusesInfo{
			Name:  ecStatus.Name,
			State: ecStatus.State,
		}
		ephemeralContainerStatus = append(ephemeralContainerStatus, status)
	}
	return containerNames, initContainerNames, ephemeralContainers, ephemeralContainerStatus
}
