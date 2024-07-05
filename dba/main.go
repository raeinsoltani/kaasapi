package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type App struct {
	ID           uint   `gorm:"primaryKey"`
	AppName      string `gorm:"unique;not null"`
	FailureCount int    `gorm:"default:0"`
	SuccessCount int    `gorm:"default:0"`
	LastFailure  time.Time
	LastSuccess  time.Time
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

var db *gorm.DB

func main() {
	var err error

	// Retrieve the database URL from the environment variable
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable not set")
	}

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalln(err)
	}

	// Auto migrate the schema
	db.AutoMigrate(&App{})

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.POST("/increase_failure", increaseFailure)
	e.POST("/increase_success", increaseSuccess)
	e.GET("/health/:app_name", getAppHealth)

	e.Logger.Fatal(e.Start(":1323"))
}

func increaseFailure(c echo.Context) error {
	appName := c.QueryParam("app_name")
	if appName == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "app_name is required"})
	}

	var app App
	if err := db.First(&app, "app_name = ?", appName).Error; err != nil {
		// Create a new app in the database
		app := App{
			AppName:      appName,
			FailureCount: 0,
			SuccessCount: 0,
			LastFailure:  time.Time{},
			LastSuccess:  time.Time{},
		}
		if err := db.Create(&app).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": "failed to create app"})
		}
		return c.JSON(http.StatusNotFound, echo.Map{"error": "app not found"})
	}

	app.FailureCount++
	app.LastFailure = time.Now()
	db.Save(&app)

	return c.JSON(http.StatusOK, echo.Map{"message": "failure count increased", "app": app})
}

func increaseSuccess(c echo.Context) error {
	appName := c.QueryParam("app_name")
	if appName == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "app_name is required"})
	}

	var app App
	if err := db.First(&app, "app_name = ?", appName).Error; err != nil {
		app := App{
			AppName:      appName,
			FailureCount: 0,
			SuccessCount: 0,
			LastFailure:  time.Time{},
			LastSuccess:  time.Time{},
		}
		if err := db.Create(&app).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": "failed to create app"})
		}
		return c.JSON(http.StatusNotFound, echo.Map{"error": "app not found"})
	}

	app.SuccessCount++
	app.LastSuccess = time.Now()
	db.Save(&app)

	return c.JSON(http.StatusOK, echo.Map{"message": "success count increased", "app": app})
}

func getAppHealth(c echo.Context) error {
	appName := c.Param("app_name")
	if appName == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "app_name is required"})
	}

	var app App
	if err := db.First(&app, "app_name = ?", appName).Error; err != nil {
		return c.JSON(http.StatusNotFound, echo.Map{"error": "app not found"})
	}

	return c.JSON(http.StatusOK, app)
}
