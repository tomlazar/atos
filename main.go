package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andersfylling/disgord"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2/clientcredentials"

	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

var (
	badTextRe = regexp.MustCompile(`( - |single| by | & |, )`)
	urlRe     = regexp.MustCompile(`https://music.apple.com/[^\s]+`)
)

type app struct {
	spotfiy spotify.Client
	discord *disgord.Client
}

func (a *app) Atos(ctx context.Context, appleMusicUrl string) ([]string, error) {
	if appleMusicUrl == "" {
		return nil, errors.New("apple music url is required")
	}

	log.Printf("apple music url: %s", appleMusicUrl)
	aurl, err := url.Parse(appleMusicUrl)
	if err != nil {
		return nil, err
	}

	if aurl.Host != "music.apple.com" {
		return nil, nil
	}

	res, err := http.Get(aurl.String())
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.New(res.Status)
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	var match string
	doc.Find(`meta[property="og:title"]`).Each(func(i int, s *goquery.Selection) {
		if match = s.AttrOr("content", ""); match != "" {
			s.End()
		}
	})
	match = badTextRe.ReplaceAllString(strings.ToLower(match), " ")

	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	sres, err := a.spotfiy.Search(match, spotify.SearchTypeTrack)
	if err != nil {
		return nil, err
	}

	matches := []string{}
	if sres.Tracks != nil {
		for _, t := range sres.Tracks.Tracks {
			matches = append(matches, "https://open.spotify.com/track/"+string(t.ID))
		}
	}

	return matches, nil
}

func (a *app) OnMessageCreate(s disgord.Session, m *disgord.MessageCreate) {
	if !urlRe.MatchString(m.Message.Content) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		content   = m.Message.Content
		matches   = urlRe.FindAllString(content, -1)
		madChange = false
	)

	for _, match := range matches {
		spotify, err := a.Atos(ctx, match)
		if err != nil {
			log.Printf("error getting spotify matches: %v", err)
			continue
		}
		if len(spotify) == 0 {
			log.Printf("no spotify matches for %s", match)
			continue
		}

		content = strings.Replace(content, match, spotify[0], -1)
		madChange = true
	}
	if !madChange {
		return
	}

	err := s.Channel(m.Message.ChannelID).Message(m.Message.ID).Delete()
	if err != nil {
		log.Printf("error deleting message: %v", err)
		return
	}

	s.Channel(m.Message.ChannelID).CreateMessage(&disgord.CreateMessageParams{
		Content: fmt.Sprintf("[%s] %s", m.Message.Author.Mention(), content),
	})
	return
}

func main() {
	var (
		spotiftClientID     = os.Getenv("SPOTIFY_CLIENT_ID")
		spotifyClientSecret = os.Getenv("SPOTIFY_CLIENT_SECRET")
		discordToken        = os.Getenv("DISCORD_TOKEN")
	)

	ctx := context.Background()

	config := &clientcredentials.Config{
		ClientID:     spotiftClientID,
		ClientSecret: spotifyClientSecret,
		TokenURL:     spotifyauth.TokenURL,
	}
	token, err := config.Token(ctx)
	if err != nil {
		log.Fatalf("couldn't get token: %v", err)
	}

	app := &app{
		spotfiy: spotify.NewClient(spotifyauth.New().Client(ctx, token)),
		discord: disgord.New(disgord.Config{
			BotToken:    discordToken,
			Intents:     disgord.IntentGuildMessages,
			ProjectName: "atos",
		}),
	}
	defer app.discord.Gateway().StayConnectedUntilInterrupted()

	log.Printf("starting up (press ctrl+c to exit)")
	// set up messengers
	app.discord.Gateway().MessageCreate(app.OnMessageCreate)
}
