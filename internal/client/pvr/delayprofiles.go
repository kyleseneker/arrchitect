package pvr

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

const delayProfilesKind = "delayProfiles"

// Lidarr lists protocols as items, preferred first, instead of the fields the
// other apps have.
var delayProtocols = []struct{ key, protocol, name string }{
	{"usenet", "UsenetDownloadProtocol", "Usenet"},
	{"torrent", "TorrentDownloadProtocol", "Torrent"},
}

// Delay profiles have no name. Ownership is by the app's id, which a change of
// tags keeps.
func delayProfileID(item map[string]any) string {
	id, _ := item["id"].(float64)
	if id <= 0 {
		return ""
	}
	return strconv.Itoa(int(id))
}

type delayPlan struct {
	name    string
	tags    []int
	spec    commonv1alpha1.DelayProfile
	current map[string]any
}

type delayProfileStage struct {
	*engine.Pass
	client *engine.HTTPClient
	path   string
	labels map[int]string
}

// reconcileDelayProfiles matches each declared profile to the app's profile
// holding any of its tags, or to the default profile when it has none. The app
// allows a tag in only one profile, so pruned profiles go first and tags leave
// a profile before any joins another.
func reconcileDelayProfiles(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, declared []commonv1alpha1.DelayProfile, pass *engine.Pass, plannedTags []string,
) []string {
	s := &delayProfileStage{Pass: pass, client: client, path: fmt.Sprintf("/api/%s/delayprofile", apiVersion)}
	return s.run(ctx, fmt.Sprintf("/api/%s/tag", apiVersion), declared, plannedTags)
}

func (s *delayProfileStage) run(ctx context.Context, tagPath string, declared []commonv1alpha1.DelayProfile, plannedTags []string) []string {
	tags, err := engine.FetchNamedIDs(ctx, s.client, tagPath, "label")
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", delayProfilesKind, err)}
	}
	tags.Plan(plannedTags...)
	s.labels = map[int]string{}
	for label, ids := range tags {
		for _, id := range ids {
			s.labels[id] = label
		}
	}
	existing, err := s.client.GetJSONList(ctx, s.path)
	if err != nil {
		return []string{fmt.Sprintf("%s: listing: %v", delayProfilesKind, err)}
	}
	plans, err := planDelayProfiles(declared, tags, existing)
	if err != nil {
		return []string{err.Error()}
	}

	if err := s.pruneUnmatched(ctx, plans, existing); err != nil {
		return []string{fmt.Sprintf("%s(prune): %v", delayProfilesKind, err)}
	}
	var errs []string
	if !s.Observe {
		for _, p := range plans {
			if err := s.releaseTags(ctx, p); err != nil {
				return []string{fmt.Sprintf("%s(%s): %v", delayProfilesKind, p.name, err)}
			}
		}
	}
	var template map[string]any
	for _, p := range plans {
		if p.current == nil && template == nil && hasProtocolItems(existing) {
			if template, err = s.client.GetJSON(ctx, s.path+"/schema"); err != nil {
				errs = append(errs, fmt.Sprintf("%s(%s): reading the schema: %v", delayProfilesKind, p.name, err))
				continue
			}
		}
		if err := s.apply(ctx, p, template, hasProtocolItems(existing)); err != nil {
			errs = append(errs, fmt.Sprintf("%s(%s): %v", delayProfilesKind, p.name, err))
		}
	}
	if len(errs) > 0 {
		return errs
	}
	if err := s.order(ctx, plans); err != nil {
		return []string{fmt.Sprintf("%s(order): %v", delayProfilesKind, err)}
	}
	return nil
}

func planDelayProfiles(declared []commonv1alpha1.DelayProfile, tags engine.NamedIDs, existing []map[string]any) ([]*delayPlan, error) {
	var fallback map[string]any
	var tagged []map[string]any
	for _, e := range existing {
		if len(profileTags(e)) == 0 {
			fallback = e
		} else {
			tagged = append(tagged, e)
		}
	}

	plans := make([]*delayPlan, 0, len(declared))
	claimed := map[int]string{}
	matched := map[string]bool{}
	for _, d := range declared {
		p := &delayPlan{name: "default", spec: d}
		if len(d.TagLabels) > 0 {
			p.name = strings.Join(d.TagLabels, ",")
		}
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%s(%s): %s", delayProfilesKind, p.name, fmt.Sprintf(format, args...))
		}
		ids, err := tags.ResolveTags(d.TagLabels)
		if err != nil {
			return nil, fail("%v", err)
		}
		slices.Sort(ids)
		p.tags = ids
		if len(ids) == 0 {
			switch {
			case fallback == nil:
				return nil, fail("the app has no default profile")
			case matched[delayProfileID(fallback)]:
				return nil, fail("only one profile can be without tags")
			}
			matched[delayProfileID(fallback)] = true
			p.current = fallback
			plans = append(plans, p)
			continue
		}
		for _, id := range ids {
			if other, ok := claimed[id]; ok {
				return nil, fail("shares a tag with %s, and the app allows a tag in only one profile", other)
			}
			claimed[id] = p.name
		}
		var best map[string]any
		bestOverlap := 0
		for _, e := range tagged {
			if matched[delayProfileID(e)] {
				continue
			}
			n := overlap(profileTags(e), ids)
			if n > bestOverlap || (n > 0 && n == bestOverlap && num(e["order"]) < num(best["order"])) {
				best, bestOverlap = e, n
			}
		}
		if best != nil {
			matched[delayProfileID(best)] = true
			p.current = best
		}
		plans = append(plans, p)
	}
	return plans, nil
}

