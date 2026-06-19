package router

import (
	"orbitterm-server/internal/controller"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"

	"github.com/gin-gonic/gin"
)

// Register 统一挂载 API 路由。
func Register(
	engine *gin.Engine,
	authController *controller.AuthController,
	configController *controller.ConfigController,
	jwtManager *utils.JWTManager,
	userRepo repository.UserRepository,
) {
	v1 := engine.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authController.Register)
			auth.POST("/login", authController.Login)
			auth.POST("/refresh", authController.Refresh)
		}

		configGroup := v1.Group("/config")
		configGroup.Use(middleware.JWTAuthMiddleware(jwtManager, userRepo))
		{
			configGroup.POST("/upload", configController.Upload)
			configGroup.GET("/pull", configController.Pull)
			configGroup.DELETE("/:id", configController.Delete)
		}
	}
}
