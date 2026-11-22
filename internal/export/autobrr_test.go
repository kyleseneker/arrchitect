package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	automationv1alpha1 "github.com/kyleseneker/arrchitect/api/automation/v1alpha1"
)

var fakeAutobrrAPI = map[string]string{
	"/api/downloaders": `[{"id":3,"name":"qBit","type":"QBITTORRENT","enabled":true,"host":"http://qbit","port":8080,
		"tls":false,"username":"admin","password":"<redacted>"},
		{"id":4,"name":"Sonarr","type":"SONARR","enabled":true,"host":"http://sonarr:8989","settings":{"apikey":"<redacted>"}}]`,
	"/api/indexer": `[{"id":1,"name":"Tracker","identifier":"ipt","implementation":"irc",
		"settings":[{"name":"passkey","type":"secret"}]},{"id":5,"name":"Feed","identifier":"torznab-feed"}]`,
	"/api/indexer/1": `{"id":1,"name":"Tracker","identifier":"ipt","implementation":"irc","enabled":true,
		"base_url":"https://iptorrents.com/","settings":{"passkey":"<redacted>","freeleech":"true"}}`,
	"/api/indexer/5": `{"id":5,"name":"Feed","identifier":"torznab-feed","implementation":"torznab","enabled":true}`,
	"/api/irc": `[{"id":1,"name":"Net","enabled":false,"server":"irc.example","port":6697,"tls":true,"nick":"bot",
		"auth":{"mechanism":"NICKSERV","account":"bot","password":"<redacted>"},
		"channels":[{"name":"#announce","password":""},{"name":"#vip","password":"<redacted>"}]}]`,
	"/api/notification": `[{"id":1,"name":"Builtin","type":"BUILTIN","enabled":true},
		{"id":2,"name":"Discord","type":"DISCORD","enabled":true,"events":["PUSH_APPROVED","IRC_DISCONNECTED"],
		"webhook":"<redacted>","title":"autobrr","priority":0,"token":""}]`,
	"/api/feeds": `[{"id":1,"name":"Feed","type":"TORZNAB","enabled":true,"url":"http://feed","interval":15,"api_key":"<redacted>",
		"indexer":{"id":5,"name":"Feed","identifier":"torznab-feed"}}]`,
	"/api/filters": `[{"id":7,"name":"TV"}]`,
	"/api/filters/7": `{"id":7,"name":"TV","enabled":true,"priority":1,"use_regex":false,"match_releases":"",
		"resolutions":["1080p"],"sources":["WEB-DL"],"codecs":[],"tags":"tv,hd","except_tags":"cam",
		"match_hdr":["DV"],"max_downloads":5,"max_downloads_unit":"DAY","freeleech":false,"min_seeders":0,
		"indexers":[{"id":1,"name":"Tracker","identifier":"ipt"}]}`,
	"/api/actions": `[{"id":2,"name":"Send","type":"QBITTORRENT","enabled":true,"client_id":3,"category":"tv","save_path":"",
		"limit_ratio":1.5,"content_layout":"SUBFOLDER_CREATE","paused":false,"webhook_headers":[]}]`,
}

func exportAutobrrForTest(t *testing.T) (*automationv1alpha1.Autobrr, Result) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Token") != "k" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		body, ok := fakeAutobrrAPI[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	res, err := Run(context.Background(), Options{App: "autobrr", URL: srv.URL, APIKey: "k"})
	require.NoError(t, err)
	cfg, ok := res.Config.(*automationv1alpha1.Autobrr)
	require.True(t, ok)
	return cfg, res
}

