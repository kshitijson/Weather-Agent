package chat

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/kshitijson/weather-agent/tools"
	"google.golang.org/genai"
)

func AnalyseInput(ctx context.Context, userInput string) (string, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_KEY"),
		Backend: genai.BackendGeminiAPI,
	})

	if err != nil {
		log.Fatal("Error connecting to client")
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

	res, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", []*genai.Content{{Parts: parts}}, nil)
	if err != nil {
		return "", err
	}

	return res.Text(), nil
}

func FinalOutput(ctx context.Context, userMsg string, weather tools.WeatherResponse) (string, error) {

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_KEY"),
		Backend: genai.BackendGeminiAPI,
	})

	if err != nil {
		log.Fatal("Error connecting to client")
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

	res, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", []*genai.Content{{Parts: parts}}, nil)
	if err != nil {
		return "", err
	}

	return res.Text(), nil
}