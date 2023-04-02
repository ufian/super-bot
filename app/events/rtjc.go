package events

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/radio-t/super-bot/app/bot"
)

//go:generate moq --out mocks/submitter.go --pkg mocks --skip-ensure . submitter:Submitter
//go:generate moq --out mocks/openai_summary.go --pkg mocks --skip-ensure . openAISummary:OpenAISummary

// pinned defines translation map for messages pinned by bot
var pinned = map[string]string{
	"⚠️ Официальный кат! - https://stream.radio-t.com/": "⚠️ Вещание подкаста началось - https://stream.radio-t.com/",
}

// Rtjc is a listener for incoming rtjc commands. Publishes whatever it got from the socket
// compatible with the legacy rtjc bot. Primarily use case is to push news events from news.radio-t.com
type Rtjc struct {
	Port          int
	Submitter     submitter
	OpenAISummary openAISummary

	UrAPI    string
	UrToken  string
	URClient *http.Client
}

type SummaryItem struct {
	title   string
	content string
}

func (s SummaryItem) summary() (title string) {
	return s.title + "\n\n" + bot.EscapeMarkDownV1Text(s.content)
}

// submitter defines interface to submit (usually asynchronously) to the chat
type submitter interface {
	Submit(ctx context.Context, text string, pin bool) error
}

type openAISummary interface {
	Summary(text string) (response string, err error)
}

// SUMPREFIX Темы слушателей 852 - https://radio-t.com/p/2023/03/28/prep-852/

// Listen on Port accept and forward to telegram
func (l Rtjc) Listen(ctx context.Context) {
	log.Printf("[INFO] rtjc listener on port %d", l.Port)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", l.Port))
	if err != nil {
		log.Fatalf("[ERROR] can't listen on %d, %v", l.Port, err)
	}

	sendSummary := func(msg string) {
		if !strings.HasPrefix(msg, "⚠") && !strings.HasPrefix(msg, "SUMPREFIX") {
			return
		}
		items, err := l.getSummaryMessages(msg)
		if err != nil {
			log.Printf("[WARN] can't get summary, %v", err)
			return
		}
		for _, item := range items {
			if item.content == "" {
				log.Printf("[WARN] empty summary for %q", msg)
				return
			}
			if serr := l.Submitter.Submit(ctx, item.summary(), false); serr != nil {
				log.Printf("[WARN] can't send summary, %v", serr)
			}
		}
	}

	for {
		conn, e := ln.Accept()
		if e != nil {
			log.Printf("[WARN] can't accept, %v", e)
			time.Sleep(time.Second * 1)
			continue
		}
		if message, rerr := bufio.NewReader(conn).ReadString('\n'); rerr == nil {
			pin, msg := l.isPinned(message)
			if serr := l.Submitter.Submit(ctx, msg, pin); serr != nil {
				log.Printf("[WARN] can't send message, %v", serr)
			}
			sendSummary(msg)
		} else {
			log.Printf("[WARN] can't read message, %v", rerr)
		}
		_ = conn.Close()
	}
}

func (l Rtjc) isPinned(msg string) (ok bool, m string) {
	cleanedMsg := strings.TrimSpace(msg)
	cleanedMsg = strings.TrimSuffix(cleanedMsg, "\n")

	for k, v := range pinned {
		if strings.EqualFold(cleanedMsg, k) {
			resMsg := v
			if strings.TrimSpace(resMsg) == "" {
				resMsg = msg
			}
			return true, resMsg
		}
	}
	return false, msg
}

// summary returns short summary of the selected news article
func (l Rtjc) getSummaryMessages(msg string) (items []SummaryItem, err error) {
	log.Printf("[DEBUG] summary for message: %s", msg)

	re := regexp.MustCompile(`https?://[^\s"'<>]+`)
	link := re.FindString(msg)
	log.Printf("[DEBUG] Link found: %s", link)
	if strings.Contains(link, "radio-t.com") {
		return l.getSummaryMessagesFromComments(link)
		//return []SummaryItem{}, nil // ignore radio-t.com links
	}

	item, err := l.getSummaryByLink(link)
	if err != nil {
		return []SummaryItem{}, fmt.Errorf("can't get summary for %s: %w", link, err)
	}

	items = append(items, item)
	return items, nil
}

