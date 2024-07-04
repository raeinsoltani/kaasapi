package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sethvargo/go-password/password"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	// Use external access config
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

	// // Use in-cluster config
	// config, err := rest.InClusterConfig()
	// if err != nil {
	// 	panic(err.Error())
	// }
	// clientset, err := kubernetes.NewForConfig(config)
	// if err != nil {
	// 	panic(err.Error())
	// }

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

	e.POST("/deployments/ready/:appType", func(c echo.Context) error {
		req := new(DeploymentRequest)
		appType := c.Param("appType")
		if err := c.Bind(req); err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error parsing request body: %v", err))
		}

		if appType == "postgres" {
			postgrespass, err := password.Generate(64, 10, 10, false, false)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf(postgrespass)
			secrets := map[string]string{
				"password": postgrespass,
			}

			req.ServicePort = 5432
			req.DomainAddress = "postgres.kubernetes.local"

			_, err = createSecret(clientset, req.AppName, secrets)
			if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error creating secret: %v", err))
			}

			err = createService(clientset, req)
			if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error creating service: %v", err))
			}

			if req.ExternalAccess {
				err = createIngress(clientset, req)
				if err != nil {
					return c.String(http.StatusInternalServerError, fmt.Sprintf("Error creating ingress: %v", err))
				}
			}

			err = postgresStatefulSet(clientset, req)
			if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error creating statefulset: %v", err))
			}

			return c.String(http.StatusCreated, "Statefulset created successfully!/nPostgres password: "+postgrespass)
		} else {
			return c.String(http.StatusNotFound, fmt.Sprintf("App type not found: %v", appType))
		}
	})

	e.Logger.Fatal(e.Start(":8081"))
}