func TestAutobrrExportsEachResourceWithSecretsInTheSecret(t *testing.T) {
	cfg, res := exportAutobrrForTest(t)

	require.Len(t, cfg.Spec.DownloadClients, 2)
	dc := cfg.Spec.DownloadClients[0]
	assert.Equal(t, "qBittorrent", dc.Type)
	assert.Equal(t, int32(8080), *dc.Port)
	assert.Equal(t, "admin", res.Secret.StringData[dc.UsernameSecretRef.Key])
	assert.Contains(t, res.Unfilled, dc.PasswordSecretRef.Key)
	assert.Nil(t, dc.APIKeySecretRef)
	sonarr := cfg.Spec.DownloadClients[1]
	require.NotNil(t, sonarr.APIKeySecretRef, "the API key Autobrr keeps in settings was not exported")
	assert.Contains(t, res.Unfilled, sonarr.APIKeySecretRef.Key)

	require.Len(t, cfg.Spec.IRCNetworks, 1)
	irc := cfg.Spec.IRCNetworks[0]
	assert.Equal(t, "NickServ", irc.AuthMechanism)
	assert.Equal(t, "bot", res.Secret.StringData[irc.AuthAccountSecretRef.Key])
	assert.Nil(t, irc.Channels[0].PasswordSecretRef)
	assert.Equal(t, "irc-net-vip-password", irc.Channels[1].PasswordSecretRef.Key)

	require.Len(t, cfg.Spec.Feeds, 1)
	feed := cfg.Spec.Feeds[0]
	assert.Equal(t, int32(15), *feed.Interval)
	assert.Equal(t, "Torznab", feed.Type)
	assert.Equal(t, "Feed", feed.IndexerRef)
	require.Len(t, cfg.Spec.Indexers, 2)
	tracker := cfg.Spec.Indexers[0]
	assert.Equal(t, "ipt", tracker.Identifier)
	require.Len(t, tracker.Settings, 2)
	assert.Equal(t, "freeleech", tracker.Settings[0].Name)
	assert.Equal(t, `"true"`, string(tracker.Settings[0].Value.Raw))
	assert.Equal(t, "passkey", tracker.Settings[1].Name)
	require.NotZero(t, tracker.Settings[1].ValueFrom)
	assert.Contains(t, res.Unfilled, tracker.Settings[1].ValueFrom.SecretKeyRef.Key)
	assert.Equal(t, "torznab", cfg.Spec.Indexers[1].Identifier, "a feed indexer exports its definition, not the name Autobrr gave it")

	require.Len(t, cfg.Spec.Filters, 1)
	filter := cfg.Spec.Filters[0]
	assert.Equal(t, []string{"1080p"}, filter.Resolutions)
	assert.Equal(t, []string{"Tracker"}, filter.Indexers)
	assert.Equal(t, []string{"tv", "hd"}, filter.Tags)
	require.Len(t, filter.Actions, 1)
	assert.Equal(t, "qBit", filter.Actions[0].ClientRef, "an action names its download client")
	assert.Equal(t, "qBittorrent", filter.Actions[0].Type)
	assert.Equal(t, "tv", filter.Actions[0].Category)

	assert.Equal(t, []string{"cam"}, filter.ExceptTags)
	assert.Equal(t, []string{"DV"}, filter.MatchHdr)
	assert.Equal(t, int32(5), *filter.MaxDownloads)
	assert.Equal(t, "DAY", filter.MaxDownloadsUnit)
	assert.Nil(t, filter.Freeleech, "Autobrr's empty defaults aren't exported")
	assert.Nil(t, filter.MinSeeders)
	assert.Equal(t, int32(1), *filter.Priority)
	action := filter.Actions[0]
	assert.Equal(t, "1.5", string(*action.LimitRatio))
	assert.Equal(t, "SUBFOLDER_CREATE", action.ContentLayout)
	assert.Nil(t, action.Paused)
	assert.Empty(t, action.WebhookHeaders)
}

func TestAutobrrExportsNotificationsButNotTheBuiltinOne(t *testing.T) {
	cfg, res := exportAutobrrForTest(t)
	require.Len(t, cfg.Spec.Notifications, 1, "the built-in notification is Autobrr's own")
	n := cfg.Spec.Notifications[0]
	assert.Equal(t, "Discord", n.Type)
	assert.Equal(t, []string{"pushApproved", "ircDisconnected"}, n.Events)
	require.Len(t, n.Options, 2, "empty and zero settings are left out")
	assert.Equal(t, "webhook", n.Options[0].Name)
	assert.Contains(t, res.Unfilled, n.Options[0].ValueFrom.SecretKeyRef.Key, "a redacted webhook is left for the user to fill in")
	assert.Equal(t, "title", n.Options[1].Name)
}
