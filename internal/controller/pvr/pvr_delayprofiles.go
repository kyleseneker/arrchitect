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
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/jsondiff"
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
	client *httpapi.HTTPClient
	path   string
	labels map[int]string
}

// reconcileDelayProfiles matches the declared profiles to the app's profiles
// holding their tags, and one without tags to the default profile. The app
// allows a tag in only one profile, so pruned profiles go first and tags leave
// a profile before any joins another.
func reconcileDelayProfiles(
	ctx context.Context, client *httpapi.HTTPClient, apiVersion string, declared []commonv1alpha1.DelayProfile, pass *engine.Pass, plannedTags []string,
) []error {
	s := &delayProfileStage{Pass: pass, client: client, path: fmt.Sprintf("/api/%s/delayprofile", apiVersion)}
	return s.run(ctx, fmt.Sprintf("/api/%s/tag", apiVersion), declared, plannedTags)
}

func (s *delayProfileStage) run(ctx context.Context, tagPath string, declared []commonv1alpha1.DelayProfile, plannedTags []string) []error {
	tags, err := engine.FetchNamedIDs(ctx, s.client, tagPath, "label")
	if err != nil {
		return []error{fmt.Errorf("%s: %w", delayProfilesKind, err)}
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
		return []error{fmt.Errorf("%s: listing: %w", delayProfilesKind, err)}
	}
	owned := map[string]bool{}
	if s.PruneEnabled {
		for _, id := range s.Owned.Managed()[delayProfilesKind] {
			owned[id] = true
		}
	}
	prunable := func(profile map[string]any) bool { return owned[delayProfileID(profile)] }
	plans, err := planDelayProfiles(declared, tags, existing, prunable)
	if err != nil {
		return []error{err}
	}

	if err := s.pruneUnmatched(ctx, plans, existing); err != nil {
		return []error{fmt.Errorf("%s(prune): %w", delayProfilesKind, err)}
	}
	var errs []error
	if !s.Observe {
		for _, p := range plans {
			if err := s.releaseTags(ctx, p); err != nil {
				return []error{fmt.Errorf("%s(%s): %w", delayProfilesKind, p.name, err)}
			}
		}
	}
	var template map[string]any
	for _, p := range plans {
		if p.current == nil && template == nil && hasProtocolItems(existing) {
			if template, err = s.client.GetJSON(ctx, s.path+"/schema"); err != nil {
				errs = append(errs, fmt.Errorf("%s(%s): reading the schema: %w", delayProfilesKind, p.name, err))
				continue
			}
		}
		if err := s.apply(ctx, p, template, hasProtocolItems(existing)); err != nil {
			errs = append(errs, fmt.Errorf("%s(%s): %w", delayProfilesKind, p.name, err))
		}
	}
	if len(errs) > 0 {
		return errs
	}
	if err := s.order(ctx, plans); err != nil {
		return []error{fmt.Errorf("%s(order): %w", delayProfilesKind, err)}
	}
	return nil
}

// planDelayProfiles matches the declared profiles to the app's. prunable
// reports a profile this pass deletes if no declaration matches it.
func planDelayProfiles(
	declared []commonv1alpha1.DelayProfile, tags engine.NamedIDs, existing []map[string]any, prunable func(map[string]any) bool,
) ([]*delayPlan, error) {
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
	var taggedPlans []*delayPlan
	claimed := map[int]string{}
	hasDefault := false
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
		plans = append(plans, p)
		if len(ids) == 0 {
			switch {
			case fallback == nil:
				return nil, fail("the app has no default profile")
			case hasDefault:
				return nil, fail("only one profile can be without tags")
			}
			hasDefault = true
			p.current = fallback
			continue
		}
		for _, id := range ids {
			if other, ok := claimed[id]; ok {
				return nil, fail("shares a tag with %s, and the app allows a tag in only one profile", other)
			}
			claimed[id] = p.name
		}
		taggedPlans = append(taggedPlans, p)
	}
	matched := map[string]bool{}
	for i, j := range matchDelayProfiles(taggedPlans, tagged) {
		if j >= 0 {
			taggedPlans[i].current = tagged[j]
			matched[delayProfileID(tagged[j])] = true
		}
	}
	// A profile left as it is that holds a declared tag would refuse the write
	// that declares it, after earlier writes had moved other tags.
	for _, e := range tagged {
		if matched[delayProfileID(e)] || prunable(e) {
			continue
		}
		for _, id := range profileTags(e) {
			name, ok := claimed[id]
			if !ok {
				continue
			}
			label := strconv.Itoa(id)
			for l, ids := range tags {
				if slices.Contains(ids, id) {
					label = l
				}
			}
			return nil, failure.InvalidConfig.Errorf(
				"%s(%s): tag %q is in profile %s, which no declared profile takes over and the operator can't prune",
				delayProfilesKind, name, label, delayProfileID(e))
		}
	}
	return plans, nil
}

// matchDelayProfiles picks the app's tagged profile each tagged plan updates,
// or -1 for one it creates. In spec order, each plan takes the untaken profile
// holding most of its tags, and on a tie the one the app checks first. A
// layout this leaves blocked is refused by planDelayProfiles before anything
// is written.
func matchDelayProfiles(plans []*delayPlan, profiles []map[string]any) []int {
	match := make([]int, len(plans))
	taken := make([]bool, len(profiles))
	for i, p := range plans {
		match[i] = -1
		best := 0
		for j, profile := range profiles {
			n := overlap(profileTags(profile), p.tags)
			if taken[j] || n == 0 {
				continue
			}
			if n > best || (n == best && num(profile["order"]) < num(profiles[match[i]]["order"])) {
				match[i], best = j, n
			}
		}
		if match[i] >= 0 {
			taken[match[i]] = true
		}
	}
	return match
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
	label := func(id string) string { return names[id] }
	err := s.PruneOwned(s.Owned, delayProfilesKind, unmatched, label, func(id string) error {
		return s.client.Delete(ctx, s.path+"/"+id)
	})
	kept, rest := engine.SplitKept(err)
	for _, k := range kept {
		for _, r := range k.Kept {
			rest = failure.Join(rest, fmt.Errorf("%s: %s", names[r.Name], r.Reason))
		}
	}
	return rest
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
	out, err := jsondiff.MergeDesired(base, set)
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
	var ids, planned []string
	for _, p := range plans {
		switch {
		case len(p.tags) == 0:
		case p.current != nil:
			ids = append(ids, delayProfileID(p.current))
		case s.Observe:
			// Enforce would have created it after the app's tagged profiles.
			ids = append(ids, "planned:"+p.name)
			planned = append(planned, ids[len(ids)-1])
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
		last := 0.0
		for _, profile := range profiles {
			positions[delayProfileID(profile)] = num(profile["order"])
			if len(profileTags(profile)) > 0 {
				last = max(last, num(profile["order"]))
			}
		}
		for k, id := range planned {
			positions[id] = last + float64(k+1)
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
