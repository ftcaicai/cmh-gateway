package brand

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Routes implements core.RouteRegistrar (H5) for the brand storefront.
//
// P0 prototype: one middleware-style handler that answers brand hosts before
// the official web fallback runs. We cannot (and must not) replace the
// official NoRoute, so brand pages are served from an early handler that only
// acts when Resolve has tagged the request as a brand request.
type Routes struct{}

func (Routes) Register(engine *gin.Engine) {
	engine.Use(serveBrandPage)
}

// serveBrandPage owns every page on a mapped brand host. Until a page is
// implemented, it returns 404 rather than exposing the official storefront.
func serveBrandPage(c *gin.Context) {
	id := BrandID(c)
	if id == "" {
		c.Next()
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if c.Request.URL.Path != "/" && c.Request.URL.Path != "/login" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("X-CMH-Tenant", id)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	c.Status(http.StatusOK)
	if c.Request.Method != http.MethodHead {
		_, _ = c.Writer.Write([]byte(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>品牌站预览</title>
<style>
*{box-sizing:border-box}body{margin:0;font-family:system-ui,-apple-system,sans-serif;color:#1b2f4f;background:#f4f7fc}
header{height:76px;background:#fff;border-bottom:1px solid #dce5f3;display:flex;align-items:center;padding:0 max(24px,calc((100% - 1240px)/2));font-weight:700}
main{max-width:1040px;margin:70px auto;padding:0 20px}.layout{display:flex;background:#fff;border:1px solid #dce5f3;border-radius:20px;overflow:hidden;min-height:530px}
.hero,.panel{flex:1;padding:48px 42px}.hero{background:#e8f0ff;display:flex;flex-direction:column}.symbol{font-size:44px;color:#285cf2}.hero h1{font-size:36px;line-height:1.35;margin:38px 0 20px}.hero p,.panel p{color:#5c6e8c;line-height:1.7}.points{color:#285cf2;line-height:2.4;margin-top:26px}.panel h2{font-size:28px;margin:20px 0}.label{color:#285cf2;font-size:12px;font-weight:700;letter-spacing:.15em}.notice{margin-top:38px;padding:20px;border:1px solid #dce5f3;border-radius:10px;background:#f8faff}.notice strong{display:block;margin-bottom:10px}.back{display:inline-block;margin-top:26px;color:#285cf2;text-decoration:none}
@media(max-width:700px){header{height:64px}main{margin:24px auto}.layout{flex-direction:column}.hero,.panel{padding:32px 24px}.hero h1{font-size:28px;margin:20px 0 12px}}
</style>
</head>
<body><header>品牌站预览</header><main><div class="layout"><section class="hero"><div class="symbol" aria-hidden="true">✳</div><h1>从灵感，<br>到下一次创造。</h1><p>品牌客户工作空间正在建设中。</p><div class="points">01 · 探索模型<br>02 · 管理服务<br>03 · 接入应用</div></section><section class="panel"><div class="label">WELCOME BACK</div><h2>欢迎回来</h2><p>登录品牌站，继续你的工作。</p><div class="notice" role="status"><strong>品牌身份认证尚未开通</strong><span>此页面仅供预览，暂不能登录或注册；请勿提交账号或密码。</span></div><a class="back" href="/">返回首页</a></section></div></main></body>
</html>`))
	}
	c.Abort()
}
