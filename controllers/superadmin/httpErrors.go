package superadmin

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/gin-gonic/gin"
)

// RespondError re-exports controllers.RespondError so every handler in this
// package can call it unqualified. Single implementation lives in
// controllers/httpErrors.go — it logs err server-side and never puts
// err.Error() in the JSON body, so DB/internal errors don't leak to the UI.
func RespondError(c *gin.Context, status int, message string, err error) {
	controllers.RespondError(c, status, message, err)
}
