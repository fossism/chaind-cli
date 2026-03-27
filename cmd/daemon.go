package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	
	"github.com/fossism/chaind-cli/internal/adapters"
	"github.com/fossism/chaind-cli/internal/auth"
	"github.com/fossism/chaind-cli/internal/daemon"
	"github.com/fossism/chaind-cli/internal/ipc"
	"github.com/fossism/chaind-cli/internal/store"
	"golang.org/x/sync/errgroup"
	
	"strings"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the chaind background sync daemon",
	Run: func(cmd *cobra.Command, args []string) {
		setupLogger()

		log.Info().Msg("Starting chaind daemon...")

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		// Initialize Database Store
		dbStore, err := store.NewStore()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to initialize database store")
		}
		defer dbStore.Close()

		// Initialize Adapter Router
		router := daemon.NewAdapterRouter()

		// Initialize IPC Server
		ipcServer := ipc.NewIPCServer(dbStore, router)

		g, gCtx := errgroup.WithContext(ctx)

		// Start StoreWriter for single-threaded sqlite inserts
		g.Go(func() error {
			dbStore.StartWriter(gCtx)
			return nil
		})

		// Start IPC Server
		g.Go(func() error {
			return ipcServer.Start(gCtx)
		})

		// Initialize Matrix Adapter
		homeServer := "https://matrix.org"
		userID := "@example:matrix.org"
		accessToken, authErr := auth.GetCredential("matrix") 
		if authErr == nil {
			matrixAdapter, err := adapters.NewMatrixAdapter(dbStore, homeServer, userID, strings.TrimSpace(accessToken))
			if err == nil {
				// Launch Matrix under dedicated supervisor isolated from root errgroup
				go supervise(gCtx, "matrix", matrixAdapter, router)
			} else {
				log.Warn().Err(err).Msg("Matrix adapter failed to initialize")
			}
		} else {
			log.Warn().Msg("Matrix token not found in keyring, skipping")
		}

		// Initialize Telegram Adapter
		tgApiID := "12345"
		tgApiHash := "example_hash"
		tgToken, authErr := auth.GetCredential("telegram")
		if authErr == nil {
			telegramAdapter, err := adapters.NewTelegramAdapter(dbStore, tgApiID, tgApiHash, strings.TrimSpace(tgToken))
			if err == nil {
				go supervise(gCtx, "telegram", telegramAdapter, router)
			} else {
				log.Warn().Err(err).Msg("Telegram adapter failed to initialize")
			}
		} else {
			log.Warn().Msg("Telegram token not found in keyring, skipping")
		}

		// Wait for shutdown signal or sub-process error
		if err := g.Wait(); err != nil {
			log.Error().Err(err).Msg("Daemon exited with error")
		} else {
			log.Info().Msg("Shutting down cleanly...")
		}
	},
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon explicitly",
	Run: daemonCmd.Run,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.AddCommand(daemonStartCmd)
}

func setupLogger() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	// Human readable to stderr instead of raw JSON for local daemon
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

// supervise runs an adapter in an isolated exponential backoff loop
func supervise(ctx context.Context, name string, starter adapters.Adapter, router *daemon.AdapterRouter) {
	backoff := 5 * time.Second
	for {
		log.Info().Str("adapter", name).Msg("connecting")
		
		// Register it so IPC can hit it
		router.Register(starter)
		
		if err := starter.Start(ctx); err != nil {
			router.Unregister(starter.Platform())
			
			log.Error().Str("adapter", name).Err(err).Dur("retry_in", backoff).Msg("adapter failed")
			select {
			case <-time.After(backoff):
				// max 5 minutes
				if backoff < 5*time.Minute {
					backoff = backoff * 2
				}
				if backoff > 5*time.Minute {
					backoff = 5 * time.Minute
				}
			case <-ctx.Done():
				return
			}
		} else {
			router.Unregister(starter.Platform())
			backoff = 5 * time.Second
		}
	}
}
