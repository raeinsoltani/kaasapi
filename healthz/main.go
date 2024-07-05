package main

import (
	"net/http"
	"sync/atomic"

	"github.com/labstack/echo"
)

var requestCount int32

func main() {
	e := echo.New()

	e.GET("/healthz", healthzHandler)

	e.Start(":8081")
}

func healthzHandler(c echo.Context) error {
	count := atomic.AddInt32(&requestCount, 1)
	if count%10 == 0 {
		return c.String(http.StatusInternalServerError, "Bye, World!")
	} else {
		return c.String(http.StatusOK, "Hello, World!")
	}
}
