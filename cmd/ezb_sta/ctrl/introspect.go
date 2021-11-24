package ctrl

import (
	"errors"
	"ezBastion/cmd/ezb_sta/models"
	"github.com/gin-gonic/gin"
	"net/http"
)

func Introspect() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.ParseForm()
		kform := "jti"
		vform := c.Request.Form[kform]
		if (len(vform) == 0) || (len(vform) > 1) {
			c.AbortWithStatusJSON(http.StatusBadRequest, errors.New("#STA0200 - wrong jti request"))
		}
		// Checks the values set by the middleware
		jwt := c.MustGet("jti").(string)
		for _, j := range vform {
			if j == jwt {
				u := c.MustGet("connection").(models.StaUser)
				c.JSON(http.StatusOK, u.ExtProperties)
				return
			}

		}
		c.AbortWithStatusJSON(http.StatusForbidden, errors.New("#STA0299 - jti is not recognized"))
	}

}
