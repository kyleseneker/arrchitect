package downloads

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/testnet"
)

// fakeQBRSS keeps RSS feeds, folders and rules as qBittorrent does: a feed's
// folder must exist, two feeds can't share a URL, and setRule replaces the rule
// with what it is sent.
type fakeQBRSS struct {
	mu      sync.Mutex
	feeds   map[string]string
	folders map[string]bool
	rules   map[string]map[string]any
	writes  []string
}

func (f *fakeQBRSS) serve(t *testing.T) string {
	t.Helper()
	if f.feeds == nil {
		f.feeds = map[string]string{}
	}
	if f.folders == nil {
		f.folders = map[string]bool{}
	}
	if f.rules == nil {
		f.rules = map[string]map[string]any{}
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(testnet.RoutableAddr(t), "0"))
	require.NoError(t, err)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodPost {
			require.NoError(t, r.ParseForm())
		}
		conflict := func(msg string) { http.Error(w, msg, http.StatusConflict) }
		parentExists := func(path string) bool {
			i := strings.LastIndex(path, `\`)
			return i < 0 || f.folders[path[:i]]
		}
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "session", Path: "/"})
			_, _ = w.Write([]byte("Ok."))
			return
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5"))
			return
		case "/api/v2/rss/items":
			_ = json.NewEncoder(w).Encode(f.tree())
			return
		case "/api/v2/rss/rules":
			_ = json.NewEncoder(w).Encode(f.rules)
			return
		case "/api/v2/torrents/categories":
			_, _ = w.Write([]byte("{}"))
			return
		}
		f.writes = append(f.writes, strings.TrimPrefix(r.URL.Path, "/api/v2/rss/"))
		switch r.URL.Path {
		case "/api/v2/rss/addFolder":
			path := r.Form.Get("path")
			if !parentExists(path) {
				conflict("Parent folder doesn't exist")
				return
			}
			f.folders[path] = true
		case "/api/v2/rss/addFeed":
			path, url := r.Form.Get("path"), r.Form.Get("url")
			for _, u := range f.feeds {
				if u == url {
					conflict("RSS feed with given URL already exists")
					return
				}
			}
			if !parentExists(path) {
				conflict("Parent folder doesn't exist")
				return
			}
			f.feeds[path] = url
		case "/api/v2/rss/setFeedURL":
			f.feeds[r.Form.Get("path")] = r.Form.Get("url")
		case "/api/v2/rss/removeItem":
			delete(f.feeds, r.Form.Get("path"))
		case "/api/v2/rss/setRule":
			var def map[string]any
			require.NoError(t, json.Unmarshal([]byte(r.Form.Get("ruleDef")), &def))
			f.rules[r.Form.Get("ruleName")] = def
		case "/api/v2/rss/removeRule":
			delete(f.rules, r.Form.Get("ruleName"))
		}
	}))
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL
}

func (f *fakeQBRSS) tree() map[string]any {
	root := map[string]any{}
	folder := func(path string) map[string]any {
		node := root
		if path == "" {
			return node
		}
		for name := range strings.SplitSeq(path, `\`) {
			next, ok := node[name].(map[string]any)
			if !ok {
				next = map[string]any{}
				node[name] = next
			}
			node = next
		}
		return node
	}
	for path := range f.folders {
		folder(path)
	}
	for path, url := range f.feeds {
		parent, name, nested := strings.CutLast(path, `\`)
		if !nested {
			parent, name = "", path
		}
		folder(parent)[name] = map[string]any{"uid": "{" + name + "}", "url": url}
	}
	return root
}

func TestQBAddsRSSFeedsWithTheirFoldersAndOwnsThem(t *testing.T) {
	app := &fakeQBRSS{feeds: map[string]string{"Mine": "https://mine.example/rss"}}
	cfg := qbConfig(app.serve(t))
	cfg.Spec.RSSFeeds = []downloadsv1alpha1.QBittorrentRSSFeed{
		{Name: `Anime\SubsPlease`, URL: "https://subsplease.example/rss"},
		{Name: "Mine", URL: "https://mine.example/rss"},
	}

	got := runQB(t, cfg)

	assert.Equal(t, []string{"addFolder", "addFeed"}, app.writes)
	assert.True(t, app.folders["Anime"])
	assert.Equal(t, "https://subsplease.example/rss", app.feeds[`Anime\SubsPlease`])
	assert.Equal(t, []string{`Anime\SubsPlease`}, got.GetManagedResources()["rssFeeds"], "an adopted feed isn't the operator's")

	runQB(t, got)
	assert.Len(t, app.writes, 2, "second pass must not write")
}

func TestQBUpdatesAnRSSFeedURLAndRefusesOneAnotherFeedHas(t *testing.T) {
	app := &fakeQBRSS{feeds: map[string]string{"Shows": "https://old.example/rss", "Other": "https://taken.example/rss"}}
	cfg := qbConfig(app.serve(t))
	cfg.Spec.RSSFeeds = []downloadsv1alpha1.QBittorrentRSSFeed{{Name: "Shows", URL: "https://new.example/rss"}}

	runQB(t, cfg)
	assert.Equal(t, []string{"setFeedURL"}, app.writes)
	assert.Equal(t, "https://new.example/rss", app.feeds["Shows"])

	cfg.Spec.RSSFeeds = []downloadsv1alpha1.QBittorrentRSSFeed{{Name: "Movies", URL: "https://taken.example/rss"}}
	got := runQB(t, cfg)
	assert.Len(t, app.writes, 1)
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Contains(t, synced.Message, `rssFeed(Movies): qBittorrent already reads https://taken.example/rss as "Other"; rename or remove that feed`)
}

func TestQBPrunesARenamedRSSFeedBeforeAddingItsNewName(t *testing.T) {
	app := &fakeQBRSS{feeds: map[string]string{"Old name": "https://feed.example/rss"}}
	cfg := qbConfig(app.serve(t))
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true)}
	cfg.Spec.RSSFeeds = []downloadsv1alpha1.QBittorrentRSSFeed{{Name: "New name", URL: "https://feed.example/rss"}}
	cfg.SetManagedResources(map[string][]string{"rssFeeds": {"Old name"}})

	got := runQB(t, cfg)

	assert.Equal(t, []string{"removeItem", "addFeed"}, app.writes)
	assert.Equal(t, map[string]string{"New name": "https://feed.example/rss"}, app.feeds)
	assert.Equal(t, []string{"New name"}, got.GetManagedResources()["rssFeeds"])
}

