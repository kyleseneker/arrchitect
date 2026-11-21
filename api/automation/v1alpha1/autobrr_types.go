/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AutobrrSpec defines the desired state of Autobrr.
type AutobrrSpec struct {
	// connection defines how to connect to the Autobrr instance.
	// +required
	Connection commonv1alpha1.AppConnection `json:"connection,omitempty"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// downloadClients defines download client connections in Autobrr.
	// +optional
	// +listType=map
	// +listMapKey=name
	DownloadClients []AutobrrDownloadClient `json:"downloadClients,omitempty"`

	// indexers defines indexer/tracker connections.
	// +optional
	// +listType=map
	// +listMapKey=name
	Indexers []AutobrrIndexer `json:"indexers,omitempty"`

	// ircNetworks defines IRC network connections for announce channel monitoring.
	// +optional
	// +listType=map
	// +listMapKey=name
	IRCNetworks []AutobrrIRCNetwork `json:"ircNetworks,omitempty"`

	// feeds defines RSS/Torznab feed sources.
	// +optional
	// +listType=map
	// +listMapKey=name
	Feeds []AutobrrFeed `json:"feeds,omitempty"`

	// filters defines release filters with match criteria and actions.
	// +optional
	// +listType=map
	// +listMapKey=name
	Filters []AutobrrFilter `json:"filters,omitempty"`
}

// AutobrrDownloadClient defines a download client connection in Autobrr.
// +kubebuilder:validation:XValidation:rule="has(self.appRef) ? !has(self.type) && !has(self.host) && !has(self.port) && !has(self.tls) && !has(self.usernameSecretRef) && !has(self.passwordSecretRef) && !has(self.apiKeySecretRef) : has(self.type) && has(self.host)",message="set appRef, or type and host, not both"
type AutobrrDownloadClient struct {
	// name is the display name of the download client.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// appRef names the download client's resource, a QBittorrent, Sabnzbd,
	// Sonarr, Radarr, Lidarr or Readarr, whose kind and connection are used in
	// place of type, host, port, tls and the credentials.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.kind in ['QBittorrent', 'Sabnzbd', 'Sonarr', 'Radarr', 'Lidarr', 'Readarr']",message="a download client refers to a QBittorrent, Sabnzbd, Sonarr, Radarr, Lidarr or Readarr"
	AppRef *commonv1alpha1.AppRef `json:"appRef,omitempty"`

	// enable controls whether this download client is active. A new download
	// client is enabled unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// type is the download client type. Deluge means Deluge 2 (Autobrr's DELUGE_V2).
	// +optional
	// +kubebuilder:validation:Enum=qBittorrent;Deluge;Transmission;rTorrent;SABnzbd;Sonarr;Radarr;Lidarr;Readarr;Whisparr
	Type string `json:"type,omitempty"`

	// host is the hostname or URL of the download client.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host,omitempty"`

	// port is the port of the download client.
	// +optional
	Port *int32 `json:"port,omitempty"`

	// tls enables TLS for the connection.
	// +optional
	TLS *bool `json:"tls,omitempty"`

	// usernameSecretRef references a Secret containing the username.
	// +optional
	UsernameSecretRef *commonv1alpha1.SecretKeyRef `json:"usernameSecretRef,omitempty"`

	// passwordSecretRef references a Secret containing the password.
	// +optional
	PasswordSecretRef *commonv1alpha1.SecretKeyRef `json:"passwordSecretRef,omitempty"`

	// apiKeySecretRef references a Secret containing the API key, for Sonarr, Radarr, Lidarr,
	// Readarr and SABnzbd clients.
	// +optional
	APIKeySecretRef *commonv1alpha1.SecretKeyRef `json:"apiKeySecretRef,omitempty"`
}

