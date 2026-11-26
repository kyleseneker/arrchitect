package downloads

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	qbclient "github.com/kyleseneker/arrchitect/internal/client/qbittorrent"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// reconcileQBRSSFeeds adds and updates the declared feeds. It prunes first, so
// a feed renamed in the resource frees its URL before the new name is added.
func reconcileQBRSSFeeds(ctx context.Context, qb *qbclient.Client, pass *engine.Pass, feeds []downloadsv1alpha1.QBittorrentRSSFeed) []string {
	current, folders, err := qb.RSSItems(ctx)
	if err != nil {
		return []string{fmt.Sprintf("listing RSS feeds: %v", err)}
	}
	names, present := qbRSSFeedNames(feeds), slices.Collect(maps.Keys(current))
	planned := pass.Owned.Candidates("rssFeeds", names, present)
	// A declared URL held by a feed this pass won't prune fails the pass, and
	// a failed pass prunes nothing.
	if pass.PruneEnabled && !qbRSSURLConflict(current, feeds, planned) {
		err := pass.Prune(pass.Owned, "rssFeeds", names, present, func(path string) error {
			return qb.RemoveRSSItem(ctx, path)
		})
		if err != nil {
			return []string{fmt.Sprintf("rssFeeds(prune): %v", err)}
		}
		for _, path := range planned {
			delete(current, path)
		}
	}
	var errs []string
	for _, feed := range feeds {
		if err := reconcileQBRSSFeed(ctx, qb, pass, current, folders, feed); err != nil {
			errs = append(errs, fmt.Sprintf("rssFeed(%s): %v", feed.Name, err))
		}
	}
	return errs
}

