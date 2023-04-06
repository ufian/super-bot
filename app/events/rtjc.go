package events

import (
	"bufio"
	"context"
	"fmt"
	"github.com/radio-t/super-bot/app/bot/openai"
	"log"
	"net"
	"regexp"
	"strings"
	"time"
)

//go:generate moq --out mocks/submitter.go --pkg mocks --skip-ensure . submitter:Submitter
//go:generate moq --out mocks/summarizer.go --pkg mocks --skip-ensure . summarizer:Summarizer
//go:generate moq --out mocks/remark.go --pkg mocks --skip-ensure . remark:RemarkClient

// pinned defines translation map for messages pinned by bot
var pinned = map[string]string{
	"⚠️ Официальный кат! - https://stream.radio-t.com/": "⚠️ Вещание подкаста началось - https://stream.radio-t.com/",
}

// Rtjc is a listener for incoming rtjc commands. Publishes whatever it got from the socket
// compatible with the legacy rtjc bot. Primarily use case is to push news events from news.radio-t.com
type Rtjc struct {
	Port      int
	Submitter submitter

	Summarizer   summarizer
	RemarkClient remark
}

// submitter defines interface to submit (usually asynchronously) to the chat
type submitter interface {
	Submit(ctx context.Context, text string, pin bool) error
	SubmitHTML(ctx context.Context, text string, pin bool) error
	WaitMessageQueue() error
}

type summarizer interface {
	Summary(link string) (string, error)
}

type remark interface {
	GetTopComments(link string) ([]openai.RemarkComment, error)
}

// Listen on Port accept and forward to telegram
func (l Rtjc) Listen(ctx context.Context) {
	log.Printf("[INFO] rtjc listener on port %d", l.Port)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", l.Port))
	if err != nil {
		log.Fatalf("[ERROR] can't listen on %d, %v", l.Port, err)
	}

	sendSummary := func(msg string) {
		if !strings.HasPrefix(msg, "⚠") {
			return
		}
		sendMessages, err := l.getSummaryMessages(msg)
		if err != nil {
			log.Printf("[WARN] can't get summary, %v", err)
			return
		}
		i := 0
		for sendMsg := range sendMessages {
			// Every 15 messages, wait for 60 seconds to avoid telegram API limit
			if i > 0 && i%15 == 0 {
				// Wait for message queue to be empty
				_ = l.Submitter.WaitMessageQueue()
				time.Sleep(60 * time.Second)
			}
			if sendMsg == "" {
				log.Printf("[WARN] empty summary item #%d for %q", i, msg)
				return
			}
			if err := l.Submitter.SubmitHTML(ctx, sendMsg, false); err != nil {
				log.Printf("[WARN] can't send summary, %v", err)
			}
			i++
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
func (l Rtjc) getSummaryMessages(msg string) (messages <-chan string, err error) {
	log.Printf("[DEBUG] summary for message: %s", msg)

	re := regexp.MustCompile(`https?://[^\s"'<>]+`)
	link := re.FindString(msg)
	log.Printf("[DEBUG] Link found: %s", link)
	if strings.Contains(link, "radio-t.com") {
		return l.getSummaryMessagesFromComments(link)
	}

	ch := make(chan string, 10)
	defer close(ch)

	if link == "" {
		return ch, fmt.Errorf("can't find link in message: %s", msg)
	}

	message, err := l.Summarizer.Summary(link)
	if err != nil {
		return ch, fmt.Errorf("can't get summary for %s: %w", link, err)
	}

	ch <- message
	return ch, nil
}

func (l Rtjc) getSummaryMessagesFromComments(remarkLink string) (messages <-chan string, err error) {
	log.Printf("[DEBUG] summary for Radio-t link: %s", remarkLink)

	ch := make(chan string, 10)

	re := regexp.MustCompile(`https?://radio-t\.com/p/[^\s"'<>]+/prep-\d+/`)
	if !re.MatchString(remarkLink) {
		defer close(ch)
		return ch, fmt.Errorf("radio-t link doesn't fit to format: %s", remarkLink) // ignore radio-t.com links
	}

	comments, err := l.RemarkClient.GetTopComments(remarkLink)
	if err != nil {
		defer close(ch)
		return ch, fmt.Errorf("can't get comments for %s: %w", remarkLink, err)
	}

	prepareComments := func() {
		defer close(ch)

		for i, c := range comments {
			prefix := fmt.Sprintf("[%d/%d] ", i+1, len(comments))
			link := c.GetLink()
			if link == "" {
				ch <- fmt.Sprintf("%s%s", prefix, c.Render())
				continue
			}

			summary, err := l.Summarizer.Summary(link)
			if err != nil {
				log.Printf("[WARN] can't get summary for %s: %v", link, err)
				summary = fmt.Sprintf("<code>Error: %v</code>", err)
			}

			ch <- fmt.Sprintf("%s%s\n\n%s", prefix, c.Render(), summary)
		}
	}
	go prepareComments()

	return ch, nil
}