// AutobrrIndexer defines an indexer/tracker connection in Autobrr.
type AutobrrIndexer struct {
	// name is the display name of the indexer.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enable controls whether this indexer is active. A new indexer is enabled
	// unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// identifier is the Autobrr indexer definition, such as "ipt" or "btn", or
	// "torznab", "newznab" or "rss" for an indexer that a feed reads from.
	// +required
	// +kubebuilder:validation:MinLength=1
	Identifier string `json:"identifier,omitempty"`

	// baseURL is the tracker's site URL. It defaults to the definition's first URL.
	// +optional
	// +kubebuilder:validation:MinLength=1
	BaseURL string `json:"baseURL,omitempty"`

	// settings are the definition's settings, such as passkey or rsskey.
	// +optional
	// +listType=map
	// +listMapKey=name
	Settings []commonv1alpha1.ConfigField `json:"settings,omitempty"`
}

// AutobrrIRCNetwork defines an IRC network connection for announce monitoring.
type AutobrrIRCNetwork struct {
	// name is the display name of the IRC network.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enable controls whether this IRC network connection is active. A new
	// network is enabled unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// server is the IRC server address (e.g., "irc.example.com").
	// +required
	// +kubebuilder:validation:MinLength=1
	Server string `json:"server,omitempty"`

	// port is the IRC server port.
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// tls enables TLS for the IRC connection.
	// +optional
	TLS *bool `json:"tls,omitempty"`

	// nick is the IRC nickname to use.
	// +required
	// +kubebuilder:validation:MinLength=1
	Nick string `json:"nick,omitempty"`

	// authMechanism is the authentication mechanism (SASL_PLAIN, NickServ, None).
	// A new network uses None unless this is set.
	// +optional
	// +kubebuilder:validation:Enum=SASL_PLAIN;NickServ;None
	AuthMechanism string `json:"authMechanism,omitempty"`

	// authAccountSecretRef references a Secret containing the auth account name.
	// +optional
	AuthAccountSecretRef *commonv1alpha1.SecretKeyRef `json:"authAccountSecretRef,omitempty"`

	// authPasswordSecretRef references a Secret containing the auth password.
	// +optional
	AuthPasswordSecretRef *commonv1alpha1.SecretKeyRef `json:"authPasswordSecretRef,omitempty"`

	// inviteCommand is the IRC command to send to request channel invites.
	// +optional
	// +kubebuilder:validation:MinLength=1
	InviteCommand string `json:"inviteCommand,omitempty"`

	// channels defines the IRC channels to join.
	// +optional
	// +listType=map
	// +listMapKey=name
	Channels []AutobrrIRCChannel `json:"channels,omitempty"`
}

// AutobrrIRCChannel defines an IRC channel to monitor.
type AutobrrIRCChannel struct {
	// name is the channel name (e.g., "#announce").
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// passwordSecretRef references a Secret containing the channel password/key.
	// +optional
	PasswordSecretRef *commonv1alpha1.SecretKeyRef `json:"passwordSecretRef,omitempty"`
}

// AutobrrFeed defines an RSS or Torznab feed source.
type AutobrrFeed struct {
	// name is the display name of the feed.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enable controls whether this feed is active. A new feed is enabled unless
	// this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// type is the feed type.
	// +required
	// +kubebuilder:validation:Enum=RSS;Torznab;Newznab
	Type string `json:"type,omitempty"`

	// url is the feed URL.
	// +required
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url,omitempty"`

	// apiKeySecretRef references a Secret containing the feed API key (for Torznab/Newznab).
	// +optional
	APIKeySecretRef *commonv1alpha1.SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// interval is the feed polling interval in minutes. A new feed polls every
	// 15 minutes unless this is set.
	// +optional
	Interval *int32 `json:"interval,omitempty"`

	// indexerRef is the name of the indexer this feed belongs to: a torznab,
	// newznab or rss indexer declared under indexers.
	// +required
	// +kubebuilder:validation:MinLength=1
	IndexerRef string `json:"indexerRef,omitempty"`
}

