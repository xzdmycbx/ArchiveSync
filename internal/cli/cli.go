// Package cli implements the archive-sync command-line interface.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"archivesync/internal/api"
	"archivesync/internal/auth"
	"archivesync/internal/backup"
	"archivesync/internal/config"
	"archivesync/internal/scheduler"
	"archivesync/internal/store"
	"archivesync/internal/version"

	"github.com/spf13/cobra"
)

var cfgPath string

// Execute runs the root command.
func Execute() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "archive-sync",
		Short:         "ArchiveSync — 备份同步系统与管理面板",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", config.DefaultPath(),
		"bootstrap 配置文件路径 (或环境变量 ARCHIVE_SYNC_CONFIG)")

	root.AddCommand(serveCmd(), statusCmd(), iamCmd(), configCmd(), backupCmd(), versionCmd())
	return root
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// loadConfig loads the config file, or returns applied defaults if it is absent
// (so `archive-sync serve` works with zero setup for local testing).
func loadConfig() (*config.Config, bool, error) {
	if _, err := os.Stat(cfgPath); err != nil {
		cfg := config.Default()
		cfg.ApplyDefaults()
		return cfg, false, nil
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, true, err
	}
	return cfg, true, nil
}

func mustLoadConfig() (*config.Config, bool) {
	cfg, existed, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取配置失败:", err)
		os.Exit(1)
	}
	return cfg, existed
}

func openStore(cfg *config.Config) store.Store {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "创建数据目录失败:", err)
		os.Exit(1)
	}
	cipher, err := cfg.Cipher()
	if err != nil {
		fmt.Fprintln(os.Stderr, "初始化加密失败:", err)
		os.Exit(1)
	}
	st, err := store.Open(cfg.DBPath(), cipher)
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开数据库失败:", err)
		os.Exit(1)
	}
	return st
}

func prompt(reader *bufio.Reader, label, current string) string {
	if current != "" {
		fmt.Printf("%s [%s]: ", label, current)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return current
	}
	return line
}

// ---------------------------------------------------------------------------
// serve
// ---------------------------------------------------------------------------

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "启动 ArchiveSync 服务（HTTP 面板 + 调度器）",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := newLogger()
			cfg, existed := mustLoadConfig()

			// Ensure a master key exists and ALWAYS persist it (creating the config
			// file if needed). Otherwise a run without a config file would generate a
			// fresh key every time and be unable to decrypt data written previously
			// ("cipher: message authentication failed").
			if gen, err := cfg.EnsureMasterKey(); err != nil {
				return err
			} else if gen {
				if err := cfg.Save(cfgPath); err != nil {
					log.Warn("无法保存生成的主密钥；重启后已加密的数据将无法解密", "path", cfgPath, "err", err)
				} else if !existed {
					log.Info("已生成主密钥并写入配置文件", "path", cfgPath)
				}
			}

			st := openStore(cfg)
			defer st.Close()
			if err := st.PruneExpiredSessions(context.Background()); err != nil {
				log.Warn("清理过期会话失败", "err", err)
			}

			engine := backup.NewEngine(st, cfg, log)
			sched := scheduler.New(st, engine, log)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			// Build the IAM authenticator when configured; otherwise dev mode.
			var authn *auth.Authenticator
			if cfg.IAM.ClientID != "" {
				actx, cancel := context.WithTimeout(ctx, 20*time.Second)
				a, err := auth.New(actx, cfg.IAM, st,
					time.Duration(cfg.SessionTTLHours)*time.Hour,
					strings.HasPrefix(cfg.BaseURL, "https"))
				cancel()
				if err != nil {
					return fmt.Errorf("初始化 IAM 认证失败（请检查 issuer 与网络连通性）: %w", err)
				}
				authn = a
			} else {
				log.Warn("未配置 IAM ClientID —— 以开发模式运行（无认证，请勿用于生产）")
			}

			srv := api.New(cfg, st, engine, sched, authn, log)

			if err := sched.Start(ctx); err != nil {
				log.Warn("调度器启动失败", "err", err)
			}

			httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv.Handler()}
			go func() {
				mode := "IAM 认证"
				if srv.DevMode() {
					mode = "开发模式（无认证，仅供本地测试）"
				}
				log.Info("ArchiveSync 已启动", "listen", cfg.Listen, "mode", mode, "version", version.Version)
				fmt.Printf("\n  ArchiveSync 管理面板: %s  (%s)\n\n", cfg.BaseURL, mode)
				if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Error("HTTP 服务异常", "err", err)
					stop()
				}
			}()

			<-ctx.Done()
			log.Info("正在关闭…")
			sched.Stop()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return httpSrv.Shutdown(shutdownCtx)
		},
	}
}

