package generic

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Handler 通用适配器的 HTTP 处理器
type Handler struct {
	agent *Agent
	store ConfigStore
}

// NewHandler 创建通用适配器 HTTP 处理器
func NewHandler(agent *Agent, store ConfigStore) *Handler {
	return &Handler{agent: agent, store: store}
}

// writeJSON 输出 JSON 响应
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError 输出错误响应
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ListPlatforms GET /api/generic/platforms
// 返回平台配置列表，附带最新快照状态
func (h *Handler) ListPlatforms(w http.ResponseWriter, r *http.Request) {
	platforms, err := LoadPlatforms(h.store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type platformWithStatus struct {
		PlatformConfig
		Snapshot *PlatformSnapshot `json:"snapshot,omitempty"`
	}
	result := make([]platformWithStatus, 0, len(platforms))
	for _, p := range platforms {
		item := platformWithStatus{PlatformConfig: p}
		if s, ok := h.agent.GetSnapshot(p.Name); ok {
			item.Snapshot = s
		}
		result = append(result, item)
	}
	writeJSON(w, http.StatusOK, result)
}

// GetMetrics GET /api/generic/metrics
// 返回全部平台的最新指标快照
func (h *Handler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.agent.GetAllSnapshots())
}

// GetPlatformMetrics GET /api/generic/metrics/{platform}
func (h *Handler) GetPlatformMetrics(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/generic/metrics/")
	if name == "" || name == r.URL.Path {
		writeError(w, http.StatusBadRequest, "缺少平台名称")
		return
	}
	s, ok := h.agent.GetSnapshot(name)
	if !ok {
		writeError(w, http.StatusNotFound, "平台不存在或尚无数据")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// UpsertPlatform POST /api/generic/platforms
// 新增或更新平台配置
func (h *Handler) UpsertPlatform(w http.ResponseWriter, r *http.Request) {
	var p PlatformConfig
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if p.Name == "" {
		writeError(w, http.StatusBadRequest, "平台名称不能为空")
		return
	}
	if len(p.Sources) == 0 {
		writeError(w, http.StatusBadRequest, "至少需要一个数据源")
		return
	}

	platforms, err := LoadPlatforms(h.store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	replaced := false
	for i := range platforms {
		if platforms[i].Name == p.Name {
			platforms[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		platforms = append(platforms, p)
	}

	if err := SavePlatforms(h.store, platforms); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 保存后立即触发一次轮询
	go h.agent.pollPlatform(context.Background(), p)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeletePlatform DELETE /api/generic/platforms/{name}
func (h *Handler) DeletePlatform(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/generic/platforms/")
	if name == "" || name == r.URL.Path {
		writeError(w, http.StatusBadRequest, "缺少平台名称")
		return
	}

	platforms, err := LoadPlatforms(h.store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filtered := platforms[:0]
	for _, p := range platforms {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == len(platforms) {
		writeError(w, http.StatusNotFound, "平台不存在")
		return
	}

	if err := SavePlatforms(h.store, filtered); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// TestConnection POST /api/generic/test
// 测试连接：请求接口并返回映射结果
func (h *Handler) TestConnection(w http.ResponseWriter, r *http.Request) {
	var p PlatformConfig
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if p.Name == "" {
		writeError(w, http.StatusBadRequest, "平台名称不能为空")
		return
	}

	result, err := h.agent.TestConnection(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}