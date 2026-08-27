package generic

import (
	_ "embed"
	"net/http"
)

//go:embed page.html
var pageHTML []byte

// ConfigPage GET /generic
// 返回通用适配器配置页
func (h *Handler) ConfigPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(pageHTML)
}