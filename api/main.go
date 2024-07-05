package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sethvargo/go-password/password"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	totalRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Number of get requests.",
		},
		[]string{"path"},
	)
	failedRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_failed_total",
			Help: "Number of failed get requests.",
		},
		[]string{"path", "error"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path"},
	)
)

func init() {
	// Register metrics with Prometheus's default registry.
	prometheus.MustRegister(totalRequests)
	prometheus.MustRegister(failedRequests)
	prometheus.MustRegister(requestDuration)
}

func requestMetricsMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		path := c.Path()

		start := time.Now()
		totalRequests.WithLabelValues(path).Inc()

		if err := next(c); err != nil {
			c.Error(err)
			failedRequests.WithLabelValues(path, err.Error()).Inc()
			return err
		}

		duration := time.Since(start).Seconds()
		requestDuration.WithLabelValues(path).Observe(duration)

		return nil
	}
}

func main() {
	// // Use external access config
	// var kubeconfig *string
	// if home := homedir.HomeDir(); home != "" {
	// 	kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	// } else {
	// 	kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	// }
	// flag.Parse()

	// config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	// if err != nil {
	// 	panic(err)
	// }
	// clientset, err := kubernetes.NewForConfig(config)
	// if err != nil {
	// 	panic(err)
	// }

	// Use in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// setup an echo server
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(requestMetricsMiddleware)

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

	e.GET("/healthz", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	e.GET("/readiness", func(c echo.Context) error {
		_, err := getAllDeploymentsInfo(clientset)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Readiness check failed: %v", err))
		}
		return c.NoContent(http.StatusOK)
	})

	e.GET("/startup", func(c echo.Context) error {
		_, err := getAllDeploymentsInfo(clientset)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Startup check failed: %v", err))
		}
		return c.NoContent(http.StatusOK)
	})

	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	e.Logger.Fatal(e.Start(":8081"))
}