// Observe mode plans as enforce mode would act: the rename frees the old
// feed's URL, so the new name is a planned addition, not a conflict.
func TestQBObservePlansARenamedRSSFeedLikeEnforce(t *testing.T) {
	app := &fakeQBRSS{feeds: map[string]string{"Old name": "https://feed.example/rss"}}
	cfg := qbConfig(app.serve(t))
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true), DriftPolicy: "observe"}
	cfg.Spec.RSSFeeds = []downloadsv1alpha1.QBittorrentRSSFeed{{Name: "New name", URL: "https://feed.example/rss"}}
	cfg.SetManagedResources(map[string][]string{"rssFeeds": {"Old name"}})

	got := runQB(t, cfg)

	assert.Empty(t, app.writes)
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Equal(t, "DriftDetected", synced.Reason, synced.Message)
	assert.Contains(t, synced.Message, "rssFeeds(Old name)")
	assert.Contains(t, synced.Message, "rssFeeds(New name)")
}

func TestQBRSSRuleUpdatesKeepItsMatchHistory(t *testing.T) {
	app := &fakeQBRSS{rules: map[string]map[string]any{"Show": {
		"enabled": true, "priority": float64(0), "useRegex": false, "mustContain": "Show 1080p", "mustNotContain": "",
		"episodeFilter": "", "affectedFeeds": []any{"https://feed.example/rss"}, "ignoreDays": float64(0), "smartFilter": true,
		"lastMatch":                 "Mon, 23 Nov 2026 10:00:00 +0000",
		"previouslyMatchedEpisodes": []any{"S01E01", "S01E02"},
		"torrentParams":             map[string]any{"category": "", "tags": []any{}, "save_path": "", "operating_mode": "AutoManaged"},
	}}}
	cfg := qbConfig(app.serve(t))
	cfg.Spec.RSSRules = []downloadsv1alpha1.QBittorrentRSSRule{
		{
			Name: "Show", MustContain: new("Show 2160p"),
			TorrentParams: downloadsv1alpha1.QBittorrentRSSTorrentParams{Category: new("tv"), Tags: []string{"rss", "anime"}},
		},
		{Name: "New", Enabled: new(false), AffectedFeeds: []string{"https://feed.example/rss"}, MustContain: new("New")},
	}

	got := runQB(t, cfg)

	assert.ElementsMatch(t, []string{"setRule", "setRule"}, app.writes)
	show := app.rules["Show"]
	assert.Equal(t, "Show 2160p", show["mustContain"])
	assert.Equal(t, "Mon, 23 Nov 2026 10:00:00 +0000", show["lastMatch"])
	assert.Equal(t, []any{"S01E01", "S01E02"}, show["previouslyMatchedEpisodes"])
	assert.Equal(t, true, show["smartFilter"])
	params, _ := show["torrentParams"].(map[string]any)
	assert.Equal(t, "tv", params["category"])
	assert.Equal(t, []any{"anime", "rss"}, params["tags"], "qBittorrent keeps tags sorted")
	assert.Equal(t, "AutoManaged", params["operating_mode"])
	assert.Equal(t, false, app.rules["New"]["enabled"])
	assert.Equal(t, []string{"New"}, got.GetManagedResources()["rssRules"])

	runQB(t, got)
	assert.Len(t, app.writes, 2, "second pass must not write")
}

