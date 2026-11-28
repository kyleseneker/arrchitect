// Command compatfloors prints the oldest release of each app the compat suite
// should pin under docs/stability.md: the newest stable release out 12 months
// before the range is set, or the app's own minimum where that is later. Run
// it before a minor release, then move oldestImages and the README to match.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

const imagesFile = "test/compat/images_test.go"

type source int

const (
	githubReleases source = iota
	githubTags
	imageBuilds
	archived
)

// app says where an app's release dates come from. minimum is the first
// release the operator works with, for a range that starts later.
type app struct {
	name    string
	repo    string
	from    source
	minimum string
}

var apps = map[string]app{
	"sonarrImage":       {name: "Sonarr", repo: "Sonarr/Sonarr"},
	"radarrImage":       {name: "Radarr", repo: "Radarr/Radarr"},
	"lidarrImage":       {name: "Lidarr", repo: "Lidarr/Lidarr"},
	"readarrImage":      {name: "Readarr", from: archived},
	"prowlarrImage":     {name: "Prowlarr", repo: "Prowlarr/Prowlarr"},
	"flaresolverrImage": {name: "FlareSolverr", repo: "FlareSolverr/FlareSolverr"},
	"autobrrImage":      {name: "Autobrr", repo: "autobrr/autobrr"},
	"bazarrImage":       {name: "Bazarr", repo: "morpheus65535/bazarr"},
	"jellyfinImage":     {name: "Jellyfin", repo: "jellyfin/jellyfin"},
	"seerrImage":        {name: "Seerr", repo: "seerr-team/seerr", minimum: "3.5.0"},
	"maintainerrImage":  {name: "Maintainerr", repo: "Maintainerr/Maintainerr", minimum: "3.30.0"},
	// Tdarr's GitHub releases start at 3.09.00, so its image builds date it.
	"tdarrImage":       {name: "Tdarr", from: imageBuilds, minimum: "3.00.00"},
	"sabnzbdImage":     {name: "SABnzbd", repo: "sabnzbd/sabnzbd"},
	"plexImage":        {name: "Plex", from: imageBuilds},
	"qbittorrentImage": {name: "qBittorrent", repo: "qbittorrent/qBittorrent", from: githubTags},
}

var (
	newestPin      = regexp.MustCompile(`(?m)^\t(\w+Image) += "([^"]+)"$`)
	mapPin         = regexp.MustCompile(`(?m)^\t(\w+Image): +"([^"]+)",$`)
	leadingVersion = regexp.MustCompile(`^\d+(\.\d+)*`)
	versionRun     = regexp.MustCompile(`\d+(?:\.\d+)*`)
	hashSuffix     = regexp.MustCompile(`-[0-9a-f]{7,}$`)
	stableTag      = regexp.MustCompile(`^release-\d+\.\d+\.\d+$`)
)

type release struct {
	version string
	date    time.Time
}

type result struct {
	name, now, image, note string
	floor                  release
	err                    error
}

