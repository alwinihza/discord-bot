package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alwinihza/discord-bot/bot"
	"github.com/alwinihza/discord-bot/database"
	"github.com/alwinihza/discord-bot/pkg/state"
)

var (
	killChannel       chan os.Signal
	paginationManager = state.NewPaginationManager()
)

func main() {
	// Initialize database connection
	db, err := database.NewDB(os.Getenv("SUPABASE_URL"))
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close(context.Background())

	pm := state.NewPaginationManager()

	// Initialize bot
	discordBot, err := bot.NewBot(os.Getenv("TOKEN"), db, pm)
	if err != nil {
		log.Fatalf("Bot initialization failed: %v", err)
	}

	// Start bot
	err = discordBot.Start()
	if err != nil {
		log.Fatalf("Cannot open session: %v", err)
	}
	defer discordBot.Stop()

	// Wait for termination signal
	killChannel = make(chan os.Signal, 1)
	signal.Notify(killChannel, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, syscall.SIGTERM)
	<-killChannel
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			paginationManager.Cleanup(24 * time.Hour)
		}
	}()

	log.Println("Terminating bot")
}
