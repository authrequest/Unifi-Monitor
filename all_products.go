package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	discordwebhook "github.com/bensch777/discord-webhook-golang"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v2"
)

const (
	HomeURL      = "https://store.ui.com/us/en"
	ProductsFile = "products.json"
)

var DiscordWebhookURL string

var logger = zerolog.New(
	zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		FormatLevel: func(i interface{}) string {
			return fmt.Sprintf("[%-6s]", i) // Custom level format in square brackets
		},
	},
).Level(zerolog.TraceLevel).With().Timestamp().Caller().Logger()

type UnifiStore struct {
	BaseURL         string
	Headers         map[string]string
	Categories      []string
	KnownProductIDs map[string]bool
	KnownProducts   map[string]Product
	Mutex           sync.Mutex
	Initialized     bool
}

type Product struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	ShortDescription string    `json:"shortDescription"`
	Slug             string    `json:"slug"`
	Thumbnail        Thumbnail `json:"thumbnail"`
	Variants         []Variant `json:"variants"`
}

type Thumbnail struct {
	URL string `json:"url"`
}

type Variant struct {
	ID           string `json:"id"`
	DisplayPrice struct {
		Amount   int    `json:"amount"`
		Currency string `json:"currency"`
	} `json:"displayPrice"`
}

type PageProps struct {
	SubCategories []struct {
		Products []Product `json:"products"`
	} `json:"subCategories"`
}

type Response struct {
	PageProps PageProps `json:"pageProps"`
}

type Config struct {
	DiscordWebhookURL string `yaml:"discord_webhook_url"`
}

var (
	// Compile regex pattern once at package level for better performance
	buildIDPattern = regexp.MustCompile(`https://assets-new\.ecomm\.ui\.com/_next/static/([a-zA-Z0-9]+)/_ssgManifest\.js`)

	// Use a custom HTTP client with timeouts and keep-alive
	httpClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
			ForceAttemptHTTP2:   true,
		},
	}
)

func CreateUnifiStore() *UnifiStore {
	store := &UnifiStore{
		Headers: map[string]string{
			"accept":     "*/*",
			"user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		},
		Categories: []string{
			"all-switching",
			"all-unifi-cloud-gateways",
			"all-wifi",
			"all-cameras-nvrs",
			"all-door-access",
			"all-cloud-keys-gateways",
			"all-power-tech",
			"all-integrations",
			"accessories-cables-dacs",
		},
		KnownProductIDs: make(map[string]bool),
		KnownProducts:   make(map[string]Product),
		Initialized:     false,
	}
	store.loadKnownProducts()
	return store
}

// loadKnownProducts reads the products.json file and loads all known products into the store's maps.
// If the file does not exist, it will be created.
// If the file is malformed, an error is logged and the store is not initialized.
func (store *UnifiStore) loadKnownProducts() {
	logger.Info().Msg("Loading known products...")
	file, err := os.Open(ProductsFile)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info().Msg("Products.json file not found, creating new file")
			file, err = os.Create(ProductsFile)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to create products.json file")
				return
			}
			store.Initialized = false
			return
		}
		logger.Error().Err(err).Msg("Failed to load products.json file")
		return

	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get file info")
		return
	}

	if fileInfo.Size() == 0 {
		return
	}

	var products []Product
	if err := json.NewDecoder(file).Decode(&products); err != nil {
		logger.Error().Err(err).Msg("Failed to decode products.json file")
		return
	}

	for _, product := range products {
		store.KnownProductIDs[product.ID] = true
		store.KnownProducts[product.ID] = product
	}
	logger.Info().Msg(fmt.Sprintf("Loaded %d known products", len(store.KnownProductIDs)))
	store.Initialized = true
}

// saveKnownProducts writes the current known products to a JSON file.
// It locks the store's mutex to ensure thread safety while accessing
// the KnownProducts map, encodes the products into JSON format, and
// saves them to the file specified by ProductsFile. If an error occurs
// during file creation or encoding, it logs the error.

