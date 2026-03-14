package models

type AuthUser struct {
	ID              string  `json:"id"`
	Email           *string `json:"email"`
	FirstName       *string `json:"firstName"`
	LastName        *string `json:"lastName"`
	ProfileImageURL *string `json:"profileImageUrl"`
	CreatedAt       *int64  `json:"createdAt"`
	UpdatedAt       *int64  `json:"updatedAt"`
}

type MessageResponse struct {
	Message string `json:"message"`
}