// pruneUnmatched deletes owned profiles no declared profile matched, before
// any write that could need their tags.
func (s *delayProfileStage) pruneUnmatched(ctx context.Context, plans []*delayPlan, existing []map[string]any) error {
	if !s.PruneEnabled {
		return nil
	}
	matched := map[string]bool{}
	for _, p := range plans {
		if p.current != nil {
			matched[delayProfileID(p.current)] = true
		}
	}
	names := map[string]string{}
	var unmatched []string
	for _, e := range existing {
		if id := delayProfileID(e); id != "" && !matched[id] {
			unmatched = append(unmatched, id)
			names[id] = s.describe(e)
		}
	}
	if s.Observe {
		for _, id := range unmatched {
			if slices.Contains(s.Owned.Managed()[delayProfilesKind], id) {
				s.RecordDrift(delayProfilesKind, names[id])
			}
		}
		return nil
	}
	return s.Owned.Cleanup(delayProfilesKind, unmatched, func(id string) error {
		err := s.client.Delete(ctx, s.path+"/"+id)
		if reason, refused := engine.Refusal(err); refused {
			return fmt.Errorf("the app refused: %s", reason)
		}
		return err
	})
}

// releaseTags drops the tags a matched profile is losing, so another declared
// profile can take them.
func (s *delayProfileStage) releaseTags(ctx context.Context, p *delayPlan) error {
	if p.current == nil || len(p.tags) == 0 {
		return nil
	}
	current := profileTags(p.current)
	kept := slices.DeleteFunc(slices.Clone(current), func(id int) bool { return !slices.Contains(p.tags, id) })
	if len(kept) == len(current) {
		return nil
	}
	tags := make([]any, 0, len(kept))
	for _, id := range kept {
		tags = append(tags, float64(id))
	}
	body := maps.Clone(p.current)
	body["tags"] = tags
	return s.Apply(delayProfilesKind, p.name, func() error {
		if err := s.client.PutJSON(ctx, s.path+"/"+delayProfileID(p.current), body); err != nil {
			return err
		}
		p.current = body
		return nil
	})
}

func (s *delayProfileStage) apply(ctx context.Context, p *delayPlan, template map[string]any, protocolItems bool) error {
	if p.current == nil {
		body := map[string]any{"enableUsenet": true, "enableTorrent": true, "preferredProtocol": "usenet"}
		if protocolItems {
			body = map[string]any{"items": template["items"]}
		}
		desired, err := delayProfileBody(body, p, protocolItems)
		if err != nil {
			return err
		}
		return s.Apply(delayProfilesKind, p.name, func() error {
			created, err := s.client.Do(ctx, http.MethodPost, s.path, desired)
			if err != nil {
				return err
			}
			var stored map[string]any
			if err := json.Unmarshal(created, &stored); err != nil || delayProfileID(stored) == "" {
				return fmt.Errorf("the app did not return the created profile")
			}
			p.current = stored
			s.Owned.Created(delayProfilesKind, delayProfileID(stored))
			s.Synced(delayProfilesKind + "(" + p.name + ")")
			return nil
		})
	}

	desired, err := delayProfileBody(p.current, p, protocolItems)
	if err != nil {
		return err
	}
	if same(p.current, desired) {
		s.Synced(delayProfilesKind + "(" + p.name + ")")
		return nil
	}
	return s.Apply(delayProfilesKind, p.name, func() error {
		if err := s.client.PutJSON(ctx, s.path+"/"+delayProfileID(p.current), desired); err != nil {
			return err
		}
		s.Synced(delayProfilesKind + "(" + p.name + ")")
		return nil
	})
}