func main() {
	asOf := flag.String("as-of", time.Now().UTC().Format(time.DateOnly), "the day the range is set, as YYYY-MM-DD")
	flag.Parse()
	day, err := time.Parse(time.DateOnly, *asOf)
	if err != nil {
		fail(err)
	}
	src, err := os.ReadFile(imagesFile)
	if err != nil {
		fail(fmt.Errorf("%w; run it from the repository root", err))
	}
	oldest := pins(mapPin, mapBlock(src, "oldestImages"))
	previous := pins(mapPin, mapBlock(src, "previousMajorImages"))
	cutoff := day.AddDate(-1, 0, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c := &client{
		http:   &http.Client{Timeout: time.Minute},
		token:  cmp.Or(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN")),
		tokens: map[string]string{},
		pushed: map[string]time.Time{},
	}
	matches := newestPin.FindAllSubmatch(src, -1)
	results := make([]result, len(matches))
	var wg sync.WaitGroup
	for i, m := range matches {
		pin, ref := string(m[1]), string(m[2])
		_, hasPrevious := previous[pin]
		wg.Go(func() { results[i] = c.check(ctx, pin, ref, oldest[pin], hasPrevious, cutoff) })
	}
	wg.Wait()
	if report(os.Stdout, results, day, cutoff, matches) {
		os.Exit(1)
	}
}

func report(w io.Writer, results []result, day, cutoff time.Time, matches [][][]byte) (failed bool) {
	_, _ = fmt.Fprintf(w, "Floors for a range set on %s: each app's newest release out by %s.\n\n",
		day.Format(time.DateOnly), cutoff.Format(time.DateOnly))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tNOW\tFLOOR\tRELEASED\tCHANGE")
	var moves []string
	for i, r := range results {
		released := "-"
		switch {
		case r.floor.version != "" && r.floor.date.IsZero():
			released = "minimum"
		case r.floor.version != "":
			released = r.floor.date.Format(time.DateOnly)
		}
		change := cmp.Or(r.note, "none")
		if r.image != "" {
			change = r.image
			if r.note != "" {
				change += "; " + r.note
			}
			moves = append(moves, fmt.Sprintf("\t%s: %q,", string(matches[i][1]), r.image))
		}
		if r.err != nil {
			change, failed = "error: "+r.err.Error(), true
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.name, r.now, cmp.Or(r.floor.version, "-"), released, change)
	}
	_ = tw.Flush()
	if len(moves) > 0 {
		_, _ = fmt.Fprintf(w, "\nMove these in oldestImages and the README's supported apps table, and say so under\n"+
			"Upgrading in the release notes:\n\n%s\n", strings.Join(moves, "\n"))
	}
	return failed
}

func (c *client) check(ctx context.Context, pin, newestRef, oldestRef string, previous bool, cutoff time.Time) result {
	repo, newestTag, _ := strings.Cut(newestRef, ":")
	_, oldestTag, _ := strings.Cut(oldestRef, ":")
	a, ok := apps[pin]
	r := result{name: cmp.Or(a.name, pin), now: appVersion(oldestTag)}
	switch {
	case !ok:
		r.err = fmt.Errorf("no release source; add %s to apps in hack/compatfloors", pin)
		return r
	case a.from == archived:
		r.note = "archived upstream; stays at its final releases"
		return r
	}
	img := parseRepo(repo)
	var releases []release
	var err error
	switch a.from {
	case githubReleases:
		releases, err = c.githubReleases(ctx, a.repo, cutoff)
	case githubTags:
		releases, err = c.githubTags(ctx, a.repo, cutoff)
	default:
		var tags []string
		if tags, err = c.tags(ctx, img, ""); err == nil {
			releases, err = c.imageReleases(ctx, img, tags, shape(newestTag), cutoff)
		}
	}
	if err != nil {
		r.err = err
		return r
	}
	if r.floor, ok = floor(releases, cutoff, a.minimum); !ok {
		r.err = errors.New("found no releases")
		return r
	}
	switch compareVersions(r.floor.version, r.now) {
	case 0:
		return r
	case -1:
		r.note = "stays; a floor only moves up"
		return r
	}
	tag, err := c.newestBuild(ctx, img, r.floor.version, shape(newestTag), shape(oldestTag))
	if err != nil {
		r.err = err
		return r
	}
	r.image = repo + ":" + tag
	if previous && major(r.floor.version) == major(appVersion(newestTag)) {
		r.note = "drop its previousMajorImages entry too"
	}
	return r
}

// floor is the newest release out by cutoff, or minimum where that is later.
// An app with no release that old is supported from its first.
func floor(releases []release, cutoff time.Time, minimum string) (release, bool) {
	if len(releases) == 0 && minimum == "" {
		return release{}, false
	}
	byVersion := slices.SortedFunc(slices.Values(releases), func(a, b release) int {
		return compareVersions(b.version, a.version)
	})
	found := release{}
	for _, r := range byVersion {
		if !r.date.After(cutoff) {
			found = r
			break
		}
	}
	if found.version == "" && len(byVersion) > 0 {
		found = byVersion[len(byVersion)-1]
	}
	if minimum != "" && (found.version == "" || compareVersions(found.version, minimum) < 0) {
		return release{version: minimum}, true
	}
	return found, true
}

// appVersion is the version an image or release tag names, without the build
// number or suffixes: 4.0.20.3014-ls2 is 4.0.20, release-5.3.1 is 5.3.1.
func appVersion(tag string) string {
	for _, prefix := range []string{"release-", "develop-", "v"} {
		tag = strings.TrimPrefix(tag, prefix)
	}
	parts := strings.Split(leadingVersion.FindString(tag), ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, ".")
}

func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if c := cmp.Compare(x, y); c != 0 {
			return c
		}
	}
	return 0
}

