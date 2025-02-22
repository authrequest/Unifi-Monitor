package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	customhttp "all-unifi-monitor/internal/http"
	"all-unifi-monitor/internal/models"

	http "github.com/saucesteals/fhttp"
)

type Author struct {
	Name     string `json:"name"`
	Icon_URL string `json:"icon_url"`
}

type Embed struct {
	Title       string    `json:"title"`
	Color       int       `json:"color"`
	Url         string    `json:"url"`
	Timestamp   time.Time `json:"timestamp"`
	Thumbnail   Thumbnail `json:"thumbnail"`
	Author      Author    `json:"author"`
	Description string    `json:"description"`
	Fields      []Field   `json:"fields"`
	Footer      Footer    `json:"footer"`
}

type Thumbnail struct {
	Url string `json:"url"`
}

type Field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type Footer struct {
	Text     string `json:"text"`
	Icon_url string `json:"icon_url"`
}

type Hook struct {
	Username   string  `json:"username"`
	Avatar_url string  `json:"avatar_url"`
	Embeds     []Embed `json:"embeds"`
}

type Webhook struct {
	url        string
	httpClient *customhttp.Client
}

func New(url string) *Webhook {
	return &Webhook{
		url:        url,
		httpClient: customhttp.NewClient(),
	}
}

func (w *Webhook) SendProduct(product models.Product) error {
	embed := Embed{
		Title:     product.Title,
		Color:     15277667,
		Url:       fmt.Sprintf("https://store.ui.com/us/en/products/%s", product.Slug),
		Timestamp: time.Now(),
		Thumbnail: Thumbnail{
			Url: product.Thumbnail.URL,
		},
		Author: Author{
			Name:     "🎉 **New Product Alert!** 🎉",
			Icon_URL: "https://tse3.mm.bing.net/th?id=OIP.RadjPrUUrLwqfVTEI5YqmwHaIV&pid=Api&P=0&w=300&h=300",
		},
		Description: fmt.Sprintf("%s\n", product.ShortDescription),
		Fields: []Field{
			{
				Name:   "Variant",
				Value:  product.Variants[0].ID,
				Inline: true,
			},
			{
				Name:   "Price",
				Value:  fmt.Sprintf("$%d.%02d", product.Variants[0].DisplayPrice.Amount/100, product.Variants[0].DisplayPrice.Amount%100),
				Inline: true,
			},
		},
		Footer: Footer{
			Text:     "Unifi Store Monitor",
			Icon_url: "https://tse3.mm.bing.net/th?id=OIP.RadjPrUUrLwqfVTEI5YqmwHaIV&pid=Api&P=0&w=300&h=300",
		},
	}

	hook := Hook{
		Username:   "Unifi Store Monitor",
		Avatar_url: "https://tse3.mm.bing.net/th?id=OIP.RadjPrUUrLwqfVTEI5YqmwHaIV&pid=Api&P=0&w=300&h=300",
		Embeds:     []Embed{embed},
	}

	payload, err := json.Marshal(hook)
	if err != nil {
		return fmt.Errorf("failed to marshal discord payload: %w", err)
	}

	req, err := http.NewRequest("POST", w.url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create discord request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		// Rate limited, wait and retry
		time.Sleep(5 * time.Second)
		return w.SendProduct(product)
	}

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("discord webhook returned status code: %d", resp.StatusCode)
	}

	return nil
}