// ---------------------------------------------------------------------------
// status
// ---------------------------------------------------------------------------

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看备份目标、调度与最近的备份记录",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := mustLoadConfig()
			st := openStore(cfg)
			defer st.Close()
			ctx := context.Background()

			fmt.Println(version.String())
			fmt.Printf("配置文件: %s\n数据目录: %s\n", cfgPath, cfg.DataDir)
			if cfg.IAM.ClientID == "" {
				fmt.Println("认证模式: 开发模式（未配置 IAM）")
			} else {
				fmt.Printf("认证模式: IAM (%s, app=%s)\n", cfg.IAM.Issuer, cfg.IAM.AppKey)
			}

			channels, _ := st.ListChannels(ctx)
			notifiers, _ := st.ListNotifiers(ctx)
			targets, _ := st.ListTargets(ctx)
			fmt.Printf("\n渠道: %d 个   通知: %d 个   目标: %d 个\n", len(channels), len(notifiers), len(targets))

			fmt.Println("\n备份目标:")
			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "  名称\t状态\t下次运行\t最近结果\t大小")
			for _, t := range targets {
				state := "启用"
				next := "—"
				if !t.Enabled {
					state = "停用"
				} else if nr := scheduler.ComputeNext(t.Schedule, time.Now()); !nr.IsZero() {
					next = nr.Local().Format("01-02 15:04")
				}
				last := "从未"
				size := "—"
				if runs, err := st.ListRuns(ctx, t.ID, 1); err == nil && len(runs) > 0 {
					last = string(runs[0].Status)
					size = humanBytes(runs[0].SizeBytes)
				}
				fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", t.Name, state, next, last, size)
			}
			tw.Flush()

			if runs, err := st.RecentRuns(ctx, 8); err == nil && len(runs) > 0 {
				fmt.Println("\n最近备份:")
				tw2 := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw2, "  时间\t目标\t状态\t大小\t文件")
				for _, r := range runs {
					fmt.Fprintf(tw2, "  %s\t%s\t%s\t%s\t%d\n",
						r.StartedAt.Local().Format("01-02 15:04"), r.TargetName, r.Status,
						humanBytes(r.SizeBytes), r.FileCount)
				}
				tw2.Flush()
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// iam
// ---------------------------------------------------------------------------

func iamCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "iam",
		Short: "交互式配置 TransCircle IAM 接入信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := mustLoadConfig()
			reader := bufio.NewReader(os.Stdin)
			fmt.Println("配置 TransCircle IAM（直接回车保留当前值）:")
			fmt.Println()

			cfg.IAM.Issuer = prompt(reader, "Issuer", cfg.IAM.Issuer)
			cfg.IAM.ClientID = prompt(reader, "Client ID", cfg.IAM.ClientID)
			cfg.IAM.ClientSecret = prompt(reader, "Client Secret（公共客户端可留空）", cfg.IAM.ClientSecret)
			cfg.BaseURL = prompt(reader, "面板外部地址 Base URL", cfg.BaseURL)
			def := cfg.BaseURL + "/api/auth/callback"
			if cfg.IAM.RedirectURL == "" {
				cfg.IAM.RedirectURL = def
			}
			cfg.IAM.RedirectURL = prompt(reader, "Redirect URL", cfg.IAM.RedirectURL)
			cfg.IAM.AppKey = prompt(reader, "应用 Key (tc_app)", cfg.IAM.AppKey)
			cfg.IAM.RequiredPermission = prompt(reader, "要求权限（可留空）", cfg.IAM.RequiredPermission)
			cfg.IAM.RequiredRole = prompt(reader, "要求角色（可留空）", cfg.IAM.RequiredRole)

			if _, err := cfg.EnsureMasterKey(); err != nil {
				return err
			}
			cfg.ApplyDefaults()
			if err := cfg.Save(cfgPath); err != nil {
				return err
			}
			fmt.Printf("\n已保存到 %s\n", cfgPath)
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

func configCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "查看或修改 bootstrap 配置"}

	c.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "打印当前配置（敏感字段已脱敏）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, existed := mustLoadConfig()
			if !existed {
				fmt.Printf("（%s 不存在，以下为默认值）\n\n", cfgPath)
			}
			mask := func(s string) string {
				if s == "" {
					return "(空)"
				}
				return "********"
			}
			fmt.Printf("config:        %s\n", cfgPath)
			fmt.Printf("listen:        %s\n", cfg.Listen)
			fmt.Printf("data_dir:      %s\n", cfg.DataDir)
			fmt.Printf("base_url:      %s\n", cfg.BaseURL)
			fmt.Printf("master_key:    %s\n", mask(cfg.MasterKey))
			fmt.Printf("session_ttl_h: %d\n", cfg.SessionTTLHours)
			fmt.Printf("iam.issuer:    %s\n", cfg.IAM.Issuer)
			fmt.Printf("iam.client_id: %s\n", cfg.IAM.ClientID)
			fmt.Printf("iam.secret:    %s\n", mask(cfg.IAM.ClientSecret))
			fmt.Printf("iam.redirect:  %s\n", cfg.IAM.RedirectURL)
			fmt.Printf("iam.app_key:   %s\n", cfg.IAM.AppKey)
			fmt.Printf("iam.req_perm:  %s\n", cfg.IAM.RequiredPermission)
			fmt.Printf("iam.req_role:  %s\n", cfg.IAM.RequiredRole)
			return nil
		},
	})

	c.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "设置一个配置项并保存",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := mustLoadConfig()
			if err := setConfigKey(cfg, args[0], args[1]); err != nil {
				return err
			}
			if _, err := cfg.EnsureMasterKey(); err != nil {
				return err
			}
			cfg.ApplyDefaults()
			if err := cfg.Save(cfgPath); err != nil {
				return err
			}
			fmt.Printf("已更新 %s\n", args[0])
			return nil
		},
	})
	return c
}

func setConfigKey(cfg *config.Config, key, val string) error {
	switch key {
	case "listen":
		cfg.Listen = val
	case "data_dir":
		cfg.DataDir = val
	case "base_url":
		cfg.BaseURL = val
	case "session_ttl_hours":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("session_ttl_hours 需要整数")
		}
		cfg.SessionTTLHours = n
	case "iam.issuer":
		cfg.IAM.Issuer = val
	case "iam.client_id":
		cfg.IAM.ClientID = val
	case "iam.client_secret":
		cfg.IAM.ClientSecret = val
	case "iam.redirect_url":
		cfg.IAM.RedirectURL = val
	case "iam.app_key":
		cfg.IAM.AppKey = val
	case "iam.required_permission":
		cfg.IAM.RequiredPermission = val
	case "iam.required_role":
		cfg.IAM.RequiredRole = val
	default:
		return fmt.Errorf("未知的配置项: %s", key)
	}
	return nil
}

// ---------------------------------------------------------------------------
// backup
// ---------------------------------------------------------------------------

func backupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backup <目标名称或ID>",
		Short: "立即执行一次指定目标的备份",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := newLogger()
			cfg, _ := mustLoadConfig()
			st := openStore(cfg)
			defer st.Close()
			engine := backup.NewEngine(st, cfg, log)

			fmt.Printf("开始备份: %s …\n", args[0])
			run, err := engine.RunByName(context.Background(), args[0], "manual")
			if err != nil {
				return err
			}
			fmt.Printf("\n结果: %s\n%s\n", run.Status, run.Message)
			for _, d := range run.Destinations {
				status := "✓"
				if !d.Success {
					status = "✗ " + d.Error
				}
				fmt.Printf("  [%s] %s  key=%s  清理=%d\n", status, d.ChannelName, d.Key, d.Pruned)
			}
			if run.Status == "failed" {
				os.Exit(2)
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// version
// ---------------------------------------------------------------------------

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "打印版本信息",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.String())
		},
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