func major(version string) string {
	m, _, _ := strings.Cut(version, ".")
	return m
}

// shape matches tags in the same form as tag, whatever their versions. The
// libtorrent major in a qBittorrent tag stays fixed, as Renovate keeps it.
func shape(tag string) *regexp.Regexp {
	suffix := ""
	if h := hashSuffix.FindString(tag); strings.ContainsAny(h, "abcdef") {
		tag, suffix = strings.TrimSuffix(tag, h), `-[0-9a-f]+`
	}
	var b strings.Builder
	last := 0
	for _, loc := range versionRun.FindAllStringIndex(tag, -1) {
		literal := tag[last:loc[0]]
		b.WriteString(regexp.QuoteMeta(literal))
		if strings.HasSuffix(literal, "_v") {
			b.WriteString(regexp.QuoteMeta(major(tag[loc[0]:loc[1]])) + `(?:\.\d+)*`)
		} else {
			b.WriteString(`\d+(?:\.\d+)*`)
		}
		last = loc[1]
	}
	b.WriteString(regexp.QuoteMeta(tag[last:]))
	return regexp.MustCompile("^" + b.String() + suffix + "$")
}

func pins(pattern *regexp.Regexp, src []byte) map[string]string {
	out := map[string]string{}
	for _, m := range pattern.FindAllSubmatch(src, -1) {
		out[string(m[1])] = string(m[2])
	}
	return out
}

func mapBlock(src []byte, name string) []byte {
	m := regexp.MustCompile(`(?s)var ` + name + ` = map\[string\]string\{\n(.*?)\n\}`).FindSubmatch(src)
	if m == nil {
		return nil
	}
	return m[1]
}

type imageRepo struct{ host, name string }

func parseRepo(repo string) imageRepo {
	host, name, _ := strings.Cut(repo, "/")
	if host == "lscr.io" {
		return imageRepo{host: "ghcr.io", name: name}
	}
	return imageRepo{host: host, name: name}
}

type client struct {
	http   *http.Client
	token  string
	mu     sync.Mutex
	tokens map[string]string
	pushed map[string]time.Time
}

