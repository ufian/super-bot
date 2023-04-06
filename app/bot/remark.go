package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-pkgz/notify"
	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type RemarkComment struct {
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

func (c RemarkComment) Render() string {
	user := tbapi.EscapeText(tbapi.ModeHTML, c.User.Name)
	text := notify.TelegramSupportedHTML(c.Text)
	return fmt.Sprintf("<b>%+d</b> от <b>%s</b>\n<i>%s</i>", c.Score, user, text)
}

func (c RemarkComment) GetLink() string {
	// Find only links in the comment
	reLink := regexp.MustCompile(`href="(https?://[^\s"'<>]+)"`)

	parts := reLink.FindStringSubmatch(c.Text)

	if len(parts) < 2 {
		return ""
	}

	link := parts[1]
	if link == "" || strings.Contains(link, "radio-t.com") {
		return ""
	}

	return link
}

type RemarkClient struct {
	HTTPClient *http.Client
}

func (c RemarkClient) getComments(remarkLink string) (comments []RemarkComment, err error) {
	rl := fmt.Sprintf("https://remark42.radio-t.com/api/v1/find?site=radiot&url=%s&sort-score&format=plain", remarkLink)
	resp, err := c.HTTPClient.Get(rl)
	if err != nil {
		return []RemarkComment{}, fmt.Errorf("can't get comments for %s: %w", remarkLink, err)
	}
	defer resp.Body.Close() // nolint
	if resp.StatusCode != http.StatusOK {
		return []RemarkComment{}, fmt.Errorf("can't get comments for %s: %d", remarkLink, resp.StatusCode)
	}

	urResp := struct {
		Comments []RemarkComment `json:"comments"`
	}{}

	if decErr := json.NewDecoder(resp.Body).Decode(&urResp); decErr != nil {
		return []RemarkComment{}, fmt.Errorf("can't decode comments for %s: %w", remarkLink, decErr)
	}

	for _, c := range urResp.Comments {
		if c.ParentID != "" || c.Deleted {
			continue
		}

		comments = append(comments, c)
	}
	return comments, nil
}

func (c RemarkClient) GetTopComments(remarkLink string) (comments []RemarkComment, err error) {
	comments, err = c.getComments(remarkLink)
	if err != nil {
		return []RemarkComment{}, err
	}

	positiveComments := make([]RemarkComment, 0, len(comments))
	for _, c := range comments {
		if c.Score >= 0 {
			positiveComments = append(positiveComments, c)
		}
	}

	sort.Slice(positiveComments, func(i, j int) bool {
		if positiveComments[i].Score < positiveComments[j].Score {
			return false
		}

		if positiveComments[i].Score > positiveComments[j].Score {
			return true
		}
		// Equal case
		return positiveComments[i].Timestamp.Before(positiveComments[j].Timestamp)
	})

	return positiveComments, nil
}