func TestQBObserveReportsRSSDifferences(t *testing.T) {
	app := &fakeQBRSS{
		feeds: map[string]string{"Shows": "https://old.example/rss", "Gone": "https://gone.example/rss"},
		rules: map[string]map[string]any{"Show": {"mustContain": "720p"}},
	}
	cfg := qbConfig(app.serve(t))
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true), DriftPolicy: "observe"}
	cfg.Spec.RSSFeeds = []downloadsv1alpha1.QBittorrentRSSFeed{{Name: "Shows", URL: "https://new.example/rss"}}
	cfg.Spec.RSSRules = []downloadsv1alpha1.QBittorrentRSSRule{{Name: "Show", MustContain: new("1080p")}}
	cfg.SetManagedResources(map[string][]string{"rssFeeds": {"Gone"}})

	got := runQB(t, cfg)

	assert.Empty(t, app.writes)
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Equal(t, "DriftDetected", synced.Reason)
	for _, item := range []string{"rssFeeds(Shows)", "rssFeeds(Gone)", "rssRules(Show)"} {
		assert.Contains(t, synced.Message, item)
	}
}

func TestQBDeletionPolicyRemovesTheRSSItCreated(t *testing.T) {
	app := &fakeQBRSS{
		feeds: map[string]string{"Ours": "https://ours.example/rss", "Theirs": "https://theirs.example/rss"},
		rules: map[string]map[string]any{"Ours": {}, "Theirs": {}},
	}
	cfg := qbConfig(app.serve(t))
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DeletionPolicy: ctrlcommon.DeletionPolicyDelete}
	cfg.SetManagedResources(map[string][]string{"rssFeeds": {"Ours"}, "rssRules": {"Ours"}})
	now := metav1.NewTime(time.Now())
	cfg.DeletionTimestamp = &now
	cfg.Finalizers = []string{ctrlcommon.Finalizer}

	runQB(t, cfg)

	assert.Equal(t, map[string]string{"Theirs": "https://theirs.example/rss"}, app.feeds)
	assert.Equal(t, map[string]map[string]any{"Theirs": {}}, app.rules)
}