// AutobrrFilter defines a release filter with match criteria and actions.
type AutobrrFilter struct {
	// name is the display name of the filter.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enable controls whether this filter is active. A new filter is enabled
	// unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// priority controls filter evaluation order. Lower values are evaluated first.
	// +optional
	Priority *int32 `json:"priority,omitempty"`

	// matchReleases is a comma-separated list of release name patterns to match (supports wildcards).
	// +optional
	// +kubebuilder:validation:MinLength=1
	MatchReleases string `json:"matchReleases,omitempty"`

	// exceptReleases is a comma-separated list of release name patterns to exclude.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExceptReleases string `json:"exceptReleases,omitempty"`

	// matchCategories is a comma-separated list of categories to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MatchCategories string `json:"matchCategories,omitempty"`

	// resolutions is a list of acceptable resolutions.
	// +optional
	// +listType=set
	Resolutions []string `json:"resolutions,omitempty"`

	// sources is a list of acceptable sources (e.g., "BluRay", "WEB-DL").
	// +optional
	// +listType=set
	Sources []string `json:"sources,omitempty"`

	// codecs is a list of acceptable codecs (e.g., "x264", "x265", "HEVC").
	// +optional
	// +listType=set
	Codecs []string `json:"codecs,omitempty"`

	// minSize is the minimum release size (e.g., "100MB", "1GB").
	// +optional
	// +kubebuilder:validation:MinLength=1
	MinSize string `json:"minSize,omitempty"`

	// maxSize is the maximum release size (e.g., "50GB").
	// +optional
	// +kubebuilder:validation:MinLength=1
	MaxSize string `json:"maxSize,omitempty"`

	// delay is the delay in seconds before grabbing a matched release.
	// +optional
	Delay *int32 `json:"delay,omitempty"`

	// useRegex enables regex matching for release patterns.
	// +optional
	UseRegex *bool `json:"useRegex,omitempty"`

	// tags is a list of tags for organizational purposes.
	// +optional
	// +listType=set
	Tags []string `json:"tags,omitempty"`

	// indexers lists the indexers, by name, whose releases this filter checks.
	// Autobrr checks a filter only against the indexers it lists.
	// +optional
	// +listType=set
	Indexers []string `json:"indexers,omitempty"`

	// matchReleaseGroups is a comma-separated list of release groups to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MatchReleaseGroups string `json:"matchReleaseGroups,omitempty"`

	// exceptReleaseGroups is a comma-separated list of release groups to skip.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExceptReleaseGroups string `json:"exceptReleaseGroups,omitempty"`

	// exceptCategories is a comma-separated list of categories to skip.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExceptCategories string `json:"exceptCategories,omitempty"`

	// matchUploaders is a comma-separated list of uploaders to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MatchUploaders string `json:"matchUploaders,omitempty"`

	// exceptUploaders is a comma-separated list of uploaders to skip.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExceptUploaders string `json:"exceptUploaders,omitempty"`

	// matchReleaseTags is a comma-separated list of release tags to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MatchReleaseTags string `json:"matchReleaseTags,omitempty"`

	// exceptReleaseTags is a comma-separated list of release tags to skip.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExceptReleaseTags string `json:"exceptReleaseTags,omitempty"`

	// useRegexReleaseTags reads the release tag patterns as regular expressions.
	// +optional
	UseRegexReleaseTags *bool `json:"useRegexReleaseTags,omitempty"`

	// matchDescription is a comma-separated list of description patterns to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MatchDescription string `json:"matchDescription,omitempty"`

	// exceptDescription is a comma-separated list of description patterns to skip.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExceptDescription string `json:"exceptDescription,omitempty"`

	// useRegexDescription reads the description patterns as regular expressions.
	// +optional
	UseRegexDescription *bool `json:"useRegexDescription,omitempty"`

	// containers is a list of acceptable containers, such as mkv.
	// +optional
	// +listType=set
	Containers []string `json:"containers,omitempty"`

	// matchHdr is a list of HDR formats to match, such as DV or HDR10+.
	// +optional
	// +listType=set
	MatchHdr []string `json:"matchHdr,omitempty"`

	// exceptHdr is a list of HDR formats to skip.
	// +optional
	// +listType=set
	ExceptHdr []string `json:"exceptHdr,omitempty"`

	// matchOther is a list of other release attributes to match, such as REMUX.
	// +optional
	// +listType=set
	MatchOther []string `json:"matchOther,omitempty"`

	// exceptOther is a list of other release attributes to skip.
	// +optional
	// +listType=set
	ExceptOther []string `json:"exceptOther,omitempty"`

	// origins is a list of origins to match, such as P2P, INTERNAL or SCENE.
	// +optional
	// +listType=set
	Origins []string `json:"origins,omitempty"`

	// exceptOrigins is a list of origins to skip.
	// +optional
	// +listType=set
	ExceptOrigins []string `json:"exceptOrigins,omitempty"`

	// announceTypes is a list of announce types to match, such as NEW or PROMO.
	// +optional
	// +listType=set
	AnnounceTypes []string `json:"announceTypes,omitempty"`

	// matchLanguage is a list of languages to match.
	// +optional
	// +listType=set
	MatchLanguage []string `json:"matchLanguage,omitempty"`

	// exceptLanguage is a list of languages to skip.
	// +optional
	// +listType=set
	ExceptLanguage []string `json:"exceptLanguage,omitempty"`

	// shows is a comma-separated list of shows to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Shows string `json:"shows,omitempty"`

	// seasons is a comma-separated list of seasons or ranges to match, such as 1,3-5.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Seasons string `json:"seasons,omitempty"`

	// episodes is a comma-separated list of episodes or ranges to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Episodes string `json:"episodes,omitempty"`

	// smartEpisode skips episodes older than ones already grabbed.
	// +optional
	SmartEpisode *bool `json:"smartEpisode,omitempty"`

	// years is a comma-separated list of years or ranges to match, such as 2018-2024.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Years string `json:"years,omitempty"`

	// months is a comma-separated list of months to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Months string `json:"months,omitempty"`

	// days is a comma-separated list of days to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Days string `json:"days,omitempty"`

	// freeleech matches only freeleech releases.
	// +optional
	Freeleech *bool `json:"freeleech,omitempty"`

	// freeleechPercent is the freeleech percentages to match, such as 50%,75%-100%.
	// +optional
	// +kubebuilder:validation:MinLength=1
	FreeleechPercent string `json:"freeleechPercent,omitempty"`

	// maxDownloads caps how many releases the filter grabs per maxDownloadsUnit.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxDownloads *int32 `json:"maxDownloads,omitempty"`

	// maxDownloadsUnit is the period maxDownloads counts over.
	// +optional
	// +kubebuilder:validation:Enum=MINUTE;HOUR;DAY;WEEK;MONTH;EVER
	MaxDownloadsUnit string `json:"maxDownloadsUnit,omitempty"`

	// minSeeders is the fewest seeders a release may have.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinSeeders *int32 `json:"minSeeders,omitempty"`

	// maxSeeders is the most seeders a release may have.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxSeeders *int32 `json:"maxSeeders,omitempty"`

	// minLeechers is the fewest leechers a release may have.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinLeechers *int32 `json:"minLeechers,omitempty"`

	// maxLeechers is the most leechers a release may have.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxLeechers *int32 `json:"maxLeechers,omitempty"`

	// exceptTags is a list of tags to skip.
	// +optional
	// +listType=set
	ExceptTags []string `json:"exceptTags,omitempty"`

	// tagsMatchLogic decides whether a release needs any or all of tags.
	// +optional
	// +kubebuilder:validation:Enum=ANY;ALL
	TagsMatchLogic string `json:"tagsMatchLogic,omitempty"`

	// exceptTagsMatchLogic decides whether any or all of exceptTags skip a release.
	// +optional
	// +kubebuilder:validation:Enum=ANY;ALL
	ExceptTagsMatchLogic string `json:"exceptTagsMatchLogic,omitempty"`

	// artists is a comma-separated list of artists to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Artists string `json:"artists,omitempty"`

	// albums is a comma-separated list of albums to match.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Albums string `json:"albums,omitempty"`

	// matchReleaseTypes is a list of music release types to match, such as Album or EP.
	// +optional
	// +listType=set
	MatchReleaseTypes []string `json:"matchReleaseTypes,omitempty"`

	// exceptReleaseTypes is a comma-separated list of music release types to skip.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExceptReleaseTypes string `json:"exceptReleaseTypes,omitempty"`

	// formats is a list of audio formats to match, such as FLAC or MP3.
	// +optional
	// +listType=set
	Formats []string `json:"formats,omitempty"`

	// quality is a list of audio qualities to match, such as Lossless or 320.
	// +optional
	// +listType=set
	Quality []string `json:"quality,omitempty"`

	// media is a list of music media to match, such as CD or WEB.
	// +optional
	// +listType=set
	Media []string `json:"media,omitempty"`

	// perfectFlac matches only lossless rips with a log of 100 and a cue.
	// +optional
	PerfectFlac *bool `json:"perfectFlac,omitempty"`

	// cue matches only releases with a cue file.
	// +optional
	Cue *bool `json:"cue,omitempty"`

	// log matches only releases with a rip log.
	// +optional
	Log *bool `json:"log,omitempty"`

	// logScore is the lowest rip log score to match.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	LogScore *int32 `json:"logScore,omitempty"`

	// actions defines what to do when a release matches this filter.
	// +optional
	// +listType=map
	// +listMapKey=name
	Actions []AutobrrFilterAction `json:"actions,omitempty"`
}

