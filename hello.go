package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func main() {
	openWeatherMapAPIKey := flag.String("openweathermap.api.key", "9c6c42895ae17f4af0799d17332d2a8e", "openweathermap.org API key")
	darkskyAPIKey := flag.String("darksky.api.key", "31ce471aec61c089cc42c63c8c08f262", "darksky.net API key")
	flag.Parse()

	mw := multiWeatherProvider {
		openWeatherMap{apiKey: *openWeatherMapAPIKey},
		darkSky{apiKey: *darkskyAPIKey},
	}

	http.HandleFunc("/hello", hello)
	http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		city := strings.SplitN(r.URL.Path, "/", 3)[2]

		temp, err := mw.temperature(city)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"city": city,
			"temp": temp,
			"took": time.Since(begin).String(),
		})
	})
	http.ListenAndServe(":8080", nil)
}

func hello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello world!"))
}

type weatherProvider interface {
	temperature(city string) (float64, error) // units in Kelvin
}

type multiWeatherProvider []weatherProvider

func (w multiWeatherProvider) temperature(city string) (float64, error) {
	// Make a channel for temperatures, and a channel for errors.
    // Each provider will push a value into only one.	
	temps := make(chan float64, len(w))
	errs := make(chan error, len(w))

	// For each provider, spawn a goroutine with an anonymous function.
    // That function will invoke the temperature method, and forward the response.
	for _, provider := range w {
		go func(p weatherProvider) {
			k, err := p.temperature(city)
			if err != nil {
				errs <- err
				return
			}
			temps <- k
		}(provider)
	}

	sum := 0.0

	// Collect a temperature or an error from each provider
	for i := 0; i < len(w); i++ {
		select {
		case temp := <-temps:
			sum += temp
		case err := <-errs:
			return 0, err
		}	
	}

	// Return the average
	return sum / float64(len(w)), nil
}

type openWeatherMap struct {
	apiKey string
}

func (w openWeatherMap) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?APPID=" + w.apiKey + "&q=" + city)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var d struct {
		Main struct {
			Kelvin float64 `json:"temp"`
		} `json:"main"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	farenheit := convertKtoF(d.Main.Kelvin)
	log.Printf("openweathermap: %s: %.2f", city, farenheit)
	return farenheit, nil
}

type darkSky struct {
	apiKey string
}

func (w darkSky) temperature(city string) (float64, error) {
	lat, long, err := findLatitudeLongitude(city)
	if err != nil {
		log.Printf("Failed to find latitude and longitude for %s.\n%s", city, err)
		return 0, err
	}
	lat_long_str := fmt.Sprintf("%f,%f", lat, long)

	resp, err := http.Get("https://api.darksky.net/forecast/" + w.apiKey + "/" + lat_long_str)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var d struct {
		Currently struct {
			Temperature float64 `json:"temperature"` // this might be degrees F
		} `json:"currently"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	farenheit := d.Currently.Temperature
	log.Printf("darksky: %s: %.2f", city, farenheit)
	return farenheit, nil
}

func findLatitudeLongitude(city string) (float64, float64, error) {
	resp, err := http.Get("https://api.opencagedata.com/geocode/v1/json?key=a96a81a58ead4cc4a8e4560d27db1d28&q=" + city)
	if err != nil {
		return 0, 0, err
	}

	defer resp.Body.Close()

	var d struct {
		Results []struct {
			Geometry struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"geometry"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		log.Printf("Decode latitude / longitude struct failed")
		return 0, 0, err
	}
	log.Printf("opencagedata: %s: lat=%f, long=%f", city, d.Results[0].Geometry.Lat, d.Results[0].Geometry.Lng)
	return d.Results[0].Geometry.Lat, d.Results[0].Geometry.Lng, nil

	// return 45.512230, -122.658722, nil // stub for Portland, OR
}

func convertKtoF(f float64) (float64) {
	return f * 9.0 / 5.0 - 459.67
}
