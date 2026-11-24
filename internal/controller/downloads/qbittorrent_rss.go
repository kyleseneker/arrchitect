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
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// reconcileQBRSSFeeds adds and updates the declared feeds. It prunes first, so
// a feed renamed in the resource frees its URL before the new name is added.
func reconcileQBRSSFeeds(
	ctx context.Context, qb *qbclient.Client, feeds []downloadsv1alpha1.QBittorrentRSSFeed, prune bool, owned *engine.Ownership,
) []string {
	current, folders, err := qb.RSSItems(ctx)
	if err != nil {
		return []string{fmt.Sprintf("listing RSS feeds: %v", err)}
	}
	if prune {
		err := owned.Prune("rssFeeds", qbRSSFeedNames(feeds), slices.Collect(maps.Keys(current)), func(path string) error {
			if err := qb.RemoveRSSItem(ctx, path); err != nil {
				return err
			}
			delete(current, path)
			return nil
		})
		if err != nil {
			return []string{fmt.Sprintf("rssFeeds(prune): %v", err)}
		}
	}
	var errs []string
	for _, feed := range feeds {
		if err := reconcileQBRSSFeed(ctx, qb, current, folders, feed, owned); err != nil {
			errs = append(errs, fmt.Sprintf("rssFeed(%s): %v", feed.Name, err))
		}
	}
	return errs
}

func reconcileQBRSSFeed(
	ctx context.Context, qb *qbclient.Client, current map[string]string, folders map[string]bool,
	feed downloadsv1alpha1.QBittorrentRSSFeed, owned *engine.Ownership,
) error {
	if url, exists := current[feed.Name]; exists {
		changed, err := engine.CorrectDifference("qbittorrent", "rssFeeds", feed.Name, map[string]any{"url": url}, map[string]any{"url": feed.URL})
		if err != nil || !changed {
			return err
		}
		if other := qbRSSFeedAt(current, feed.URL); other != "" {
			return fmt.Errorf("qBittorrent already reads %s as %q", feed.URL, other)
		}
		if err := qb.SetRSSFeedURL(ctx, feed.Name, feed.URL); err != nil {
			return err
		}
		current[feed.Name] = feed.URL
		return nil
	}
	if other := qbRSSFeedAt(current, feed.URL); other != "" {
		return fmt.Errorf("qBittorrent already reads %s as %q; rename or remove that feed", feed.URL, other)
	}
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
	current[feed.Name] = feed.URL
	owned.Created("rssFeeds", feed.Name)
	return nil
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
func reconcileQBRSSRules(
	ctx context.Context, qb *qbclient.Client, rules []downloadsv1alpha1.QBittorrentRSSRule, prune bool, owned *engine.Ownership,
) []string {
	current, err := qb.RSSRules(ctx)
	if err != nil {
		return []string{fmt.Sprintf("listing RSS rules: %v", err)}
	}
	var errs []string
	for _, rule := range rules {
		if err := reconcileQBRSSRule(ctx, qb, current, rule, owned); err != nil {
			errs = append(errs, fmt.Sprintf("rssRule(%s): %v", rule.Name, err))
		}
	}
	if !prune || len(errs) > 0 {
		return errs
	}
	err = owned.Prune("rssRules", qbRSSRuleNames(rules), slices.Collect(maps.Keys(current)), func(name string) error {
		return qb.RemoveRSSRule(ctx, name)
	})
	if err != nil {
		return []string{fmt.Sprintf("rssRules(prune): %v", err)}
	}
	return nil
}

func reconcileQBRSSRule(
	ctx context.Context, qb *qbclient.Client, current map[string]any, rule downloadsv1alpha1.QBittorrentRSSRule, owned *engine.Ownership,
) error {
	desired := qbRSSRuleDef(rule)
	existing, exists := current[rule.Name].(map[string]any)
	if !exists {
		if err := qb.SetRSSRule(ctx, rule.Name, desired); err != nil {
			return err
		}
		owned.Created("rssRules", rule.Name)
		return nil
	}
	changed, err := engine.CorrectDifference("qbittorrent", "rssRules", rule.Name, existing, desired)
	if err != nil || !changed {
		return err
	}
	out, err := reconciler.MergeDesired(existing, desired)
	if err != nil {
		return err
	}
	return qb.SetRSSRule(ctx, rule.Name, out.Merged)
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

// observeQBRSS reports how the declared feeds and rules differ from
// qBittorrent's, and what prune would delete.
func observeQBRSS(
	ctx context.Context, qb *qbclient.Client, spec downloadsv1alpha1.QBittorrentSpec, prune bool, owned *engine.Ownership, w *engine.Writes,
) error {
	if len(spec.RSSFeeds) > 0 {
		feeds, _, err := qb.RSSItems(ctx)
		if err != nil {
			return fmt.Errorf("listing RSS feeds: %w", err)
		}
		for _, feed := range spec.RSSFeeds {
			var current map[string]any
			if url, ok := feeds[feed.Name]; ok {
				current = map[string]any{"url": url}
			}
			if err := w.Compare("rssFeeds", feed.Name, current, map[string]any{"url": feed.URL}); err != nil {
				return err
			}
		}
		if prune {
			if err := w.Prune(owned, "rssFeeds", qbRSSFeedNames(spec.RSSFeeds), slices.Collect(maps.Keys(feeds)), nil); err != nil {
				return err
			}
		}
	}
	if len(spec.RSSRules) > 0 {
		rules, err := qb.RSSRules(ctx)
		if err != nil {
			return fmt.Errorf("listing RSS rules: %w", err)
		}
		for _, rule := range spec.RSSRules {
			current, _ := rules[rule.Name].(map[string]any)
			if err := w.Compare("rssRules", rule.Name, current, qbRSSRuleDef(rule)); err != nil {
				return err
			}
		}
		if prune {
			return w.Prune(owned, "rssRules", qbRSSRuleNames(spec.RSSRules), slices.Collect(maps.Keys(rules)), nil)
		}
	}
	return nil
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
