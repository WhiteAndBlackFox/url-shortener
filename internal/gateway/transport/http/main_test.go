package httpapi_test

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode) // silences the "running in debug mode" warning noise in test output
	os.Exit(m.Run())
}
