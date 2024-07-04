package main

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type DeploymentRequest struct {
	AppName        string          `json:"appName"`
	Replicas       int32           `json:"replicas"`
	ImageAddress   string          `json:"imageAddress"`
	ImageTag       string          `json:"imageTag"`
	DomainAddress  string          `json:"domainAddress"`
	ServicePort    int32           `json:"servicePort"`
	Resources      ResourceRequest `json:"resources"`
	Envs           []KeyValuePair  `json:"envs"`
	Secrets        []KeyValuePair  `json:"secrets"`
	ExternalAccess bool            `json:"ExternalAccess"`
}

type ResourceRequest struct {
	CPU  string `json:"cpu"`
	RAM  string `json:"ram"`
	Disk string `json:"disk"`
}

type KeyValuePair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type DeploymentInfo struct {
	DeploymentName string      `json:"deploymentName"`
	Replicas       int32       `json:"replicas"`
	ReadyReplicas  int32       `json:"readyReplicas"`
	PodStatuses    []PodStatus `json:"podStatuses"`
}

type PodStatus struct {
	Name      string      `json:"name"`
	Phase     string      `json:"phase"`
	HostIP    string      `json:"hostIP"`
	PodIP     string      `json:"podIP"`
	StartTime metav1.Time `json:"startTime"`
}
