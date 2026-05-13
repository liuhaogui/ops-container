package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liuhaogui/ops-container/internal/config"
	"go.uber.org/zap"
)

// SecretHolder 持有从 ops-api 拉取的本机 secret，纯内存，不落盘。
// 当连续 token 验证失败次数达到阈值时自动触发 re-fetch。
type SecretHolder struct {
	secret         atomic.Value // string
	failCount      atomic.Int32
	refreshThresh  int32
	refreshing     atomic.Bool
	mu             sync.Mutex
	cfgManager     interface{ Current() config.Config }
	log            *zap.Logger
}

func NewSecretHolder(cfgManager interface{ Current() config.Config }, log *zap.Logger) *SecretHolder {
	return &SecretHolder{
		refreshThresh: 3,
		cfgManager:    cfgManager,
		log:           log,
	}
}

func (h *SecretHolder) Get() string {
	v := h.secret.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

func (h *SecretHolder) set(s string) {
	h.secret.Store(s)
}

// RecordFailure 记录一次 token 验证失败；连续失败达到阈值后异步触发 re-fetch。
func (h *SecretHolder) RecordFailure() {
	n := h.failCount.Add(1)
	if n >= h.refreshThresh {
		h.failCount.Store(0)
		go h.refresh()
	}
}

// RecordSuccess 重置连续失败计数。
func (h *SecretHolder) RecordSuccess() {
	h.failCount.Store(0)
}

func (h *SecretHolder) refresh() {
	if !h.refreshing.CompareAndSwap(false, true) {
		return
	}
	defer h.refreshing.Store(false)
	cfg := h.cfgManager.Current()
	if err := FetchAndHold(cfg, h, h.log); err != nil {
		h.log.Error("re-fetch container secret failed", zap.Error(err))
	}
}

// FetchAndHold 从 ops-api 拉取本机 secret 并存入 holder。
// ops_api.url 已配置时拉取失败直接返回错误，调用方应拒绝启动。
func FetchAndHold(cfg config.Config, holder *SecretHolder, log *zap.Logger) error {
	url := strings.TrimSpace(cfg.OpsAPI.URL)
	if url == "" {
		return nil
	}
	timeout := cfg.OpsAPI.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/v1/container/secret", nil)
	if err != nil {
		return fmt.Errorf("fetch container secret: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch container secret: request to %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch container secret: ops-api returned %d (IP not registered?)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("fetch container secret: read body: %w", err)
	}

	var payload struct {
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("fetch container secret: parse response: %w", err)
	}

	secret := strings.TrimSpace(payload.Data.Secret)
	if secret == "" {
		return fmt.Errorf("fetch container secret: ops-api returned empty secret")
	}

	holder.set(secret)
	log.Info(fmt.Sprintf("container secret fetched from ops-api (%s)", url))
	return nil
}
