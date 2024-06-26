package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type DeploymentRequest struct {
	AppName        string                `json:"appName"`
	Replicas       int32                 `json:"replicas"`
	ImageAddress   string                `json:"imageAddress"`
	ImageTag       string                `json:"imageTag"`
	DomainAddress  string                `json:"domainAddress"`
	ServicePort    int32                 `json:"servicePort"`
	Resources      ResourceRequest       `json:"resources"`
	Envs           []KeyValuePair        `json:"envs"`
	Secrets        []KeyValuePair        `json:"secrets"`
	ExternalAccess ExternalAccessRequest `json:"externalAccess"`
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

type ExternalAccessRequest struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Path    string `json:"path"`
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

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	// setup an echo server
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/deployments/:appName", func(c echo.Context) error {
		appName := c.Param("appName")
		deploymentInfo, err := getDeploymentInfo(clientset, appName)
		if err != nil {
			return c.String(http.StatusNotFound, fmt.Sprintf("Error fetching deployment: %v", err))
		}

		return c.JSON(http.StatusOK, deploymentInfo)
	})

	e.GET("/deployments", func(c echo.Context) error {
		deploymentsInfo, err := getAllDeploymentsInfo(clientset)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error fetching deployments: %v", err))
		}

		return c.JSON(http.StatusOK, deploymentsInfo)
	})

	e.POST("/deployments", func(c echo.Context) error {
		req := new(DeploymentRequest)
		if err := c.Bind(req); err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error parsing request body: %v", err))
		}
		err := createDeployment(clientset, req)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error creating deployment: %v", err))
		}

		return c.String(http.StatusCreated, "Deployment created successfully!")
	})

	e.Logger.Fatal(e.Start(":8081"))
}

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
		}

		deploymentInfo := DeploymentInfo{
			DeploymentName: deployment.Name,
			Replicas:       *deployment.Spec.Replicas,
			ReadyReplicas:  deployment.Status.ReadyReplicas,
			PodStatuses:    podStatuses,
		}

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
	fmt.Println(req)
	_, err := deploymentsClient.Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	if req.ExternalAccess.Enabled {
		ingress := &extensionsv1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: req.AppName + "-ingress",
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/rewrite-target": "/",
				},
			},
			Spec: extensionsv1beta1.IngressSpec{
				Rules: []extensionsv1beta1.IngressRule{
					{
						Host: req.ExternalAccess.Host,
						IngressRuleValue: extensionsv1beta1.IngressRuleValue{
							HTTP: &extensionsv1beta1.HTTPIngressRuleValue{
								Paths: []extensionsv1beta1.HTTPIngressPath{
									{
										Path: req.ExternalAccess.Path,
										Backend: extensionsv1beta1.IngressBackend{
											ServiceName: req.AppName,
											ServicePort: intstr.FromInt(int(req.ServicePort)),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		ingressesClient := clientset.ExtensionsV1beta1().Ingresses(corev1.NamespaceDefault)
		fmt.Println("Creating ingress...")
		_, err := ingressesClient.Create(context.TODO(), ingress, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("error creating ingress: %v", err)
		}
	}

	return nil
}

func createSecret(clientset *kubernetes.Clientset, SecretName string, Secrets map[string]string) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: SecretName,
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
