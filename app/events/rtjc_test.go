package events

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/radio-t/super-bot/app/bot"
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

//func TestRtjc_getSummaryByLink(t *testing.T) {
//	oai := &mocks.OpenAISummary{
//		SummaryFunc: func(text string) (string, error) {
//			return "ai summary", nil
//		},
//	}
//
//	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		assert.Equal(t, http.MethodGet, r.Method)
//		assert.Equal(t, "token123", r.URL.Query().Get("token"))
//		_, err := w.Write([]byte(`{"Content": "some Content", "Title": "some Title"}`))
//		require.NoError(t, err)
//	}))
//
//	summary := bot.NewSummarizer(oai, ts.URL, "token123", ts.Client(), false)
//	rtjc := Rtjc{Summarizer: summary, RemarkClient: ts.Client()}
//
//	{
//		item, err := rtjc.get("https://example.com")
//		require.NoError(t, err)
//		assert.Equal(t, "ai summary", item.Content)
//		assert.Equal(t, "some Title - some Content", oai.SummaryCalls()[0].Text)
//		assert.Equal(t, "some Title", item.Title)
//	}
//
//	// We aren't limited for requests to radio-t.com
//	{
//		item, err := rtjc.getSummaryByLink("https://radio-t.com")
//		require.NoError(t, err)
//		assert.Equal(t, "ai summary", item.Content)
//		assert.Equal(t, "some Title", item.Title)
//	}
//}

func TestRtjc_getSummaryMessages(t *testing.T) {
	oai := &mocks.OpenAISummary{
		SummaryFunc: func(text string) (string, error) {
			return "ai summary", nil
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "token123", r.URL.Query().Get("token"))
		_, err := w.Write([]byte(`{"Content": "some Content", "Title": "some Title", "Type": "text/html"}`))
		require.NoError(t, err)
	}))

	summary := bot.NewSummarizer(oai, ts.URL, "token123", ts.Client(), false)
	remarkClient := bot.RemarkClient{
		HTTPClient: ts.Client(),
	}
	rtjc := Rtjc{Summarizer: summary, RemarkClient: remarkClient}

	{
		ch, err := rtjc.getSummaryMessages("some message blah https://example.com")
		messages := make([]string, 0)
		for message := range ch {
			messages = append(messages, message)
		}
		require.NoError(t, err)
		assert.Equal(t, 1, len(messages))
		assert.Equal(t, "<b>some Title</b>\n\nai summary", messages[0])
		assert.Equal(t, "some Title - some Content", oai.SummaryCalls()[0].Text)
	}

	//{
	//	items, err := rtjc.getSummaryMessages("some message blah https://radio-t.com")
	//	require.NoError(t, err)
	//	assert.Equal(t, 0, len(items))
	//}

}
