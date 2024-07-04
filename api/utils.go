package main

import (
	"context"
	"fmt"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func getAllDeploymentsInfo(clientset *kubernetes.Clientset) ([]DeploymentInfo, error) {
	deploymentList, err := clientset.AppsV1().Deployments(corev1.NamespaceDefault).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing deployments: %v", err)
	}

	deploymentsInfo := make([]DeploymentInfo, 0)
	for _, deployment := range deploymentList.Items {
		podList, err := clientset.CoreV1().Pods(corev1.NamespaceDefault).List(context.TODO(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", deployment.Labels["app"]),
		})
		if err != nil {
			return nil, fmt.Errorf("error listing pods for deployment %s: %v", deployment.Name, err)
		}

		podStatuses := make([]PodStatus, 0)
		for _, pod := range podList.Items {
			podStatuses = append(podStatuses, PodStatus{
				Name:      pod.Name,
				Phase:     string(pod.Status.Phase),
				HostIP:    pod.Status.HostIP,
				PodIP:     pod.Status.PodIP,
				StartTime: *pod.Status.StartTime,
			})

			fmt.Println(podStatuses)
		}

		deploymentInfo := DeploymentInfo{
			DeploymentName: deployment.Name,
			Replicas:       *deployment.Spec.Replicas,
			ReadyReplicas:  deployment.Status.ReadyReplicas,
			PodStatuses:    podStatuses,
		}

		fmt.Println(deploymentInfo)

		deploymentsInfo = append(deploymentsInfo, deploymentInfo)
	}

	return deploymentsInfo, nil
}

func getDeploymentInfo(clientset *kubernetes.Clientset, appName string) (*DeploymentInfo, error) {
	deployment, err := clientset.AppsV1().Deployments(corev1.NamespaceDefault).Get(context.TODO(), appName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("deployment not found: %v", err)
	}

	podList, err := clientset.CoreV1().Pods(corev1.NamespaceDefault).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", deployment.Labels["app"]),
	})
	if err != nil {
		return nil, fmt.Errorf("error listing pods: %v", err)
	}

	podStatuses := make([]PodStatus, 0)
	for _, pod := range podList.Items {
		podStatuses = append(podStatuses, PodStatus{
			Name:      pod.Name,
			Phase:     string(pod.Status.Phase),
			HostIP:    pod.Status.HostIP,
			PodIP:     pod.Status.PodIP,
			StartTime: *pod.Status.StartTime,
		})
	}

	deploymentInfo := &DeploymentInfo{
		DeploymentName: deployment.Name,
		Replicas:       *deployment.Spec.Replicas,
		ReadyReplicas:  deployment.Status.ReadyReplicas,
		PodStatuses:    podStatuses,
	}

	return deploymentInfo, nil
}

func createDeployment(clientset *kubernetes.Clientset, req *DeploymentRequest) error {

	fmt.Println(req)

	// Create service
	err := createService(clientset, req)
	if err != nil {
		return err
	}

	// ExternalAccess True, create ingress object
	if req.ExternalAccess {
		err = createIngress(clientset, req)
		if err != nil {
			return err
		}
	}

	// create secrets if requested
	if len(req.Secrets) > 0 {
		secretName := fmt.Sprintf("%v-secret", req.AppName)
		secretsMap := make(map[string]string)
		for _, kv := range req.Secrets {
			secretsMap[kv.Key] = kv.Value
		}
		_, err := createSecret(clientset, secretName, secretsMap)
		if err != nil {
			return err
		}
	}

	// create secrets if requested
	if len(req.Envs) > 0 {
		configMapName := fmt.Sprintf("%v-config", req.AppName)
		configsMap := make(map[string]string)
		for _, kv := range req.Envs {
			configsMap[kv.Key] = kv.Value
		}
		_, err := createConfigMap(clientset, configMapName, configsMap)
		if err != nil {
			return err
		}
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.AppName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(req.Replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": req.AppName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": req.AppName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  req.AppName,
							Image: fmt.Sprintf("%s:%s", req.ImageAddress, req.ImageTag),
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: req.ServicePort,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resourceQuantity(req.Resources.CPU),
									corev1.ResourceMemory: resourceQuantity(req.Resources.RAM),
								},
							},

							// Conditionally add EnvFrom based on Secrets or Envs
							EnvFrom: func() []corev1.EnvFromSource {
								var envFromSources []corev1.EnvFromSource
								if len(req.Secrets) > 0 {
									secretName := fmt.Sprintf("%v-secret", req.AppName)
									envFromSources = append(envFromSources, corev1.EnvFromSource{
										SecretRef: &corev1.SecretEnvSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
										},
									})
								}
								if len(req.Envs) > 0 {
									configMapName := fmt.Sprintf("%v-config", req.AppName)
									envFromSources = append(envFromSources, corev1.EnvFromSource{
										ConfigMapRef: &corev1.ConfigMapEnvSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
										},
									})
								}
								return envFromSources
							}(),
						},
					},
				},
			},
		},
	}

	deploymentsClient := clientset.AppsV1().Deployments(corev1.NamespaceDefault)
	fmt.Println("Creating deployment...")
	_, err = deploymentsClient.Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func createSecret(clientset *kubernetes.Clientset, SecretName string, Secrets map[string]string) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: SecretName + "-secret",
		},
		StringData: Secrets,
	}

	_, err := clientset.CoreV1().Secrets(corev1.NamespaceDefault).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("error creating secret: %v", err)
	}

	return secret, nil
}

