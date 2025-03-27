package store

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"sync"
	"syscall"
	"time"

	http "github.com/saucesteals/fhttp"

	"all-unifi-monitor/internal/config"
	"all-unifi-monitor/internal/discord"
	customhttp "all-unifi-monitor/internal/http"
	"all-unifi-monitor/internal/models"
	"all-unifi-monitor/pkg/logger"
)

var buildIDPattern = regexp.MustCompile(`https://[^/]+/_next/static/([a-zA-Z0-9]+)/_ssgManifest\.js`)

type UnifiStore struct {
	cfg             *config.Config
	httpClient      *customhttp.Client
	discord         *discord.Webhook
	baseURL         string
	categories      []string
	knownProductIDs map[string]bool
	knownProducts   map[string]models.Product
	mutex           sync.Mutex
	initialized     bool
	pendingProducts []models.Product
}

func New(cfg *config.Config) *UnifiStore {
	return &UnifiStore{
		cfg:             cfg,
		httpClient:      customhttp.NewClient(),
		discord:         discord.New(cfg.DiscordWebhookURL),
		categories:      defaultCategories(),
		knownProductIDs: make(map[string]bool),
		knownProducts:   make(map[string]models.Product),
	}
}

func defaultCategories() []string {
	return []string{
		"all-switching",
		"all-unifi-cloud-gateways",
		"all-wifi",
		"all-cameras-nvrs",
		"all-door-access",
		"all-cloud-keys-gateways",
		"all-power-tech",
		"all-integrations",
		"accessories-cables-dacs",
	}
}

func (s *UnifiStore) loadKnownProducts() {
	logger.Info().Msg("Loading known products...")
	file, err := os.Open(s.cfg.ProductsFile)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info().Msg("Products.json file not found, creating new file")
			file, err = os.Create(s.cfg.ProductsFile)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to create products.json file")
				return
			}
			file.Close()
			s.initialized = false
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

	var products []models.Product
	if err := json.NewDecoder(file).Decode(&products); err != nil {
		logger.Error().Err(err).Msg("Failed to decode products.json file")
		return
	}

	for _, product := range products {
		s.knownProductIDs[product.ID] = true
		s.knownProducts[product.ID] = product
	}
	logger.Info().Msgf("Loaded %d known products", len(s.knownProductIDs))
	s.initialized = true
}

func (s *UnifiStore) saveKnownProducts() error {
	logger.Info().Msg("Saving known products...")
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Create a slice with all products
	allProducts := make([]models.Product, 0, len(s.knownProducts))
	for _, product := range s.knownProducts {
		allProducts = append(allProducts, product)
	}

	// Create the file with 0644 permissions
	file, err := os.OpenFile(s.cfg.ProductsFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Use buffered writer for better performance
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "    ")

	if err := encoder.Encode(allProducts); err != nil {
		return fmt.Errorf("failed to encode products: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	// Clear pending products after successful save
	s.pendingProducts = s.pendingProducts[:0]

	logger.Info().Msgf("Successfully saved %d products", len(allProducts))
	return nil
}

func (s *UnifiStore) fetchBuildID() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.HomeURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	buffer := &bytes.Buffer{}
	if _, err := io.Copy(buffer, resp.Body); err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	matches := buildIDPattern.FindStringSubmatch(buffer.String())
	if len(matches) < 2 {
		return fmt.Errorf("failed to extract build ID from response")
	}

	buildID := matches[1]
	s.baseURL = fmt.Sprintf("https://store.ui.com/_next/data/%s/us/en.json", buildID)
	logger.Info().Str("buildID", buildID).Msg("Successfully extracted build ID")

	return nil
}

func (s *UnifiStore) fetchProducts(category string) ([]models.Product, error) {
	url := fmt.Sprintf("%s?category=%s&store=us&language=en", s.baseURL, category)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch products: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var response models.Response
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var products []models.Product
	for _, subCategory := range response.PageProps.SubCategories {
		products = append(products, subCategory.Products...)
	}
	return products, nil
}

func (s *UnifiStore) Start() {
	logger.Info().Msg("Starting Monitor")
	s.loadKnownProducts()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a ticker for periodic saves
	saveTicker := time.NewTicker(5 * time.Minute)
	defer saveTicker.Stop()

	// Start signal handler
	go func() {
		<-sigChan
		logger.Info().Msg("Received shutdown signal")
		if err := s.saveKnownProducts(); err != nil {
			logger.Error().Err(err).Msg("Failed to save products during shutdown")
		}
		cancel() // Cancel the context
		logger.Info().Msg("Shutdown complete")
		os.Exit(0)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := s.fetchBuildID(); err != nil {
				logger.Error().Err(err).Msg("Failed to fetch build ID")
				time.Sleep(30 * time.Second)
				continue
			}

			for _, category := range s.categories {
				select {
				case <-ctx.Done():
					return
				default:
					products, err := s.fetchProducts(category)
					if err != nil {
						logger.Error().Err(err).Msg("Failed to fetch products")
						continue
					}

					s.mutex.Lock()
					for _, product := range products {
						if !s.knownProductIDs[product.ID] {
							s.knownProductIDs[product.ID] = true
							s.knownProducts[product.ID] = product
							s.pendingProducts = append(s.pendingProducts, product)
							logger.Info().
								Str("id", product.ID).
								Str("title", product.Title).
								Msg("New product found")

							if err := s.discord.SendProduct(product); err != nil {
								logger.Error().Err(err).Msg("Failed to send Discord notification")
							}
						}
					}
					s.mutex.Unlock()
				}
			}

			// Check for pending products to save
			s.mutex.Lock()
			shouldSave := len(s.pendingProducts) > 0 && (len(s.pendingProducts) >= s.cfg.SaveBatchSize)
			s.mutex.Unlock()

			if shouldSave {
				if err := s.saveKnownProducts(); err != nil {
					logger.Error().Err(err).Msg("Failed to save known products")
				}
			}

			// Check if it's time for a periodic save
			select {
			case <-saveTicker.C:
				s.mutex.Lock()
				hasPending := len(s.pendingProducts) > 0
				s.mutex.Unlock()

				if hasPending {
					if err := s.saveKnownProducts(); err != nil {
						logger.Error().Err(err).Msg("Failed to save known products")
					}
				}
			default:
			}

			logger.Info().Msg("Sleeping for 30 seconds...")
			time.Sleep(30 * time.Second)
		}
	}
}
