package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5"
)

// Bot parameters
var (
	RemoveCommands = true
)

var s *discordgo.Session
var conn *pgx.Conn

var (
	killChannel chan os.Signal
)

func init() {

	var err error
	s, err = discordgo.New("Bot " + os.Getenv("TOKEN"))
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}
	connectionString := os.Getenv("SUPABASE_URL")
	conn, err = pgx.Connect(context.Background(), connectionString)
	if err != nil {
		fmt.Println("Error connecting to the database:", err)
		return
	}
}

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name: "list-video",
			// All commands and options must have a description
			// Commands/options without description will fail the registration
			// of the command.
			Description: "List Karaoke Video",
		},
		{
			Name:        "random",
			Description: "Get a random song by Fiona",
		},
		{
			Name:        "search",
			Description: "Search the song",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "search",
					Description: "Title",
					Required:    true,
				},
			},
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"list-video": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			fmt.Println("test video")
			rows, err := conn.Query(context.Background(), "SELECT video_url,video_title FROM karaoke_video")
			if err != nil {
				panic(err)
			}
			var videoList string
			for rows.Next() {
				var videoTitle, videoUrl string
				err = rows.Scan(&videoUrl, &videoTitle)
				videoList += fmt.Sprintf("%s - %s\n", videoUrl, videoTitle)
			}
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: videoList,
				},
			})
		},
		"random": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			var videoTitle, videoUrl, songTitle, timestamp string
			err := conn.QueryRow(context.Background(), "SELECT song_title,timestamp,video_title,video_url FROM karaoke_song s JOIN karaoke_video v on v.id=s.video_id and not v.membership ORDER BY RANDOM() LIMIT 1").
				Scan(&songTitle, &timestamp, &videoTitle, &videoUrl)
			if err != nil {
				panic(err)
			}
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("%s \nhttps://youtu.be/%s?t=%d", songTitle, videoUrl, timestampToSeconds(timestamp)),
				},
			})
		},
		"search": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			// Access options in the order provided by the user.
			options := i.ApplicationCommandData().Options
			fields := []*discordgo.MessageEmbedField{}
			searchResult := make(map[string][]VideoSong)

			// Or convert the slice into a map
			optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
			for _, opt := range options {
				optionMap[opt.Name] = opt
			}

			// Get the value from the option map.
			// When the option exists, ok = true
			if option, ok := optionMap["search"]; ok {
				// Option values must be type asserted from interface{}.
				// Discordgo provides utility functions to make this simple.
				search := option.StringValue()
				searchValue := "%" + search + "%"
				log.Println(searchValue)
				rows, err := conn.Query(context.Background(), "SELECT song_title,timestamp,video_title,video_url FROM karaoke_song s JOIN karaoke_video v on v.id=s.video_id WHERE s.song_title ILIKE $1", searchValue)
				if err != nil {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: err.Error(),
						},
					})
				}
				for rows.Next() {
					var videoTitle, videoUrl, songTitle, timestamp string
					rows.Scan(&songTitle, &timestamp, &videoTitle, &videoUrl)
					temp := VideoSong{
						title: songTitle,
						url:   fmt.Sprintf("https://youtu.be/%s?t=%d", videoUrl, timestampToSeconds(timestamp)),
					}
					searchResult[videoTitle] = append(searchResult[videoTitle], temp)
				}
			}
			for k, s := range searchResult {
				value := ""
				for _, v := range s {
					value += fmt.Sprintf("[%s](%s) \n", v.title, v.url)
				}
				fields = append(fields, &discordgo.MessageEmbedField{
					Name:   k,
					Value:  value,
					Inline: false,
				})
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				// Ignore type for now, they will be discussed in "responses"
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Embeds: []*discordgo.MessageEmbed{
						{
							Title:  "Search Result",
							Fields: fields,
						},
					},
				},
			})
		},
	}
)

func init() {
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
}
func main() {
	defer conn.Close(context.Background())
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		fmt.Println("Bot is ready")
	})
	err := s.Open()
	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}

	log.Println("Adding commands...")

	// Just like the ping pong example, we only care about receiving message
	// events in this example.
	s.Identify.Intents = discordgo.IntentsGuildMessages
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := s.ApplicationCommandCreate(s.State.User.ID, "", v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}

	defer s.Close()

	// Wait for the bot to be killed
	killChannel = make(chan os.Signal, 1)
	signal.Notify(killChannel, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-killChannel

	log.Println("Terminating bot")
}

func timestampToSeconds(timestamp string) int {
	parts := strings.Split(timestamp, ":")
	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])

	totalSeconds := hours*3600 + minutes*60 + seconds
	return totalSeconds
}