func reconcileQBRSSFeed(
	ctx context.Context, qb *qbclient.Client, pass *engine.Pass, current map[string]string, folders map[string]bool,
	feed downloadsv1alpha1.QBittorrentRSSFeed,
) error {
	if url, exists := current[feed.Name]; exists {
		if url == feed.URL {
			return nil
		}
		if other := qbRSSFeedAt(current, feed.URL); other != "" {
			return fmt.Errorf("qBittorrent already reads %s as %q", feed.URL, other)
		}
		err := pass.Apply("rssFeeds", feed.Name, func() error { return qb.SetRSSFeedURL(ctx, feed.Name, feed.URL) })
		if err == nil {
			current[feed.Name] = feed.URL
		}
		return err
	}
	if other := qbRSSFeedAt(current, feed.URL); other != "" {
		return fmt.Errorf("qBittorrent already reads %s as %q; rename or remove that feed", feed.URL, other)
	}
	err := pass.Apply("rssFeeds", feed.Name, func() error {
		parts := strings.Split(feed.Name, `\`)
		for i := 1; i < len(parts); i++ {
			folder := strings.Join(parts[:i], `\`)
			if folders[folder] {
				continue
			}
			if err := qb.AddRSSFolder(ctx, folder); err != nil {
				return fmt.Errorf("adding folder %q: %w", folder, err)
			}
			folders[folder] = true
		}
		if err := qb.AddRSSFeed(ctx, feed.URL, feed.Name); err != nil {
			return err
		}
		pass.Owned.Created("rssFeeds", feed.Name)
		return nil
	})
	if err == nil {
		current[feed.Name] = feed.URL
	}
	return err
}

// qbRSSURLConflict reports whether a declared feed's URL is held by a feed
// other than itself that the planned prune leaves in place.
func qbRSSURLConflict(current map[string]string, feeds []downloadsv1alpha1.QBittorrentRSSFeed, planned []string) bool {
	for _, feed := range feeds {
		holder := qbRSSFeedAt(current, feed.URL)
		if holder != "" && holder != feed.Name && !slices.Contains(planned, holder) {
			return true
		}
	}
	return false
}

// qbRSSFeedAt returns the path of the feed with url, or "".
func qbRSSFeedAt(feeds map[string]string, url string) string {
	for path, u := range feeds {
		if u == url {
			return path
		}
	}
	return ""
}

func qbRSSFeedNames(feeds []downloadsv1alpha1.QBittorrentRSSFeed) []string {
	names := make([]string, 0, len(feeds))
	for _, f := range feeds {
		names = append(names, f.Name)
	}
	return names
}

// reconcileQBRSSRules adds and updates the declared rules. qBittorrent rebuilds
// a rule from what it is sent, so an update sends the declared fields merged
// over the rule as it stands, keeping its last match and matched episodes.
func reconcileQBRSSRules(ctx context.Context, qb *qbclient.Client, pass *engine.Pass, rules []downloadsv1alpha1.QBittorrentRSSRule) []string {
	current, err := qb.RSSRules(ctx)
	if err != nil {
		return []string{fmt.Sprintf("listing RSS rules: %v", err)}
	}
	var errs []string
	for _, rule := range rules {
		existing, _ := current[rule.Name].(map[string]any)
		err := pass.Sync("rssRules", rule.Name, existing, qbRSSRuleDef(rule), func(merged map[string]any) error {
			if err := qb.SetRSSRule(ctx, rule.Name, merged); err != nil {
				return err
			}
			if existing == nil {
				pass.Owned.Created("rssRules", rule.Name)
			}
			return nil
		})
		if err != nil {
			errs = append(errs, fmt.Sprintf("rssRule(%s): %v", rule.Name, err))
		}
	}
	if !pass.PruneEnabled || len(errs) > 0 {
		return errs
	}
	err = pass.Prune(pass.Owned, "rssRules", qbRSSRuleNames(rules), slices.Collect(maps.Keys(current)), func(name string) error {
		return qb.RemoveRSSRule(ctx, name)
	})
	if err != nil {
		return []string{fmt.Sprintf("rssRules(prune): %v", err)}
	}
	return nil
}

// qbRSSRuleDef is a rule's declared fields as qBittorrent's API names them.
func qbRSSRuleDef(rule downloadsv1alpha1.QBittorrentRSSRule) map[string]any {
	def := map[string]any{}
	for key, value := range map[string]*bool{"enabled": rule.Enabled, "useRegex": rule.UseRegex, "smartFilter": rule.SmartFilter} {
		if value != nil {
			def[key] = *value
		}
	}
	for key, value := range map[string]*string{
		"mustContain": rule.MustContain, "mustNotContain": rule.MustNotContain, "episodeFilter": rule.EpisodeFilter,
	} {
		if value != nil {
			def[key] = *value
		}
	}
	for key, value := range map[string]*int32{"priority": rule.Priority, "ignoreDays": rule.IgnoreDays} {
		if value != nil {
			def[key] = *value
		}
	}
	if rule.AffectedFeeds != nil {
		def["affectedFeeds"] = slices.Clone(rule.AffectedFeeds)
	}
	if params := qbRSSTorrentParams(rule.TorrentParams); len(params) > 0 {
		def["torrentParams"] = params
	}
	return def
}

// qbRSSTorrentParams is a rule's declared torrent parameters as qBittorrent
// names them. qBittorrent keeps tags sorted.
func qbRSSTorrentParams(p downloadsv1alpha1.QBittorrentRSSTorrentParams) map[string]any {
	params := map[string]any{}
	if p.Category != nil {
		params["category"] = *p.Category
	}
	if p.Tags != nil {
		params["tags"] = slices.Sorted(slices.Values(p.Tags))
	}
	if p.SavePath != "" {
		params["save_path"] = qbPath(p.SavePath)
	}
	for key, value := range map[string]*bool{
		"use_auto_tmm": p.UseAutoTMM, "stopped": p.Stopped, "add_to_top_of_queue": p.AddToTopOfQueue,
	} {
		if value != nil {
			params[key] = *value
		}
	}
	if p.ContentLayout != "" {
		params["content_layout"] = p.ContentLayout
	}
	return params
}

func qbRSSRuleNames(rules []downloadsv1alpha1.QBittorrentRSSRule) []string {
	names := make([]string, 0, len(rules))
	for _, r := range rules {
		names = append(names, r.Name)
	}
	return names
}

// cleanupQBRSS removes the feeds and rules the operator created.
func cleanupQBRSS(ctx context.Context, qb *qbclient.Client, owned *engine.Ownership) error {
	if owned.Owns("rssRules") {
		rules, err := qb.RSSRules(ctx)
		if err != nil {
			return err
		}
		if err := owned.Cleanup("rssRules", slices.Collect(maps.Keys(rules)), func(name string) error {
			return qb.RemoveRSSRule(ctx, name)
		}); err != nil {
			return err
		}
	}
	if !owned.Owns("rssFeeds") {
		return nil
	}
	feeds, _, err := qb.RSSItems(ctx)
	if err != nil {
		return err
	}
	return owned.Cleanup("rssFeeds", slices.Collect(maps.Keys(feeds)), func(path string) error {
		return qb.RemoveRSSItem(ctx, path)
	})
}