func (store *UnifiStore) saveKnownProducts(filename string, newProducts []Product) error {
	logger.Info().Msg("Saving known products...")
	store.Mutex.Lock()
	defer store.Mutex.Unlock()

	// Read existing products from the file
	var existingProducts []Product
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open known products file: %v", err)
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&existingProducts); err != nil && err != io.EOF {
		return fmt.Errorf("failed to decode existing products: %v", err)
	}

	// Append new products to the existing products
	existingProducts = append(existingProducts, newProducts...)

	// Write the combined list back to the file
	file.Seek(0, 0)
	file.Truncate(0)
	if err := json.NewEncoder(file).Encode(existingProducts); err != nil {
		return fmt.Errorf("failed to encode products: %v", err)
	}

	return nil
}

// fetchBuildID attempts to retrieve the build ID from the Unifi store homepage.
// It sends a GET request to the HomeURL and searches the response body for a
// build ID using a regex pattern. If successful, it sets the BaseURL of the
// UnifiStore with the extracted build ID. If the build ID cannot be extracted,
// it logs an error and returns an error.

func (store *UnifiStore) fetchBuildID() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, HomeURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers in a single loop
	for key, value := range store.Headers {
		req.Header.Set(key, value)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Use a buffer pool for better memory management
	buffer := bufPool.Get().(*bytes.Buffer)
	buffer.Reset()
	defer bufPool.Put(buffer)

	// Use io.Copy instead of ioutil.ReadAll for better memory efficiency
	if _, err := io.Copy(buffer, resp.Body); err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	matches := buildIDPattern.FindStringSubmatch(buffer.String())
	if len(matches) < 2 {
		return fmt.Errorf("failed to extract build ID from response")
	}

	buildID := matches[1]
	store.BaseURL = fmt.Sprintf("https://store.ui.com/_next/data/%s/us/en.json", buildID)
	logger.Info().Str("buildID", buildID).Msg("Successfully extracted build ID")

	return nil
}

// Create a buffer pool for reusing buffers
var bufPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// Add a retry mechanism for better reliability
func (store *UnifiStore) fetchBuildIDWithRetry(maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := store.fetchBuildID(); err != nil {
			lastErr = err
			backoff := time.Duration(math.Pow(2, float64(i))) * time.Second
			logger.Warn().
				Err(err).
				Int("attempt", i+1).
				Dur("backoff", backoff).
				Msg("Retrying fetch build ID")
			time.Sleep(backoff)
			continue
		}
		return nil
	}
	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// fetchProducts fetches the products for a given category from the Unifi store.
//
// It takes a category as a string and returns a slice of Product objects and an error.
// If the error is not nil, it should be logged and the program should retry the call.
//
// The products are fetched from the Unifi store with a GET request to the URL
// <BaseURL>?category=<category>&store=us&language=en.
//
// The response is unmarshaled into a struct with a field "pageProps" which contains
// a slice of structs with a field "subCategories" which contains a slice of structs
// with a field "products". The latter is the slice of Product objects that is returned.
//
// The function logs an info message with the category and an error message with the
// error if it occurs.
func (store *UnifiStore) fetchProducts(category string) ([]Product, error) {
	// logger.Info().Msg(fmt.Sprintf("Fetching products for category: %s", category))
	url := fmt.Sprintf("%s?category=%s&store=us&language=en", store.BaseURL, category)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request: %v", err)
	}

	for k, v := range store.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch products: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read response body: %v", err)
	}

	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal JSON: %v", err)
	}

	var products []Product
	for _, subCategory := range response.PageProps.SubCategories {
		products = append(products, subCategory.Products...)
	}
	return products, nil
}

