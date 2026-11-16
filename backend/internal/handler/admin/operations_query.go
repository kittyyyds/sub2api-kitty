package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// operationsInt parses optional numeric filters shared by supplier and wallet management.
func operationsInt(c *gin.Context, key string, def int64) (int64, error) {
	if c.Query(key) == "" {
		return def, nil
	}
	return strconv.ParseInt(c.Query(key), 10, 64)
}
