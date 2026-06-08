package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/kshitijson/weather-agent/tools"
	"google.golang.org/genai"
)

type toolCall struct {
	Name string `json:"name"`
	City string `json:"city"`
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

func analyseInput(ctx context.Context, userInput string) (string, error) {
client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: os.Getenv("GEMINI_KEY"),
		Backend: genai.BackendGeminiAPI,
	})

	if err != nil {
		log.Fatal("Error connecting to client");
	}

	parts := []*genai.Part{
		{Text: "You are a weather assistant."},
		{Text: "Available tool:"},
		{Text: "get_weather(city)"},
		{Text: "When weather information is required,"},
		{Text: "respond ONLY in JSON, for example:"},
		{Text: `{"name":"get_weather","city":"Mumbai"}`},
		{Text: "Otherwise answer normally with the following:"},
		{Text: `{"name":"no_tool","city":"Generate your resposne in here"}`},
		{Text: "User Input:"},
		{Text: userInput},
	}

	res, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", []*genai.Content{{Parts: parts}}, nil);

	return res.Text(), err;
}

func finalOutput(ctx context.Context, userMsg string, weather tools.WeatherResponse) (string, error) {

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: os.Getenv("GEMINI_KEY"),
		Backend: genai.BackendGeminiAPI,
	})

	if err != nil {
		log.Fatal("Error connecting to client");
	}

	parts := []*genai.Part{
		{Text: "User Input: "},
		{Text: userMsg},
		{Text: "Tool Result:"},
		{Text: fmt.Sprintf("City: %s", weather.Location.Name)},	
		{Text: fmt.Sprintf("Temperature: %.1f°C", weather.Current.TempC)},
		{Text: fmt.Sprintf("Condition: %s", weather.Current.Condition.Text)},
		{Text: fmt.Sprintf("Humidity: %d%%", weather.Current.Humidity)},
		{Text: "Generate Final Answer"},
	}

	res, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", []*genai.Content{{Parts: parts}}, nil);

	return res.Text(), err;
}

func helper(userInput string) string {

	ctx := context.Background();

	var toolcall toolCall;

	res, err := analyseInput(ctx, userInput);
	if err != nil {
		log.Fatal("Something wrong with gemini Analyze");
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

		result, err := tools.GetWeather(toolcall.City)
		if err != nil {
			return err.Error();
		}

		msg, err = finalOutput(ctx, userInput, result);
		if err != nil {
			log.Fatal("Something wrong with gemini finalOutput");
		}

	} else {
		msg = toolcall.City;
	}

	return msg;
}