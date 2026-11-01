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

	discordBot, err := bot.NewBot(token, db, pm, os.Getenv("ANNOUNCE_CHANNEL_ID"), youtube.NewClient(os.Getenv("YOUTUBE_API_KEY")))
	if err != nil {
		return err
	}
	if err := discordBot.Start(); err != nil {
		return err
	}
	defer discordBot.Stop()

	log.Println("Bot is running, press Ctrl+C to stop")
	<-ctx.Done()
	log.Println("Shutting down")
	return nil
}
