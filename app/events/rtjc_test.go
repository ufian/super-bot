package events

import (
	"fmt"
	"github.com/radio-t/super-bot/app/bot/openai"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/radio-t/super-bot/app/events/mocks"
)

func TestRtjc_isPinned(t *testing.T) {
	tbl := []struct {
		inp string
		out string
		pin bool
	}{
		{"blah", "blah", false},
		{"⚠️ Официальный кАт! - https://stream.radio-t.com/", "⚠️ Вещание подкаста началось - https://stream.radio-t.com/", true},
		{" ⚠️ Официальный кАт! - https://stream.radio-t.com/ ", "⚠️ Вещание подкаста началось - https://stream.radio-t.com/", true},
		{" ⚠️ Официальный кАт! - https://stream.radio-t.com/\n", "⚠️ Вещание подкаста началось - https://stream.radio-t.com/", true},
	}

	rtjc := Rtjc{}
	for i, tt := range tbl {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			pin, out := rtjc.isPinned(tt.inp)
			assert.Equal(t, tt.pin, pin)
			assert.Equal(t, tt.out, out)
		})
	}
}

func genRemarkComment(user, text string, score int) openai.RemarkComment {
	return openai.RemarkComment{
		ParentID: "0",
		Text:     text,
		User: struct {
			Name     string `json:"name"`
			Admin    bool   `json:"admin"`
			Verified bool   `json:"verified,omitempty"`
		}{Name: user, Admin: false, Verified: true},
		Score: score,
	}
}

func TestRtjc_getSummaryMessages(t *testing.T) {
	s := &mocks.Summarizer{
		SummaryFunc: func(link string) (string, error) {
			return "ai summary", nil
		},
	}

	rc := &mocks.RemarkClient{
		GetTopCommentsFunc: func(text string) ([]openai.RemarkComment, error) {
			return []openai.RemarkComment{
				genRemarkComment("User1", "some message blah <a href=\"https://example.user1.com\">Link</a>", 2),
				genRemarkComment("User2", "some message blah <a href=\"https://example.user2.com\">Link</a>", 1),
			}, nil
		},
	}

	rtjc := Rtjc{Summarizer: s, RemarkClient: rc}

	{
		// Test case of regular theme
		ch, err := rtjc.getSummaryMessages("some message blah https://example.theme.com")
		assert.NoError(t, err)
		messages := make([]string, 0)
		for message := range ch {
			messages = append(messages, message)
		}
		require.NoError(t, err)
		assert.Equal(t, 1, len(messages))
		assert.Equal(t, "ai summary", messages[0])
		assert.Equal(t, 1, len(s.SummaryCalls()))
		assert.Equal(t, 0, len(rc.GetTopCommentsCalls()))
		assert.Equal(t, "https://example.theme.com", s.SummaryCalls()[0].Link)
	}

	{
		// Test case of user themes
		ch, err := rtjc.getSummaryMessages("some message blah https://radio-t.com/p/2023/04/04/prep-853/")
		assert.NoError(t, err)
		messages := make([]string, 0)
		for message := range ch {
			messages = append(messages, message)
		}
		require.NoError(t, err)
		assert.Equal(t, 2, len(messages))
		assert.Equal(t, "[1/2] <b>+2</b> от <b>User1</b>\n<i>some message blah <a href=\"https://example.user1.com\">Link</a></i>\n\nai summary", messages[0])
		assert.Equal(t, "[2/2] <b>+1</b> от <b>User2</b>\n<i>some message blah <a href=\"https://example.user2.com\">Link</a></i>\n\nai summary", messages[1])
		assert.Equal(t, 3, len(s.SummaryCalls()))
		assert.Equal(t, "https://example.user1.com", s.SummaryCalls()[1].Link)
		assert.Equal(t, "https://example.user2.com", s.SummaryCalls()[2].Link)
		assert.Equal(t, 1, len(rc.GetTopCommentsCalls()))
		assert.Equal(t, "https://radio-t.com/p/2023/04/04/prep-853/", rc.GetTopCommentsCalls()[0].Link)
	}
}

