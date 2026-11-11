// dsh-ee 服务端入口。
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/hikariming/dsh-ee/server/internal/datahub"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/config"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
	"github.com/hikariming/dsh-ee/server/internal/webui"
)

// version 由构建时 -ldflags "-X main.version=…" 写入。
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version)
		return
	}
	if handled, code := runCLI(os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("启动失败", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	created, err := api.Bootstrap(ctx, st, cfg.BootstrapAdmin, cfg.BootstrapAdminPassword)
	if err != nil {
		return err
	}
	if created {
		log.Info("已创建初始管理员", "username", cfg.BootstrapAdmin)
	} else if n, _ := st.CountUsers(ctx); n == 0 {
		log.Warn("还没有任何用户：设置 DSH_EE_BOOTSTRAP_ADMIN_PASSWORD 后重启以创建管理员")
	}

	box, err := secret.NewBox(cfg.SecretKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("创建数据目录 %s: %w", cfg.DataDir, err)
	}
	if err := st.FailRunningJobs(ctx); err != nil {
		return err
	}
	// 数据中台：开不起来时只关掉这一块，其余照常
	var dataHub *datahub.Hub
	dataErr := ""
	if cfg.DataDSN != "" {
		if dataHub, err = api.OpenDataHub(ctx, st, box, cfg.DataDSN, cfg.DataReadDSN); err != nil {
			log.Warn("数据中台没有开起来", "err", err)
			dataErr = "数据中台没有开起来：" + err.Error()
		} else if dataHub.ReadErr != "" {
			log.Warn("数据中台的只读账号不可用，数据集暂时不能查", "reason", dataHub.ReadErr)
		}
	}
	srv := api.New(st, box, api.Options{
		CookieSecure:  cfg.CookieSecure,
		SessionTTL:    cfg.SessionTTL,
		Version:       version,
		Web:           webui.Handler(),
		DataDir:       cfg.DataDir,
		PublicURL:     cfg.PublicURL,
		Addr:          cfg.Addr,
		AdminModeUnit: cfg.AdminModeUnit,
		DataHub:       dataHub,
		DataHubErr:    dataErr,
		DemoSite:      demoSite(cfg),
	}, log)

	go sweepSessions(ctx, st, log)
	go srv.RunBackground(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// 不设 WriteTimeout：流式回答可能持续数分钟
	}
	httpSrv.RegisterOnShutdown(srv.CloseStreams)
	errCh := make(chan error, 1)
	go func() {
		log.Info("dsh-ee 已启动", "addr", cfg.Addr, "version", version)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("正在关闭")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
	return nil
}

// openStore 连接数据库；连不上时在 DBWait 内每 3 秒重试一次。
func openStore(ctx context.Context, cfg *config.Config, log *slog.Logger) (*store.Store, error) {
	deadline := time.Now().Add(cfg.DBWait)
	for {
		st, err := store.Open(ctx, cfg.DSN)
		if err == nil {
			return st, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return nil, fmt.Errorf("连接数据库: %w", err)
		}
		log.Warn("数据库还没就绪，3 秒后重试", "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// sweepSessions 每小时清理过期会话与设备码。
func sweepSessions(ctx context.Context, st *store.Store, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := st.DeleteExpiredSessions(ctx); err != nil {
				log.Warn("清理过期会话失败", "err", err)
			} else if n > 0 {
				log.Info("已清理过期会话", "count", n)
			}
			if _, err := st.DeleteExpiredAgentConfirms(ctx); err != nil {
				log.Warn("清理过期确认码失败", "err", err)
			}
			if n, err := st.DeleteExpiredDeviceCodes(ctx); err != nil {
				log.Warn("清理过期设备码失败", "err", err)
			} else if n > 0 {
				log.Info("已清理过期设备码", "count", n)
			}
		}
	}
}

// demoSite 把配置里的演示站开关转成服务端选项；没开时返回 nil。
func demoSite(cfg *config.Config) *api.DemoSite {
	if !cfg.DemoMode {
		return nil
	}
	return &api.DemoSite{Username: cfg.BootstrapAdmin, Password: cfg.BootstrapAdminPassword, Reset: cfg.DemoReset, ResetEvery: cfg.DemoResetEvery, VideoDir: cfg.DemoVideoDir, StatsFile: cfg.DemoStatsFile}
}
