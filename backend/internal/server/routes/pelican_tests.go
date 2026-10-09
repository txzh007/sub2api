package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerPelicanTestRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	if h.Admin == nil || h.Admin.PelicanTest == nil {
		return
	}
	tests := admin.Group("/pelican-tests")
	tests.GET("/options", h.Admin.PelicanTest.Options)
	tests.GET("/models", h.Admin.PelicanTest.Models)
	tests.GET("/runs", h.Admin.PelicanTest.ListRuns)
	tests.POST("/runs", h.Admin.PelicanTest.CreateRun)
	tests.GET("/runs/:id", h.Admin.PelicanTest.GetRun)
	tests.POST("/runs/:id/cancel", h.Admin.PelicanTest.CancelRun)
	tests.POST("/batches", h.Admin.PelicanTest.CreateBatch)
	tests.GET("/batches/:batch", h.Admin.PelicanTest.GetBatch)
	tests.POST("/batches/:batch/cancel", h.Admin.PelicanTest.CancelBatch)
	tests.GET("/templates", h.Admin.PelicanTest.ListTemplates)
	tests.POST("/templates", h.Admin.PelicanTest.SaveTemplate)
	tests.PUT("/templates/:id", h.Admin.PelicanTest.SaveTemplate)
	tests.DELETE("/templates/:id", h.Admin.PelicanTest.DeleteTemplate)
}
