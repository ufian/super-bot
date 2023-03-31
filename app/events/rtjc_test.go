package events

import (
	"net/http"
	"net/http/httptest"
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

func TestRtjc_getSummaryByLink(t *testing.T) {
	oai := &mocks.OpenAISummary{
		SummaryFunc: func(text string) (string, error) {
			return "ai summary", nil
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "token123", r.URL.Query().Get("token"))
		_, err := w.Write([]byte(`{"content": "some content", "title": "some title"}`))
		require.NoError(t, err)
	}))

	rtjc := Rtjc{OpenAISummary: oai, URClient: ts.Client(), UrAPI: ts.URL, UrToken: "token123"}

	{
		item, err := rtjc.getSummaryByLink("https://example.com")
		require.NoError(t, err)
		assert.Equal(t, "ai summary", item.content)
		assert.Equal(t, "some title - some content", oai.SummaryCalls()[0].Text)
		assert.Equal(t, "some title", item.title)
	}

	// We aren't limited for requests to radio-t.com
	{
		item, err := rtjc.getSummaryByLink("https://radio-t.com")
		require.NoError(t, err)
		assert.Equal(t, "ai summary", item.content)
		assert.Equal(t, "some title", item.title)
	}
}

func TestRtjc_getSummaryMessages(t *testing.T) {
	oai := &mocks.OpenAISummary{
		SummaryFunc: func(text string) (string, error) {
			return "ai summary", nil
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "token123", r.URL.Query().Get("token"))
		_, err := w.Write([]byte(`{"content": "some content", "title": "some title"}`))
		require.NoError(t, err)
	}))

	rtjc := Rtjc{OpenAISummary: oai, URClient: ts.Client(), UrAPI: ts.URL, UrToken: "token123"}

	{
		items, err := rtjc.getSummaryMessages("some message blah https://example.com")
		require.NoError(t, err)
		assert.Equal(t, 1, len(items))
		assert.Equal(t, "ai summary", items[0].content)
		assert.Equal(t, "some title - some content", oai.SummaryCalls()[0].Text)
		assert.Equal(t, "some title", items[0].title)
	}

	{
		items, err := rtjc.getSummaryMessages("some message blah https://radio-t.com")
		require.NoError(t, err)
		assert.Equal(t, 0, len(items))
	}

}
