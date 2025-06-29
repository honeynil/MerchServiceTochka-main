package models

type Merch struct {
	ID    int32  `json:"id"`
	Price int32  `json:"price"`
	Name  string `json:"name"`
}