func TestRtjc_getSummaryMessagesErrCases(t *testing.T) {
	{
		// Message doesn't have link
		s := &mocks.Summarizer{
			SummaryFunc: func(link string) (string, error) {
				return "ai summary", nil
			},
		}

		rc := &mocks.RemarkClient{
			GetTopCommentsFunc: func(text string) ([]openai.RemarkComment, error) {
				return []openai.RemarkComment{}, nil
			},
		}

		rtjc := Rtjc{Summarizer: s, RemarkClient: rc}

		// Test case of regular theme
		ch, err := rtjc.getSummaryMessages("some message blah")
		require.Error(t, err)

		messages := make([]string, 0)
		for message := range ch {
			messages = append(messages, message)
		}
		assert.Equal(t, 0, len(messages))
		assert.Equal(t, 0, len(s.SummaryCalls()))
		assert.Equal(t, 0, len(rc.GetTopCommentsCalls()))
	}

	{
		// Summarizer failed
		s := &mocks.Summarizer{
			SummaryFunc: func(link string) (string, error) {
				return "", fmt.Errorf("some error")
			},
		}

		rc := &mocks.RemarkClient{
			GetTopCommentsFunc: func(text string) ([]openai.RemarkComment, error) {
				return []openai.RemarkComment{}, nil
			},
		}

		rtjc := Rtjc{Summarizer: s, RemarkClient: rc}

		// Test case of regular theme
		ch, err := rtjc.getSummaryMessages("some message blah https://example.theme.com")
		require.Error(t, err)

		messages := make([]string, 0)
		for message := range ch {
			messages = append(messages, message)
		}
		assert.Equal(t, 0, len(messages))
		assert.Equal(t, 1, len(s.SummaryCalls()))
		assert.Equal(t, "https://example.theme.com", s.SummaryCalls()[0].Link)
		assert.Equal(t, 0, len(rc.GetTopCommentsCalls()))
	}

	{
		// Bad radio-t link to user themes
		s := &mocks.Summarizer{
			SummaryFunc: func(link string) (string, error) {
				return "ai summary", nil
			},
		}

		rc := &mocks.RemarkClient{
			GetTopCommentsFunc: func(text string) ([]openai.RemarkComment, error) {
				return []openai.RemarkComment{}, nil
			},
		}

		rtjc := Rtjc{Summarizer: s, RemarkClient: rc}

		// Test case of regular theme
		ch, err := rtjc.getSummaryMessages("some message blah https://radio-t.com/about/")
		require.Error(t, err)

		messages := make([]string, 0)
		for message := range ch {
			messages = append(messages, message)
		}
		assert.Equal(t, 0, len(messages))
		assert.Equal(t, 0, len(s.SummaryCalls()))
		assert.Equal(t, 0, len(rc.GetTopCommentsCalls()))
	}

	{
		// Failure of RemarkClient.GetTopComments
		s := &mocks.Summarizer{
			SummaryFunc: func(link string) (string, error) {
				return "ai summary", nil
			},
		}

		rc := &mocks.RemarkClient{
			GetTopCommentsFunc: func(text string) ([]openai.RemarkComment, error) {
				return []openai.RemarkComment{}, fmt.Errorf("some error")
			},
		}

		rtjc := Rtjc{Summarizer: s, RemarkClient: rc}

		// Test case of regular theme
		ch, err := rtjc.getSummaryMessages("some message blah https://radio-t.com/p/2023/04/04/prep-853/")
		require.Error(t, err)

		messages := make([]string, 0)
		for message := range ch {
			messages = append(messages, message)
		}
		assert.Equal(t, 0, len(messages))
		assert.Equal(t, 0, len(s.SummaryCalls()))
		assert.Equal(t, 1, len(rc.GetTopCommentsCalls()))
		assert.Equal(t, "https://radio-t.com/p/2023/04/04/prep-853/", rc.GetTopCommentsCalls()[0].Link)
	}
}
