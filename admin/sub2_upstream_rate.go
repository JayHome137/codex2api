package admin

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
)

// ProbeSub2APIUpstreamRate performs an explicit, read-only multiplier probe.
func (h *Handler) ProbeSub2APIUpstreamRate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		err = strconv.ErrSyntax
	}
	if err != nil {
		writeError(c, http.StatusBadRequest, "无效的账号 ID")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	row, err := h.db.GetAccountByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(c, http.StatusNotFound, "账号不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "读取账号失败: "+err.Error())
		return
	}
	if !strings.EqualFold(strings.TrimSpace(row.GetCredential("upstream_type")), auth.UpstreamOpenAIResponses) {
		writeError(c, http.StatusBadRequest, "该账号不是可探查上游倍率的账号")
		return
	}
	baseURL, apiKey := row.GetCredential("base_url"), row.GetCredential("api_key")
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(apiKey) == "" {
		writeError(c, http.StatusBadRequest, "账号缺少上游地址或 API Key")
		return
	}
	multiplier, ok := proxy.QuerySub2EffectiveRateMultiplier(baseURL, apiKey, row.ProxyURL, row.GetCredentialStringMap("custom_headers"))
	now := time.Now().UTC().Format(time.RFC3339)
	updates := map[string]interface{}{"sub2_upstream_rate_probe_at": now}
	if ok {
		updates["sub2_upstream_rate_multiplier"] = multiplier
		updates["sub2_upstream_rate_success_at"] = now
		updates["sub2_upstream_rate_probe_error"] = ""
	} else {
		updates["sub2_upstream_rate_probe_error"] = "上游未返回有效倍率"
	}
	if err := h.db.UpdateCredentials(ctx, id, updates); err != nil {
		writeError(c, http.StatusInternalServerError, "保存探查结果失败: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": row.GetCredentialBool("sub2_upstream_rate_probe_enabled"), "available": ok, "multiplier": multiplier, "probed_at": now, "error": updates["sub2_upstream_rate_probe_error"]})
}
