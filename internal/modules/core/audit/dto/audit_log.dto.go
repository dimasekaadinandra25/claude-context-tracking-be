package dto

type AuditLogQuery struct {
	Page   int    `form:"page,default=1" binding:"min=1"`
	Limit  int    `form:"limit,default=50" binding:"min=1,max=200"`
	Action string `form:"action" binding:"omitempty,max=64"`
	Actor  string `form:"actor" binding:"omitempty,max=255"`
}
