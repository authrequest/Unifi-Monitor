package models

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