// AutobrrFilterAction defines an action to take when a filter matches.
type AutobrrFilterAction struct {
	// name is the display name of the action.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enable controls whether this action is active. A new action is enabled
	// unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// type is the action type. Deluge means Deluge 2 (Autobrr's DELUGE_V2).
	// +required
	// +kubebuilder:validation:Enum=qBittorrent;Deluge;Transmission;rTorrent;SABnzbd;Sonarr;Radarr;Lidarr;Readarr;Whisparr;Webhook;Exec;WatchFolder
	Type string `json:"type,omitempty"`

	// clientRef is the name of the download client to use (must match a downloadClients entry).
	// +optional
	// +kubebuilder:validation:MinLength=1
	ClientRef string `json:"clientRef,omitempty"`

	// category overrides the download client's default category.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Category string `json:"category,omitempty"`

	// savePath overrides the download client's default save path.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SavePath string `json:"savePath,omitempty"`

	// webhookURL is the webhook URL (for Webhook action type).
	// +optional
	// +kubebuilder:validation:MinLength=1
	WebhookURL string `json:"webhookURL,omitempty"`

	// execCommand is the command to execute (for Exec action type).
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExecCommand string `json:"execCommand,omitempty"`

	// execArgs is the arguments for the exec command.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExecArgs string `json:"execArgs,omitempty"`

	// watchFolder is the folder path (for WatchFolder action type).
	// +optional
	// +kubebuilder:validation:MinLength=1
	WatchFolder string `json:"watchFolder,omitempty"`

	// tags is a comma-separated list of tags the download client gives the torrent.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Tags string `json:"tags,omitempty"`

	// label is the label the download client gives the torrent.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Label string `json:"label,omitempty"`

	// downloadPath is where the download client keeps the torrent while it downloads.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DownloadPath string `json:"downloadPath,omitempty"`

	// paused adds the torrent paused.
	// +optional
	Paused *bool `json:"paused,omitempty"`

	// ignoreRules sends the release even when the download client is past its limits.
	// +optional
	IgnoreRules *bool `json:"ignoreRules,omitempty"`

	// skipHashCheck adds the torrent without checking existing data.
	// +optional
	SkipHashCheck *bool `json:"skipHashCheck,omitempty"`

	// firstLastPiecePrio downloads the first and last pieces first.
	// +optional
	FirstLastPiecePrio *bool `json:"firstLastPiecePrio,omitempty"`

	// contentLayout decides whether the torrent's files go in a folder of their own.
	// +optional
	// +kubebuilder:validation:Enum=ORIGINAL;SUBFOLDER_CREATE;SUBFOLDER_NONE
	ContentLayout string `json:"contentLayout,omitempty"`

	// priority is where the torrent goes in the download client's queue.
	// +optional
	// +kubebuilder:validation:Enum=MAX;MIN;LOW;NORMAL;HIGH
	Priority string `json:"priority,omitempty"`

	// limitUploadSpeed caps the torrent's upload speed, in KiB/s.
	// +optional
	// +kubebuilder:validation:Minimum=0
	LimitUploadSpeed *int32 `json:"limitUploadSpeed,omitempty"`

	// limitDownloadSpeed caps the torrent's download speed, in KiB/s.
	// +optional
	// +kubebuilder:validation:Minimum=0
	LimitDownloadSpeed *int32 `json:"limitDownloadSpeed,omitempty"`

	// limitRatio is the share ratio at which the torrent stops seeding.
	// +optional
	LimitRatio *commonv1alpha1.Decimal `json:"limitRatio,omitempty"`

	// limitSeedTime is how many minutes the torrent seeds.
	// +optional
	// +kubebuilder:validation:Minimum=0
	LimitSeedTime *int32 `json:"limitSeedTime,omitempty"`

	// reannounceSkip adds the torrent without waiting for it to announce.
	// +optional
	ReannounceSkip *bool `json:"reannounceSkip,omitempty"`

	// reannounceDelete removes the torrent if it never announces.
	// +optional
	ReannounceDelete *bool `json:"reannounceDelete,omitempty"`

	// reannounceInterval is how many seconds to wait between reannounce attempts.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ReannounceInterval int32 `json:"reannounceInterval,omitempty"`

	// reannounceMaxAttempts is how many times to reannounce.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ReannounceMaxAttempts int32 `json:"reannounceMaxAttempts,omitempty"`

	// webhookType is the content type the webhook sends, such as JSON.
	// +optional
	// +kubebuilder:validation:MinLength=1
	WebhookType string `json:"webhookType,omitempty"`

	// webhookMethod is the HTTP method the webhook uses.
	// +optional
	// +kubebuilder:validation:Enum=GET;POST;PUT;PATCH;DELETE
	WebhookMethod string `json:"webhookMethod,omitempty"`

	// webhookData is the body the webhook sends; Autobrr fills in its macros.
	// +optional
	// +kubebuilder:validation:MinLength=1
	WebhookData string `json:"webhookData,omitempty"`

	// webhookHeaders are headers the webhook sends, as Name=Value.
	// +optional
	// +listType=set
	WebhookHeaders []string `json:"webhookHeaders,omitempty"`

	// externalDownloadClient is the download client, by name in Sonarr, Radarr, Lidarr or Readarr,
	// that the app sends the release to.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExternalDownloadClient string `json:"externalDownloadClient,omitempty"`
}

