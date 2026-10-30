package response

import "github.com/gin-gonic/gin"

// Pagination represents pagination metadata
type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// Meta represents response metadata
type Meta struct {
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Response represents standard API response structure
type Response struct {
	Data    interface{}         `json:"data"`
	Message string              `json:"message"`
	Meta    *Meta               `json:"meta,omitempty"`
	Errors  map[string][]string `json:"errors,omitempty"`
}

// Success sends a success response without pagination
func Success(c *gin.Context, statusCode int, message string, data interface{}) {
	c.JSON(statusCode, Response{
		Data:    data,
		Message: message,
	})
}

// SuccessWithPagination sends a success response with pagination metadata
func SuccessWithPagination(c *gin.Context, statusCode int, message string, data interface{}, page, limit int, total int64) {
	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}

	c.JSON(statusCode, Response{
		Data:    data,
		Message: message,
		Meta: &Meta{
			Pagination: &Pagination{
				Page:       page,
				Limit:      limit,
				Total:      total,
				TotalPages: totalPages,
			},
		},
	})
}

// Error sends an error response
func Error(c *gin.Context, statusCode int, message string, errorDetail string) {
	resp := Response{
		Data:    nil,
		Message: message,
	}

	if errorDetail != "" {
		resp.Errors = map[string][]string{
			"detail": {errorDetail},
		}
	}

	c.JSON(statusCode, resp)
}

// ValidationError sends a validation error response
func ValidationError(c *gin.Context, statusCode int, message string, errors map[string][]string) {
	c.JSON(statusCode, Response{
		Data:    nil,
		Message: message,
		Errors:  errors,
	})
}