func (l Rtjc) getSummaryByLink(link string) (item SummaryItem, err error) {
	log.Printf("[DEBUG] summary for link:%s", link)

	rl := fmt.Sprintf("%s?token=%s&url=%s", l.UrAPI, l.UrToken, link)
	resp, err := l.URClient.Get(rl)
	if err != nil {
		return SummaryItem{}, fmt.Errorf("can't get summary for %s: %w", link, err)
	}
	defer resp.Body.Close() // nolint
	if resp.StatusCode != http.StatusOK {
		return SummaryItem{}, fmt.Errorf("can't get summary for %s: %d", link, resp.StatusCode)
	}

	urResp := struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}{}
	if decErr := json.NewDecoder(resp.Body).Decode(&urResp); decErr != nil {
		return SummaryItem{}, fmt.Errorf("can't decode summary for %s: %w", link, decErr)
	}

	res, err := l.OpenAISummary.Summary(urResp.Title + " - " + urResp.Content)
	if err != nil {
		return SummaryItem{}, fmt.Errorf("can't get summary for %s: %w", link, err)
	}

	return SummaryItem{
		title:   urResp.Title,
		content: res,
	}, nil
}

func (l Rtjc) getSummaryMessagesFromComments(remarkLink string) (items []SummaryItem, err error) {
	type Comment struct {
		ParentID string `json:"pid"`
		Text     string `json:"text"`
		Orig     string `json:"orig,omitempty"` // important: never render this as HTML! It's not sanitized.
		User     struct {
			Name     string `json:"name"`
			Admin    bool   `json:"admin"`
			Verified bool   `json:"verified,omitempty"`
			PaidSub  bool   `json:"paid_sub,omitempty"`
		} `json:"user"`
		Score     int       `json:"score"`
		Deleted   bool      `json:"delete,omitempty" bson:"delete"`
		Timestamp time.Time `json:"time" bson:"time"`
	}

	loadTopComments := func(remarkLink string) (comments []Comment, err error) {
		rl := fmt.Sprintf("https://remark42.radio-t.com/api/v1/find?site=radiot&url=%s&sort-score&format=plain", remarkLink)
		resp, err := l.URClient.Get(rl)
		if err != nil {
			return []Comment{}, fmt.Errorf("can't get comments for %s: %w", remarkLink, err)
		}
		defer resp.Body.Close() // nolint
		if resp.StatusCode != http.StatusOK {
			return []Comment{}, fmt.Errorf("can't get comments for %s: %d", remarkLink, resp.StatusCode)
		}

		urResp := struct {
			Comments []Comment `json:"comments"`
		}{}

		if decErr := json.NewDecoder(resp.Body).Decode(&urResp); decErr != nil {
			return []Comment{}, fmt.Errorf("can't decode comments for %s: %w", remarkLink, decErr)
		}

		for _, c := range urResp.Comments {
			if c.ParentID != "" || c.Deleted || c.Score < 0 {
				continue
			}

			comments = append(comments, c)
		}
		sort.Slice(comments, func(i, j int) bool {
			if comments[i].Score < comments[j].Score {
				return false
			}

			if comments[i].Score > comments[j].Score {
				return true
			}
			// Equal case
			return comments[i].Timestamp.Before(comments[j].Timestamp)
		})
		return comments, nil
	}

	log.Printf("[DEBUG] summary for Radio-t link: %s", remarkLink)

	re := regexp.MustCompile(`https?://radio-t.com/p/[^\s"'<>]+/prep-[0-9]+/`)
	if !re.MatchString(remarkLink) {
		return []SummaryItem{}, fmt.Errorf("radio-t link doesn't fit to format: %s", remarkLink) // ignore radio-t.com links
	}

	comments, err := loadTopComments(remarkLink)
	if err != nil {
		return []SummaryItem{}, fmt.Errorf("can't get comments for %s: %w", remarkLink, err)
	}

	reLink := regexp.MustCompile(`https?://[^\s"'<>]+`)
	for _, c := range comments {
		link := reLink.FindString(c.Text)

		if link == "" || strings.Contains(link, "radio-t.com") {
			item := SummaryItem{
				title:   fmt.Sprintf("*+%d* от *%s*", c.Score, bot.EscapeMarkDownV1Text(c.User.Name)),
				content: bot.EscapeMarkDownV1Text(c.Orig),
			}
			items = append(items, item)
			continue
		}

		item, err := l.getSummaryByLink(link)
		if err != nil {
			log.Printf("[WARN] can't get summary for %s: %v", link, err)
			continue
		}

		item.title = fmt.Sprintf("*+%d* от *%s*\n_%s_\n\n%s", c.Score, bot.EscapeMarkDownV1Text(c.User.Name), bot.EscapeMarkDownV1Text(c.Orig), bot.EscapeMarkDownV1Text(item.title))

		items = append(items, item)
	}

	return items, nil
}