func createConfigMap(clientset *kubernetes.Clientset, configMapName string, envs map[string]string) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapName,
		},
		Data: envs,
	}

	_, err := clientset.CoreV1().ConfigMaps(corev1.NamespaceDefault).Create(context.TODO(), configMap, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("error creating config map: %v", err)
	}

	return configMap, nil
}

func createService(clientset *kubernetes.Clientset, req *DeploymentRequest) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.AppName + "-service",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": req.AppName,
			},
			Ports: []corev1.ServicePort{
				{
					Port: req.ServicePort,
				},
			},
		},
	}
	servicesClient := clientset.CoreV1().Services(corev1.NamespaceDefault)
	fmt.Println("Creating service...")
	_, err := servicesClient.Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func createIngress(clientset *kubernetes.Clientset, req *DeploymentRequest) error {
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.AppName + "-ingress",
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: req.DomainAddress,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									PathType: func() *networkingv1.PathType {
										pt := networkingv1.PathTypePrefix
										return &pt
									}(),
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: req.AppName + "-service",
											Port: networkingv1.ServiceBackendPort{
												Number: req.ServicePort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ingressesClient := clientset.NetworkingV1().Ingresses(corev1.NamespaceDefault)
	fmt.Println("Creating ingress...")
	_, err := ingressesClient.Create(context.TODO(), ingress, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func int32Ptr(i int32) *int32 {
	return &i
}

func resourceQuantity(value string) resource.Quantity {
	qty, err := resource.ParseQuantity(value)
	if err != nil {
		log.Printf("Error parsing resource quantity %s: %v", value, err)
	}
	return qty
}

func postgresStatefulSet(clientSet *kubernetes.Clientset, req *DeploymentRequest) error {

	replicas := int32Ptr(1)

	statefulSet := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: req.AppName,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: req.AppName,
			Replicas:    replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": req.AppName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": req.AppName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  req.AppName,
							Image: "postgres:13",
							Env: []corev1.EnvVar{
								{
									Name:  "POSTGRES_USER",
									Value: "postgres",
								},
								{
									Name: "POSTGRES_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: req.AppName + "-secret",
											},
											Key: "password",
										},
									},
								},
								{
									Name:  "PGDATA",
									Value: "/var/lib/postgresql/data/pgdata",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: req.ServicePort,
									Name:          "postgres",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      req.AppName + "-pv-claim",
									MountPath: "/var/lib/postgresql/data",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resourceQuantity(req.Resources.CPU),
									corev1.ResourceMemory: resourceQuantity(req.Resources.RAM),
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: req.AppName + "-pv-claim",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("2Gi"),
							},
						},
					},
				},
			},
		},
	}

	statefulSetClient := clientSet.AppsV1().StatefulSets(corev1.NamespaceDefault)
	fmt.Println("Creating deployment...")
	_, err := statefulSetClient.Create(context.TODO(), statefulSet, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}
