package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	handler "github.com/kshitijson/weather-agent/handlers"
)

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading env");
	}

	r := gin.Default();

	r.POST("/chat", func(c *gin.Context){
		var body struct {
			Input string `json:"input"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Input == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Input is required"})
			return
		}
		msg := handler.HandleInput(body.Input)
		c.JSON(http.StatusOK, gin.H{
			"message": msg,
		})
	})

	r.Run();

}