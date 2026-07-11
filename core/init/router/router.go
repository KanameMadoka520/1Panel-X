package router

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	appauth "github.com/1Panel-dev/1Panel/core/app/auth"
	"github.com/1Panel-dev/1Panel/core/app/service"
	"github.com/1Panel-dev/1Panel/core/cmd/server/docs"
	"github.com/1Panel-dev/1Panel/core/cmd/server/web"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/i18n"
	"github.com/1Panel-dev/1Panel/core/init/swagger"
	"github.com/1Panel-dev/1Panel/core/middleware"
	rou "github.com/1Panel-dev/1Panel/core/router"
	"github.com/1Panel-dev/1Panel/core/utils/security"
	"github.com/1Panel-dev/1Panel/core/utils/xpack"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

var (
	Router *gin.Engine
)

func setWebStatic(rootRouter *gin.RouterGroup) {
	rootRouter.StaticFS("/public", http.FS(web.Favicon))
	rootRouter.StaticFS("/favicon.ico", http.FS(web.Favicon))
	RegisterImages(rootRouter)
	setStaticResource(rootRouter)
	rootRouter.GET("/assets/*filepath", func(c *gin.Context) {
		c.Writer.Header().Set("Cache-Control", "private, max-age=2628000, immutable")
		if c.Request.URL.Path[len(c.Request.URL.Path)-1] == '/' {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		staticServer := http.FileServer(http.FS(web.Assets))
		staticServer.ServeHTTP(c.Writer, c.Request)
	})
	authService := service.NewIAuthService()
	entrance := authService.GetSecurityEntrance()
	if entrance != "" {
		rootRouter.GET("/"+entrance, func(c *gin.Context) {
			currentEntrance := authService.GetSecurityEntrance()
			if currentEntrance != entrance {
				security.HandleNotSecurity(c, "")
				return
			}
			security.ToIndexHtml(c)
		})
	}
	rootRouter.GET("/", func(c *gin.Context) {
		if !security.CheckSecurity(c) {
			return
		}
		entrance = authService.GetSecurityEntrance()
		if entrance != "" {
			appauth.SetSecurityEntranceCookie(c, entrance)
		}
		staticServer := http.FileServer(http.FS(web.IndexHtml))
		staticServer.ServeHTTP(c.Writer, c.Request)
	})
}

func Routers() *gin.Engine {
	Router = gin.New()
	Router.Use(i18n.UseI18n())
	Router.Use(middleware.WhiteAllow())
	Router.Use(middleware.BindDomain())

	swaggerRouter := Router.Group("1panel")
	docs.SwaggerInfo.BasePath = "/api/v2"
	swaggerRouter.Use(middleware.SessionAuth()).GET("/swagger/*any", swagger.SwaggerHandler())

	PublicGroup := Router.Group("")
	{
		PublicGroup.Use(gzip.Gzip(gzip.DefaultCompression))
		setWebStatic(PublicGroup)
	}
	if global.CONF.Base.IsDemo {
		Router.Use(middleware.DemoHandle())
	}

	Router.Use(middleware.FrontendFallback())
	Router.Use(middleware.OperationLog())
	Router.Use(middleware.GlobalLoading())
	Router.Use(xpack.AuthProvider.CoreAPIAuthMiddleware())
	Router.Use(middleware.PasswordExpired())
	Router.Use(middleware.CSRFTokenGuard())
	Router.Use(xpack.AuthProvider.CoreRBACMiddlewares()...)
	Router.Use(Proxy())

	PrivateGroup := Router.Group("/api/v2/core")
	PrivateGroup.Use(middleware.SetPasswordPublicKey())
	for _, router := range rou.RouterGroupApp {
		router.InitRouter(PrivateGroup)
	}

	Router.NoRoute(func(c *gin.Context) {
		if !security.HandleNotRoute(c) {
			return
		}
		security.HandleNotSecurity(c, "")
	})

	return Router
}

func RegisterImages(rootRouter *gin.RouterGroup) {
	staticDir := filepath.Join(global.CONF.Base.InstallDir, "1panel/uploads/theme")
	rootRouter.GET("/api/v2/images/*filename", func(c *gin.Context) {
		fileName := filepath.Base(c.Param("filename"))
		// Serve only the fixed set of branding assets, never a stray or
		// traversal-derived name (T3/T6).
		if !service.IsBrandingAssetFileName(fileName) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		filePath := filepath.Join(staticDir, fileName)
		if !strings.HasPrefix(filePath, staticDir) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		fi, err := os.Lstat(filePath)
		if err != nil || !fi.Mode().IsRegular() {
			c.Status(http.StatusNotFound)
			return
		}
		f, err := os.Open(filePath)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer f.Close()
		buf := make([]byte, 512)
		n, _ := f.Read(buf)
		// Constrain the served type to a raster-image allowlist and forbid
		// content sniffing. The prior <svg>-forcing override is removed, so a
		// stored (or stray) body can never be served as image/svg+xml, HTML, or
		// any other active-content type (T1/T2).
		mimeType := sniffImageContentType(buf[:n])
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Header("Content-Type", mimeType)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "no-cache")
		_, _ = io.Copy(c.Writer, f)
	})
}

// sniffImageContentType returns the content type of a served branding asset,
// collapsing anything that is not one of the raster formats the upload path
// accepts (png/jpeg/gif/webp) to a non-renderable octet-stream, so a mislabeled
// or stray body is never served as SVG, HTML, or another active-content type.
// The set is kept identical to the upload whitelist so serve and upload cannot
// drift.
func sniffImageContentType(head []byte) string {
	switch detected := http.DetectContentType(head); detected {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return detected
	default:
		return "application/octet-stream"
	}
}

func setStaticResource(rootRouter *gin.RouterGroup) {
	rootRouter.GET("/api/v2/static/*filename", func(c *gin.Context) {
		c.Writer.Header().Set("Cache-Control", "private, max-age=2628000")
		filename := c.Param("filename")
		filePath := "static" + filename
		data, err := web.Static.ReadFile(filePath)
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		sum := sha256.Sum256(data)
		etag := `"` + hex.EncodeToString(sum[:]) + `"`
		c.Writer.Header().Set("ETag", etag)
		if c.GetHeader("If-None-Match") == etag {
			c.AbortWithStatus(http.StatusNotModified)
			return
		}
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Writer.Write(data)
	})
}
