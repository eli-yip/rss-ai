package feed_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/feed"
)

// basicRSS: item 0 has a guid and a description; item 1 has no guid (link is the
// id fallback) and a plain-text title.
const basicRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Feed</title>
    <item>
      <title>Original One</title>
      <link>https://example.com/1</link>
      <guid>guid-1</guid>
      <description>Body one</description>
    </item>
    <item>
      <title>Original Two</title>
      <link>https://example.com/2</link>
      <description>Body two</description>
    </item>
  </channel>
</rss>`

// richRSS: namespaces, enclosure, media, category, author, pubDate, a CDATA
// title, and a <content:encoded> body that should win over <description>.
const richRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:media="http://search.yahoo.com/mrss/">
  <channel>
    <title>Rich</title>
    <item>
      <title><![CDATA[Title With <html> & co]]></title>
      <link>https://example.com/a</link>
      <guid isPermaLink="false">guid-a</guid>
      <category>news</category>
      <author>a@example.com</author>
      <pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>
      <enclosure url="https://example.com/a.mp3" length="123" type="audio/mpeg"/>
      <media:content url="https://example.com/a.jpg"/>
      <content:encoded><![CDATA[<p>Full body</p>]]></content:encoded>
      <description>short desc</description>
    </item>
  </channel>
</rss>`

// baseline is the no-op round-trip output: the fixed point the surgical-diff
// assertions compare against, independent of any etree formatting quirks.
func baseline(t *testing.T, raw string) string {
	t.Helper()
	d, err := feed.Parse([]byte(raw))
	require.NoError(t, err)
	out, err := d.Bytes()
	require.NoError(t, err)
	return string(out)
}

func TestRoundTripIdentity(t *testing.T) {
	for _, raw := range []string{basicRSS, richRSS} {
		require.Equal(t, raw, baseline(t, raw), "parse then serialize reproduces input")
	}
}

func TestSurgicalChangeOnlyTitle(t *testing.T) {
	d, err := feed.Parse([]byte(basicRSS))
	require.NoError(t, err)
	d.SetTitle(0, "New One")
	out, err := d.Bytes()
	require.NoError(t, err)

	// Relative to the no-op baseline, exactly the one title text changed.
	want := strings.Replace(baseline(t, basicRSS), "Original One", "New One", 1)
	require.Equal(t, want, string(out))
}

func TestPreservationAcrossRewrite(t *testing.T) {
	d, err := feed.Parse([]byte(richRSS))
	require.NoError(t, err)
	d.SetTitle(0, "Plain New")
	out, err := d.Bytes()
	require.NoError(t, err)

	// CDATA title stays CDATA-wrapped; everything else is byte-identical to the
	// baseline with only the title text swapped.
	want := strings.Replace(baseline(t, richRSS), "Title With <html> & co", "Plain New", 1)
	require.Equal(t, want, string(out))

	// Spot-check the structural fields survived verbatim.
	for _, frag := range []string{
		`<enclosure url="https://example.com/a.mp3" length="123" type="audio/mpeg"/>`,
		`<media:content url="https://example.com/a.jpg"/>`,
		`<category>news</category>`,
		`<author>a@example.com</author>`,
		`<pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>`,
		`<guid isPermaLink="false">guid-a</guid>`,
		`<content:encoded><![CDATA[<p>Full body</p>]]></content:encoded>`,
	} {
		require.Contains(t, string(out), frag)
	}
	require.Contains(t, string(out), `<title><![CDATA[Plain New]]></title>`)
}

func TestItemIDPriority(t *testing.T) {
	d, err := feed.Parse([]byte(basicRSS))
	require.NoError(t, err)
	items := d.Items()
	require.Len(t, items, 2)
	require.Equal(t, "guid-1", items[0].ID, "guid wins")
	require.Equal(t, "https://example.com/2", items[1].ID, "link fallback when no guid")
}

func TestItemIDEmptyWhenNoGuidOrLink(t *testing.T) {
	const noID = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Just a title</title>
    </item>
  </channel>
</rss>`
	d, err := feed.Parse([]byte(noID))
	require.NoError(t, err)
	require.Equal(t, "", d.Items()[0].ID)
}

func TestBodyExtraction(t *testing.T) {
	rich, err := feed.Parse([]byte(richRSS))
	require.NoError(t, err)
	require.Equal(t, "<p>Full body</p>", rich.Items()[0].Body, "content:encoded wins over description")

	basic, err := feed.Parse([]byte(basicRSS))
	require.NoError(t, err)
	require.Equal(t, "Body one", basic.Items()[0].Body, "falls back to description")
}

func TestSetTitleNoOpWhenMissing(t *testing.T) {
	const noTitle = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <link>https://example.com/x</link>
    </item>
  </channel>
</rss>`
	d, err := feed.Parse([]byte(noTitle))
	require.NoError(t, err)
	require.Equal(t, "", d.Items()[0].Title)
	d.SetTitle(0, "ignored") // must not panic
	out, err := d.Bytes()
	require.NoError(t, err)
	require.NotContains(t, string(out), "ignored")
}

func TestParseNonRSSYieldsNoItems(t *testing.T) {
	d, err := feed.Parse([]byte(`<?xml version="1.0"?><feed><entry/></feed>`))
	require.NoError(t, err)
	require.Empty(t, d.Items())
}
