// Copyright 2019 Rob Scott
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package capacity

import (
	"fmt"
	"os"

	"github.com/robscott/kube-capacity/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// List gathers cluster resource data and outputs it
func List(args []string, showPods bool, showUtil bool) {
	podList, nodeList := getPodsAndNodes()
	pmList := &v1beta1.PodMetricsList{}
	nmList := &v1beta1.NodeMetricsList{}
	if showUtil {
		pmList, nmList = getMetrics()
	}
	cm := buildClusterMetric(podList, pmList, nodeList, nmList)
	printList(&cm, showPods, showUtil)
}

func getPodsAndNodes() (*corev1.PodList, *corev1.NodeList) {
	clientset, err := kube.NewClientSet()
	if err != nil {
		fmt.Printf("Error connecting to Kubernetes: %v\n", err)
		os.Exit(1)
	}

	nodeList, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error listing Nodes: %v\n", err)
		os.Exit(2)
	}

	podList, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error listing Pods: %v\n", err)
		os.Exit(3)
	}

	return podList, nodeList
}

func getMetrics() (*v1beta1.PodMetricsList, *v1beta1.NodeMetricsList) {
	mClientset, err := kube.NewMetricsClientSet()
	if err != nil {
		fmt.Printf("Error connecting to Metrics API: %v\n", err)
		os.Exit(4)
	}

	nmList, err := mClientset.MetricsV1beta1().NodeMetricses().List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error getting Node Metrics: %v\n", err)
		fmt.Println("For this to work, metrics-server needs to be running in your cluster")
		os.Exit(5)
	}

	pmList, err := mClientset.MetricsV1beta1().PodMetricses("").List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error getting Pod Metrics: %v\n", err)
		fmt.Println("For this to work, metrics-server needs to be running in your cluster")
		os.Exit(6)
	}

	return pmList, nmList
}

func buildClusterMetric(podList *corev1.PodList, pmList *v1beta1.PodMetricsList, nodeList *corev1.NodeList, nmList *v1beta1.NodeMetricsList) clusterMetric {
	cm := clusterMetric{
		cpu:         &resourceMetric{resourceType: "cpu"},
		memory:      &resourceMetric{resourceType: "memory"},
		nodeMetrics: map[string]*nodeMetric{},
		podMetrics:  map[string]*podMetric{},
	}

	for _, node := range nodeList.Items {
		cm.nodeMetrics[node.Name] = &nodeMetric{
			cpu: &resourceMetric{
				resourceType: "cpu",
				allocatable:  node.Status.Allocatable["cpu"],
			},
			memory: &resourceMetric{
				resourceType: "memory",
				allocatable:  node.Status.Allocatable["memory"],
			},
			podMetrics: map[string]*podMetric{},
		}
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			cm.addPodMetric(&pod)
		}
	}

	for _, node := range nodeList.Items {
		cm.addNodeMetric(cm.nodeMetrics[node.Name])
	}

	for _, node := range nmList.Items {
		nm := cm.nodeMetrics[node.GetName()]
		cm.cpu.utilization.Add(node.Usage["cpu"])
		cm.memory.utilization.Add(node.Usage["memory"])
		nm.cpu.utilization = node.Usage["cpu"]
		nm.memory.utilization = node.Usage["memory"]
	}

	for _, pod := range pmList.Items {
		pm := cm.podMetrics[fmt.Sprintf("%s-%s", pod.GetNamespace(), pod.GetName())]
		if pm != nil {
			for _, container := range pod.Containers {
				pm.cpu.utilization.Add(container.Usage["cpu"])
				pm.memory.utilization.Add(container.Usage["memory"])
			}
		}
	}

	return cm
}
