package adminweb

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// content 嵌入最小管理端控制台静态资源。
// 这样 1Panel 只需要部署后端容器，不需要额外配置静态站点。
//
//go:embed static/*
var content embed.FS

var staticFiles fs.FS

func Register(engine *gin.Engine) {
	staticFS, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}
	staticFiles = staticFS

	engine.GET("/admin-console", func(ctx *gin.Context) {
		ctx.Redirect(http.StatusPermanentRedirect, "/admin-console/")
	})
	engine.GET("/admin-console/*filepath", serveConsole)
}

func serveConsole(ctx *gin.Context) {
	filePath := ctx.Param("filepath")
	if filePath == "" || filePath == "/" || filePath == "/index.html" {
		serveEmbeddedFile(ctx, "index.html")
		return
	}
	if strings.HasPrefix(filePath, "/assets/") {
		name := path.Clean(strings.TrimPrefix(filePath, "/assets/"))
		if name == "." || strings.HasPrefix(name, "..") {
			ctx.AbortWithStatus(http.StatusBadRequest)
			return
		}
		serveEmbeddedFile(ctx, name)
		return
	}
	ctx.AbortWithStatus(http.StatusNotFound)
}

func serveEmbeddedFile(ctx *gin.Context, name string) {
	data, err := fs.ReadFile(staticFiles, name)
	if err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	ctx.Data(http.StatusOK, contentType, data)
}
