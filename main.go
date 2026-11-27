package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alwinihza/discord-bot/bot"
	"github.com/alwinihza/discord-bot/database"
	"github.com/alwinihza/discord-bot/internal/commissions"
	"github.com/alwinihza/discord-bot/internal/golive"
	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/alwinihza/discord-bot/internal/webhook"
	"github.com/alwinihza/discord-bot/internal/youtube"
	"github.com/alwinihza/discord-bot/pkg/state"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
	log.Println("Bot stopped")
}

func run() error {
	token, dbPath := os.Getenv("TOKEN"), os.Getenv("DATABASE_PATH")
	if token == "" || dbPath == "" {
		return errors.New("TOKEN and DATABASE_PATH must be set")
	}

	if os.Getenv("YOUTUBE_API_KEY") == "" {
		log.Println("YOUTUBE_API_KEY is not set, /setlist cannot add new streams")
	}

	var siteClient *site.Client
	if url, secret := os.Getenv("SITE_ADMIN_URL"), os.Getenv("SITE_ADMIN_SECRET"); url != "" && secret != "" {
		siteClient = site.NewClient(url, secret, envOr("SITE_PUBLIC_URL", "https://fionastation.vercel.app"))
	} else {
		log.Println("SITE_ADMIN_URL or SITE_ADMIN_SECRET is not set, /playlist-mod and /playlist are disabled")
	}

	var commissionsConfig *bot.CommissionsConfig
	if channel := os.Getenv("COMMISSIONS_CHANNEL_ID"); channel == "" {
		log.Println("COMMISSIONS_CHANNEL_ID is not set, commission posting is disabled")
	} else if siteClient == nil {
		log.Println("COMMISSIONS_CHANNEL_ID is set but SITE_ADMIN_URL or SITE_ADMIN_SECRET is not, commission posting is disabled")
	} else {
		managers, bad := commissions.ParseManagerIDs(os.Getenv("COMMISSION_MANAGER_IDS"))
		for _, id := range bad {
			log.Printf("COMMISSION_MANAGER_IDS: ignoring %q, it is not a Discord user id", id)
		}
		if len(managers) == 0 {
			log.Println("COMMISSION_MANAGER_IDS has no valid ids, nobody can press commission buttons")
		}
		commissionsConfig = &bot.CommissionsConfig{ChannelID: channel, Managers: managers}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	pm := state.NewPaginationManager()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pm.Cleanup(24 * time.Hour)
			}
		}
	}()

	yt := youtube.NewClient(os.Getenv("YOUTUBE_API_KEY"))
	discordBot, err := bot.NewBot(token, db, pm, os.Getenv("ANNOUNCE_CHANNEL_ID"), yt, siteClient, commissionsConfig)
	if err != nil {
		return err
	}
	if err := discordBot.Start(); err != nil {
		return err
	}
	defer discordBot.Stop()

	secret, liveChannel, ytChannel := os.Getenv("WEBHOOK_SECRET"), os.Getenv("LIVE_CHANNEL_ID"), os.Getenv("YOUTUBE_CHANNEL_ID")
	if secret == "" || liveChannel == "" || ytChannel == "" {
		log.Println("WEBHOOK_SECRET, LIVE_CHANNEL_ID or YOUTUBE_CHANNEL_ID is not set, the OBS go-live webhook is disabled")
	} else {
		announcer := golive.New(golive.Config{
			ChannelID: liveChannel, YouTubeChannelID: ytChannel, PingRoleID: os.Getenv("LIVE_PING_ROLE_ID"),
			SearchInterval: golive.SearchInterval, SearchWindow: golive.SearchWindow,
		}, golive.Session{Session: discordBot.Session()}, yt)
		go announcer.Run(ctx)
		addr := envOr("WEBHOOK_ADDR", ":8080")
		handler := webhook.Handler(secret, announcer.OnEvent)
		go func() {
			log.Printf("OBS go-live webhook listening on %s", addr)
			if err := webhook.Serve(ctx, addr, handler); err != nil {
				log.Printf("OBS go-live webhook stopped: %v", err)
			}
		}()
	}

	log.Println("Bot is running, press Ctrl+C to stop")
	<-ctx.Done()
	log.Println("Shutting down")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
