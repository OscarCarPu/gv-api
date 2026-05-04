package varieties

type Variety struct {
	ID       int32   `json:"id"`
	Name     string  `json:"name"`
	Scent    float32 `json:"scent"`
	Flavor   float32 `json:"flavor"`
	Power    float32 `json:"power"`
	Quality  float32 `json:"quality"`
	Score    float32 `json:"score"`
	Price    float32 `json:"price"`
	Comments *string `json:"comments"`
}

type CreateVarietyRequest struct {
	Name     string  `json:"name"`
	Scent    float32 `json:"scent"`
	Flavor   float32 `json:"flavor"`
	Power    float32 `json:"power"`
	Quality  float32 `json:"quality"`
	Price    float32 `json:"price"`
	Comments *string `json:"comments"`
}

type UpdateVarietyRequest struct {
	ID       int32   `json:"-"`
	Name     string  `json:"name"`
	Scent    float32 `json:"scent"`
	Flavor   float32 `json:"flavor"`
	Power    float32 `json:"power"`
	Quality  float32 `json:"quality"`
	Price    float32 `json:"price"`
	Comments *string `json:"comments"`
}
