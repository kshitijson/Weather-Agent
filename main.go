package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	chat "github.com/kshitijson/weather-agent/handlers"
	"github.com/kshitijson/weather-agent/tools"
)

type toolCall struct {
	Name string `json:"name"`
	City string `json:"city"`
	IsForecast bool `json:"is_forecast"`
	ForecastDays int64 `json:"forecast_days"`
}

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
		msg := helper(body.Input)
		c.JSON(http.StatusOK, gin.H{
			"message": msg,
		})
	})

	r.Run();

}

func helper(userInput string) string {

	ctx := context.Background();

	var toolcall toolCall;

	res, err := chat.AnalyseInput(ctx, userInput);
	if err != nil {
		log.Printf("AnalyseInput error: %s", err)
		return err.Error()
	}

	res = strings.TrimSpace(res)
	res = strings.TrimPrefix(res, "```json")
	res = strings.TrimPrefix(res, "```")
	res = strings.TrimSuffix(res, "```")
	res = strings.TrimSpace(res)

	if err = json.Unmarshal([]byte(res), &toolcall); err != nil {
		log.Printf("Failed to parse Gemini response: %s\nRaw: %s", err, res)
		return ""
	}

	var msg string;

	if toolcall.Name == "get_weather" {

		result, err := tools.GetWeather(toolcall.City, toolcall.IsForecast, toolcall.ForecastDays)
		if err != nil {
			return err.Error();
		}

		msg, err = chat.FinalOutput(ctx, userInput, result, toolcall.ForecastDays);
		if err != nil {
			log.Fatal("Something wrong with gemini finalOutput");
		}

	} else {
		msg = toolcall.City;
	}

	return msg;
}