package tools

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

type WeatherResponse struct {
	Location struct {
		Name string `json:"name"`
	} `json:"location"`

	Current struct {
		TempC     float64 `json:"temp_c"`
		Condition struct {
			Text string `json:"text"`
		} `json:"condition"`
		Humidity int `json:"humidity"`
	} `json:"current"`
}

func GetWeather(city string) (WeatherResponse, error) {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error Loading env");
	}

	os.Getenv("WEATHER_KEY")

	url := fmt.Sprintf("%s/%s?key=%s&q=%s", os.Getenv("API_URL"), os.Getenv("CURRENT_METHOD"), os.Getenv("WEATHER_KEY"), city);

	resp, err := http.Get(url)
	if err != nil {
		return WeatherResponse{}, err
	}
	defer resp.Body.Close()

	var weather WeatherResponse;

	if err := json.NewDecoder(resp.Body).Decode(&weather); err != nil {
		return WeatherResponse{}, err;
	}

	// result := fmt.Sprintf(
	// 	"City: %s\nTemperature: %.1f°C\nCondition: %s\nHumidity: %d%%",
	// 	weather.Location.Name,
	// 	weather.Current.TempC,
	// 	weather.Current.Condition.Text,
	// 	weather.Current.Humidity,
	// )

	return weather, nil;
}