// AutobrrStatus defines the observed state of Autobrr.
// +kubebuilder:validation:MinProperties=1
type AutobrrStatus struct {
	// conditions represent the current state of the Autobrr resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastSyncTime is the timestamp of the last successful reconciliation.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// managedResources records the resources this operator created, keyed by
	// resource type. Prune only removes entries listed here, so resources
	// created outside the operator are never deleted.
	// +optional
	// +listType=map
	// +listMapKey=type
	ManagedResources []commonv1alpha1.ManagedResource `json:"managedResources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=arrchitect
// +kubebuilder:printcolumn:name="Synced",type="string",JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Last Sync",type="date",JSONPath=`.status.lastSyncTime`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// Autobrr configures an Autobrr instance the operator connects to.
type Autobrr struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Autobrr.
	// +required
	Spec AutobrrSpec `json:"spec,omitzero"`

	// status is the observed state of the Autobrr.
	// +optional
	Status AutobrrStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AutobrrList contains a list of Autobrr.
type AutobrrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Autobrr `json:"items"`
}

func (c *Autobrr) GetConditions() *[]metav1.Condition                  { return &c.Status.Conditions }
func (c *Autobrr) GetObservedGeneration() *int64                       { return &c.Status.ObservedGeneration }
func (c *Autobrr) GetLastSyncTime() **metav1.Time                      { return &c.Status.LastSyncTime }
func (c *Autobrr) GetReconcileConfig() *commonv1alpha1.ReconcileConfig { return c.Spec.Reconcile }
func (c *Autobrr) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Autobrr) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
