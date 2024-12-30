package types

type Work struct {
	Authors         []Author    `json:"authors" db:"authors"`
	CreatedAt       string      `json:"createdAt" db:"created_at"`
	ForegroundColor string      `json:"foregroundColor" db:"foreground_color"`
	BackgroundColor string      `json:"backgroundColor" db:"background_color"`
	ID              string      `json:"id" db:"id"`
	InAquarium      bool        `json:"inAquarium" db:"in_aquarium"`
	Name            string      `json:"name" db:"name"`
	Object          ModelObject `json:"object" db:"object"`
	PreviewURL      string      `json:"previewUrl" db:"preview_url"`
}

type Author struct {
	Channel        string `json:"channel" db:"channel"`
	Logo           string `json:"logo" db:"logo"`
	Name           string `json:"name" db:"name"`
	TelegramUserID int    `json:"telegramUserId" db:"telegram_user_id"`
}

type ModelObject struct {
	HdriURL   string      `json:"hdriUrl" db:"hdri_url"`
	ObjectURL string      `json:"objectUrl" db:"object_url"`
	Position  []float64   `json:"position" db:"position"`
	Scale     interface{} `json:"scale" db:"scale"`
}