// delayProfileBody overlays the declared settings on base, the app's profile or
// a new one.
func delayProfileBody(base map[string]any, p *delayPlan, protocolItems bool) (map[string]any, error) {
	d := p.spec
	set := map[string]any{}
	for key, value := range map[string]*bool{
		"bypassIfHighestQuality":         d.BypassIfHighestQuality,
		"bypassIfAboveCustomFormatScore": d.BypassIfAboveCustomFormatScore,
	} {
		if value != nil {
			set[key] = *value
		}
	}
	if d.MinimumCustomFormatScore != nil {
		set["minimumCustomFormatScore"] = *d.MinimumCustomFormatScore
	}
	if len(p.tags) > 0 {
		set["tags"] = p.tags
		if current := profileTags(base); len(current) == len(p.tags) && overlap(current, p.tags) == len(p.tags) {
			set["tags"] = current
		}
	}
	if !protocolItems {
		for key, value := range map[string]*bool{"enableUsenet": d.EnableUsenet, "enableTorrent": d.EnableTorrent} {
			if value != nil {
				set[key] = *value
			}
		}
		for key, value := range map[string]*int32{"usenetDelay": d.UsenetDelay, "torrentDelay": d.TorrentDelay} {
			if value != nil {
				set[key] = *value
			}
		}
		if d.PreferredProtocol != "" {
			set["preferredProtocol"] = d.PreferredProtocol
		}
	}
	out, err := reconciler.MergeDesired(base, set)
	if err != nil {
		return nil, err
	}
	if protocolItems {
		out.Merged["items"] = protocolItemsFor(base["items"], d)
	}
	return out.Merged, nil
}

func protocolItemsFor(current any, d commonv1alpha1.DelayProfile) []any {
	raw, _ := current.([]any)
	items := make([]any, 0, len(raw)+len(delayProtocols))
	seen := map[string]bool{}
	for _, it := range raw {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}
		item = maps.Clone(item)
		protocol, _ := item["protocol"].(string)
		seen[protocol] = true
		items = append(items, item)
	}
	for _, p := range delayProtocols {
		if !seen[p.protocol] {
			items = append(items, map[string]any{"name": p.name, "protocol": p.protocol, "allowed": false, "delay": 0})
		}
	}
	settings := map[string]struct {
		allowed *bool
		delay   *int32
	}{
		delayProtocols[0].protocol: {d.EnableUsenet, d.UsenetDelay},
		delayProtocols[1].protocol: {d.EnableTorrent, d.TorrentDelay},
	}
	for _, it := range items {
		item, _ := it.(map[string]any)
		protocol, _ := item["protocol"].(string)
		if setting, ok := settings[protocol]; ok {
			if setting.allowed != nil {
				item["allowed"] = *setting.allowed
			}
			if setting.delay != nil {
				item["delay"] = *setting.delay
			}
		}
	}
	preferred := ""
	for _, p := range delayProtocols {
		if p.key == d.PreferredProtocol {
			preferred = p.protocol
		}
	}
	if preferred != "" {
		slices.SortStableFunc(items, func(a, b any) int {
			pa, _ := a.(map[string]any)["protocol"].(string)
			pb, _ := b.(map[string]any)["protocol"].(string)
			switch {
			case pa == preferred && pb != preferred:
				return -1
			case pb == preferred && pa != preferred:
				return 1
			}
			return 0
		})
	}
	return items
}

// order moves declared tagged profiles the app checks out of turn, keeping
// where it has the others. Sonarr and Radarr read the profile to follow from
// after, Lidarr and Readarr from afterId.
func (s *delayProfileStage) order(ctx context.Context, plans []*delayPlan) error {
	var ids []string
	for _, p := range plans {
		if len(p.tags) > 0 && p.current != nil {
			ids = append(ids, delayProfileID(p.current))
		}
	}
	if len(ids) < 2 {
		return nil
	}
	profiles, err := s.client.GetJSONList(ctx, s.path)
	if err != nil {
		return err
	}
	for i := 1; i < len(ids); i++ {
		positions := map[string]float64{}
		for _, profile := range profiles {
			positions[delayProfileID(profile)] = num(profile["order"])
		}
		if positions[ids[i]] > positions[ids[i-1]] {
			continue
		}
		err := s.Apply(delayProfilesKind, "order", func() error {
			body, err := s.client.Do(ctx, http.MethodPut, fmt.Sprintf("%s/reorder/%s?after=%s&afterId=%s", s.path, ids[i], ids[i-1], ids[i-1]), nil)
			if err != nil {
				return err
			}
			profiles = nil
			return json.Unmarshal(body, &profiles)
		})
		if err != nil || s.Observe {
			return err
		}
	}
	return nil
}

func (s *delayProfileStage) describe(profile map[string]any) string {
	ids := profileTags(profile)
	if len(ids) == 0 {
		return "default"
	}
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		if label, ok := s.labels[id]; ok {
			labels = append(labels, label)
		} else {
			labels = append(labels, strconv.Itoa(id))
		}
	}
	return strings.Join(labels, ",")
}

func hasProtocolItems(existing []map[string]any) bool {
	for _, e := range existing {
		if _, ok := e["items"]; ok {
			return true
		}
	}
	return false
}

func profileTags(profile map[string]any) []int {
	raw, _ := profile["tags"].([]any)
	ids := make([]int, 0, len(raw))
	for _, v := range raw {
		if id, ok := v.(float64); ok {
			ids = append(ids, int(id))
		}
	}
	return ids
}

func overlap(a, b []int) int {
	n := 0
	for _, id := range a {
		if slices.Contains(b, id) {
			n++
		}
	}
	return n
}
