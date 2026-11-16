package dto

type AlertSettings struct {
	Alerts       *string `json:"alerts" binding:"required,max=500"`
	RunawayShare int     `json:"runaway_share" binding:"required,min=1,max=100"`
	ProjectShare int     `json:"project_share" binding:"required,min=1,max=100"`
}