func (c *client) getJSON(ctx context.Context, rawURL string, header http.Header, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if header != nil {
		req.Header = header.Clone()
	}
	req.Header.Set("User-Agent", "arrchitect-compatfloors")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("GET %s: %s: %s", rawURL, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *client) github(ctx context.Context, path string, out any) error {
	header := http.Header{"Accept": {"application/vnd.github+json"}}
	if c.token != "" {
		header.Set("Authorization", "Bearer "+c.token)
	}
	return c.getJSON(ctx, "https://api.github.com/"+path, header, out)
}

// githubReleases lists stable releases, newest first, until a page reaches a
// quarter before cutoff, which leaves room for releases out of version order.
func (c *client) githubReleases(ctx context.Context, repo string, cutoff time.Time) ([]release, error) {
	var out []release
	for page := 1; ; page++ {
		var rs []struct {
			TagName     string    `json:"tag_name"`
			Draft       bool      `json:"draft"`
			Prerelease  bool      `json:"prerelease"`
			PublishedAt time.Time `json:"published_at"`
		}
		if err := c.github(ctx, fmt.Sprintf("repos/%s/releases?per_page=100&page=%d", repo, page), &rs); err != nil {
			return nil, err
		}
		for _, r := range rs {
			if !r.Draft && !r.Prerelease {
				out = append(out, release{version: appVersion(r.TagName), date: r.PublishedAt})
			}
		}
		if len(rs) < 100 || rs[len(rs)-1].PublishedAt.Before(cutoff.AddDate(0, -3, 0)) {
			return out, nil
		}
	}
}

// githubTags dates stable release-x.y.z tags by their commits, newest version
// first, until one was out by cutoff.
func (c *client) githubTags(ctx context.Context, repo string, cutoff time.Time) ([]release, error) {
	type tag struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	var stable []tag
	for page := 1; ; page++ {
		var ts []tag
		if err := c.github(ctx, fmt.Sprintf("repos/%s/tags?per_page=100&page=%d", repo, page), &ts); err != nil {
			return nil, err
		}
		for _, t := range ts {
			if stableTag.MatchString(t.Name) {
				stable = append(stable, t)
			}
		}
		if len(ts) < 100 {
			break
		}
	}
	slices.SortFunc(stable, func(a, b tag) int { return compareVersions(appVersion(b.Name), appVersion(a.Name)) })
	var out []release
	for _, t := range stable {
		var commit struct {
			Commit struct {
				Committer struct {
					Date time.Time `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		}
		if err := c.github(ctx, fmt.Sprintf("repos/%s/commits/%s", repo, t.Commit.SHA), &commit); err != nil {
			return nil, err
		}
		out = append(out, release{version: appVersion(t.Name), date: commit.Commit.Committer.Date})
		if !commit.Commit.Committer.Date.After(cutoff) {
			break
		}
	}
	return out, nil
}

// imageReleases dates each version in the pinned form by its first build,
// newest version first, until one was out by cutoff.
func (c *client) imageReleases(ctx context.Context, img imageRepo, tags []string, form *regexp.Regexp,
	cutoff time.Time) ([]release, error) {
	byVersion := map[string][]string{}
	for _, t := range tags {
		if form.MatchString(t) {
			byVersion[appVersion(t)] = append(byVersion[appVersion(t)], t)
		}
	}
	versions := slices.SortedFunc(maps.Keys(byVersion), func(a, b string) int { return compareVersions(b, a) })
	var out []release
	for _, v := range versions {
		var first time.Time
		for _, t := range byVersion[v] {
			built, err := c.built(ctx, img, t)
			if err != nil {
				return nil, err
			}
			if first.IsZero() || built.Before(first) {
				first = built
			}
		}
		out = append(out, release{version: v, date: first})
		if !first.After(cutoff) {
			break
		}
	}
	return out, nil
}

// newestBuild picks the most recently built tag of version, in the newest
// pin's form where one exists, else in the oldest pin's.
func (c *client) newestBuild(ctx context.Context, img imageRepo, version string,
	forms ...*regexp.Regexp) (string, error) {
	tags, err := c.tags(ctx, img, version)
	if err != nil {
		return "", err
	}
	for _, form := range forms {
		best, bestBuilt := "", time.Time{}
		for _, t := range tags {
			if !form.MatchString(t) || appVersion(t) != version {
				continue
			}
			built, err := c.built(ctx, img, t)
			if err != nil {
				return "", err
			}
			if best == "" || built.After(bestBuilt) {
				best, bestBuilt = t, built
			}
		}
		if best != "" {
			return best, nil
		}
	}
	return "", fmt.Errorf("no %s tag of %s in the form of either pin", img.name, version)
}

// tags lists an image's tags. On Docker Hub, containing narrows the list to
// tags with that text in them, since Hub won't page through every tag of a
// busy image without signing in.
func (c *client) tags(ctx context.Context, img imageRepo, containing string) ([]string, error) {
	switch img.host {
	case "ghcr.io":
		return c.ghcrTags(ctx, img.name)
	case "docker.io":
		return c.hubTags(ctx, img.name, containing)
	}
	return nil, fmt.Errorf("can't list tags on %s", img.host)
}

func (c *client) built(ctx context.Context, img imageRepo, tag string) (time.Time, error) {
	if img.host == "ghcr.io" {
		return c.ghcrBuilt(ctx, img.name, tag)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pushed[img.name+":"+tag], nil
}

func (c *client) ghcrToken(ctx context.Context, name string) (http.Header, error) {
	c.mu.Lock()
	token, ok := c.tokens[name]
	c.mu.Unlock()
	if !ok {
		var t struct {
			Token string `json:"token"`
		}
		if err := c.getJSON(ctx, "https://ghcr.io/token?scope=repository:"+name+":pull", nil, &t); err != nil {
			return nil, err
		}
		token = t.Token
		c.mu.Lock()
		c.tokens[name] = token
		c.mu.Unlock()
	}
	return http.Header{"Authorization": {"Bearer " + token}}, nil
}

func (c *client) ghcrTags(ctx context.Context, name string) ([]string, error) {
	header, err := c.ghcrToken(ctx, name)
	if err != nil {
		return nil, err
	}
	var all []string
	last := ""
	for {
		u := "https://ghcr.io/v2/" + name + "/tags/list?n=1000"
		if last != "" {
			u += "&last=" + url.QueryEscape(last)
		}
		var page struct {
			Tags []string `json:"tags"`
		}
		if err := c.getJSON(ctx, u, header, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Tags...)
		if len(page.Tags) < 1000 {
			return all, nil
		}
		last = page.Tags[len(page.Tags)-1]
	}
}

// manifest is an image index, which lists one manifest per platform, or a
// single platform's manifest, which names its config.
type manifest struct {
	Manifests []platformManifest `json:"manifests"`
	Config    struct {
		Digest string `json:"digest"`
	} `json:"config"`
}

type platformManifest struct {
	Digest   string `json:"digest"`
	Platform struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
	} `json:"platform"`
}

func (c *client) ghcrBuilt(ctx context.Context, name, tag string) (time.Time, error) {
	header, err := c.ghcrToken(ctx, name)
	if err != nil {
		return time.Time{}, err
	}
	header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json",
	}, ","))
	base := "https://ghcr.io/v2/" + name
	var m manifest
	if err := c.getJSON(ctx, base+"/manifests/"+tag, header, &m); err != nil {
		return time.Time{}, err
	}
	if len(m.Manifests) > 0 {
		i := slices.IndexFunc(m.Manifests, func(e platformManifest) bool {
			return e.Platform.OS == "linux" && e.Platform.Architecture == "amd64"
		})
		if i < 0 {
			return time.Time{}, fmt.Errorf("%s:%s has no linux/amd64 image", name, tag)
		}
		digest := m.Manifests[i].Digest
		m = manifest{}
		if err := c.getJSON(ctx, base+"/manifests/"+digest, header, &m); err != nil {
			return time.Time{}, err
		}
	}
	var config struct {
		Created time.Time `json:"created"`
	}
	if err := c.getJSON(ctx, base+"/blobs/"+m.Config.Digest, header, &config); err != nil {
		return time.Time{}, err
	}
	return config.Created, nil
}

func (c *client) hubTags(ctx context.Context, name, containing string) ([]string, error) {
	var all []string
	next := "https://hub.docker.com/v2/repositories/" + name + "/tags?page_size=100"
	if containing != "" {
		next += "&name=" + url.QueryEscape(containing)
	}
	for next != "" {
		var page struct {
			Next    string `json:"next"`
			Results []struct {
				Name   string `json:"name"`
				Pushed string `json:"tag_last_pushed"`
			} `json:"results"`
		}
		if err := c.getJSON(ctx, next, nil, &page); err != nil {
			return nil, err
		}
		c.mu.Lock()
		for _, r := range page.Results {
			all = append(all, r.Name)
			c.pushed[name+":"+r.Name], _ = time.Parse(time.RFC3339Nano, r.Pushed)
		}
		c.mu.Unlock()
		next = page.Next
	}
	return all, nil
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