func (store *UnifiStore) sendToDiscord(product Product) {
	if !store.Initialized {
		return
	}

	fmt.Println("Sending to Discord...")

	embed := discordwebhook.Embed{
		Title:     product.Title,
		Color:     15277667,
		Url:       fmt.Sprintf("https://store.ui.com/us/en/products/%s", product.Slug),
		Timestamp: time.Now(),
		Thumbnail: discordwebhook.Thumbnail{
			Url: product.Thumbnail.URL,
		},
		Author: discordwebhook.Author{
			Name:     "🎉 **New Product Alert!** 🎉",
			Icon_URL: "https://tse3.mm.bing.net/th?id=OIP.RadjPrUUrLwqfVTEI5YqmwHaIV&pid=Api&P=0&w=300&h=300",
		},
		Description: fmt.Sprintf("%s\n", product.ShortDescription),
		Fields: []discordwebhook.Field{
			discordwebhook.Field{
				Name:   "Variant",
				Value:  product.Variants[0].ID,
				Inline: true,
			},
			discordwebhook.Field{
				Name:   "Price",
				Value:  fmt.Sprintf("$%d.%02d", product.Variants[0].DisplayPrice.Amount/100, product.Variants[0].DisplayPrice.Amount%100),
				Inline: true,
			},
		},
		Footer: discordwebhook.Footer{
			Text:     "Unifi Store Monitor",
			Icon_url: "https://tse3.mm.bing.net/th?id=OIP.RadjPrUUrLwqfVTEI5YqmwHaIV&pid=Api&P=0&w=300&h=300",
		},
	}

	hook := discordwebhook.Hook{
		Username:   "Unifi Store Monitor",
		Avatar_url: "https://tse3.mm.bing.net/th?id=OIP.RadjPrUUrLwqfVTEI5YqmwHaIV&pid=Api&P=0&w=300&h=300",
		Embeds:     []discordwebhook.Embed{embed},
	}

	payload, err := json.Marshal(hook)
	if err != nil {
		log.Fatal(err)
	}
	err = discordwebhook.ExecuteWebhook(DiscordWebhookURL, payload)
	if err != nil {
		fmt.Printf("Failed to send to Discord: %v\n", err)
	}
}

func readEnv(key, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if exists {
		return value
	}

	configFile := "/etc/config.yml"
	if _, err := os.Stat(configFile); err == nil {
		data, err := ioutil.ReadFile(configFile)
		if err == nil {
			var config Config
			if err := yaml.Unmarshal(data, &config); err == nil {
				if key == "DISCORD_WEBHOOK_URL" {
					return config.DiscordWebhookURL
				}
			}
		}
	}
	return defaultValue
}

// Start begins an infinite loop to monitor and fetch new products from the Unifi store.
// It iterates through each category in the store, fetching products and checking if
// they are new by comparing against known product IDs. If a new product is found, it
// is added to the store's known products and a log message is generated. The loop
// sleeps for 30 seconds between each complete iteration. In case of an error while
// fetching products, it logs the error and retries after a 30-second delay.

func (store *UnifiStore) Start() {
	logger.Info().Msg("Starting Monitor")

	for {
		// Use retry mechanism with 3 attempts
		if err := store.fetchBuildIDWithRetry(3); err != nil {
			logger.Fatal().Err(err).Msg("Failed to fetch build ID after retries")
		}
		for _, category := range store.Categories {
			products, err := store.fetchProducts(category)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to fetch products")
				time.Sleep(30 * time.Second)
				continue
			}
			var newProducts []Product
			store.Mutex.Lock()
			for _, product := range products {
				if !store.KnownProductIDs[product.ID] {
					store.KnownProductIDs[product.ID] = true
					store.KnownProducts[product.ID] = product
					logger.Info().Msg(fmt.Sprintf("New Product Alert! ID: %s, Title: %s", product.ID, product.Title))
					newProducts = append(newProducts, product)
					store.sendToDiscord(product)
				}
			}
			store.Mutex.Unlock()

			if len(newProducts) > 0 {
				if err := store.saveKnownProducts(ProductsFile, newProducts); err != nil {
					logger.Error().Err(err).Msg("Failed to save known products")
				}
			}
			// logger.Info().Msg(fmt.Sprintf("Fetched %d products for category: %s", len(products), category))
		}
		logger.Info().Msg("Sleeping for 30 seconds...")
		time.Sleep(30 * time.Second)
	}
}

func main() {
	logger.Info().Msg("Initializing...")
	DiscordWebhookURL = readEnv("DISCORD_WEBHOOK_URL", "")
	if DiscordWebhookURL == "" {
		logger.Fatal().Msg("DISCORD_WEBHOOK_URL is not set. Please set it in the environment or in the config file.")
	}
	store := CreateUnifiStore()
	go store.Start()
	// Keep the main thread alive
	select {}
}
