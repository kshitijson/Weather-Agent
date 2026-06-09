package handler

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	chat "github.com/kshitijson/weather-agent/chat"
	"github.com/kshitijson/weather-agent/tools"
)

type toolCall struct {
	Name string `json:"name"`
	City string `json:"city"`
	IsForecast bool `json:"is_forecast"`
	ForecastDays int64 `json:"forecast_days"`
}

func HandleInput(userInput string) string {

	ctx := context.Background()

	var toolcall toolCall

	res, err := chat.AnalyseInput(ctx, userInput)
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

	var msg string

	if toolcall.Name == "get_weather" {

		result, err := tools.GetWeather(toolcall.City, toolcall.IsForecast, toolcall.ForecastDays)
		if err != nil {
			return err.Error()
		}

		msg, err = chat.FinalOutput(ctx, userInput, result, toolcall.ForecastDays)
		if err != nil {
			log.Fatal("Something wrong with gemini finalOutput")
		}

	} else {
		msg = toolcall.City
	}

	return msg
}