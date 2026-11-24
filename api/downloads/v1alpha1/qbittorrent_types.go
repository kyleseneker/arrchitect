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

// QBittorrentSpec defines the desired state of QBittorrent.
type QBittorrentSpec struct {
	// connection defines how to connect to the qBittorrent instance.
	// +required
	Connection QBittorrentConnection `json:"connection,omitzero"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// preferences configures qBittorrent application preferences.
	// +optional
	Preferences *QBittorrentPreferences `json:"preferences,omitempty"`

	// categories defines download categories and their save paths.
	// +optional
	// +listType=map
	// +listMapKey=name
	Categories []QBittorrentCategory `json:"categories,omitempty"`

	// rssFeeds are the RSS feeds qBittorrent reads. Folders a feed's name puts
	// it in are created. Turn on preferences.rssProcessingEnabled for
	// qBittorrent to fetch them.
	// +optional
	// +listType=map
	// +listMapKey=name
	RSSFeeds []QBittorrentRSSFeed `json:"rssFeeds,omitempty"`

	// rssRules are qBittorrent's RSS download rules. Turn on
	// preferences.rssAutoDownloadingEnabled for qBittorrent to apply them.
	// +optional
	// +listType=map
	// +listMapKey=name
	RSSRules []QBittorrentRSSRule `json:"rssRules,omitempty"`
}

// QBittorrentConnection defines how the operator connects to qBittorrent.
// Unlike AppConnection, qBittorrent uses username/password authentication.
type QBittorrentConnection struct {
	// url is the base URL of the qBittorrent Web UI (e.g., http://qbittorrent.media.svc.cluster.local:8080).
	// +required
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url,omitempty"`

	// usernameSecretRef references a Secret containing the username.
	// +required
	UsernameSecretRef commonv1alpha1.SecretKeyRef `json:"usernameSecretRef,omitempty"`

	// passwordSecretRef references a Secret containing the password.
	// +required
	PasswordSecretRef commonv1alpha1.SecretKeyRef `json:"passwordSecretRef,omitempty"`

	// tls configures TLS settings for the connection.
	// Only needed when qBittorrent uses HTTPS with self-signed or private CA certificates.
	// +optional
	TLS *commonv1alpha1.TLSConfig `json:"tls,omitempty"`
}

// QBittorrentPreferences configures qBittorrent application preferences.
type QBittorrentPreferences struct {
	// savePath is the default save path for downloads.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SavePath string `json:"savePath,omitempty"`

	// tempPathEnabled enables using a temporary path for incomplete downloads.
	// +optional
	TempPathEnabled *bool `json:"tempPathEnabled,omitempty"`

	// dht enables the DHT (Distributed Hash Table) network.
	// +optional
	DHT *bool `json:"dht,omitempty"`

	// pex enables Peer Exchange.
	// +optional
	PEX *bool `json:"pex,omitempty"`

	// lsd enables Local Service Discovery.
	// +optional
	LSD *bool `json:"lsd,omitempty"`

	// encryption controls whether peers may, must or must not encrypt connections.
	// +optional
	// +kubebuilder:validation:Enum=allow;require;disable
	Encryption string `json:"encryption,omitempty"`

	// maxConnec is the global maximum number of connections. -1 means unlimited.
	// +optional
	MaxConnec *int32 `json:"maxConnec,omitempty"`

	// maxConnecPerTorrent is the maximum connections per torrent. -1 means unlimited.
	// +optional
	MaxConnecPerTorrent *int32 `json:"maxConnecPerTorrent,omitempty"`

	// maxUploads is the global maximum number of upload slots. -1 means unlimited.
	// +optional
	MaxUploads *int32 `json:"maxUploads,omitempty"`

	// maxUploadsPerTorrent is the maximum upload slots per torrent. -1 means unlimited.
	// +optional
	MaxUploadsPerTorrent *int32 `json:"maxUploadsPerTorrent,omitempty"`

	// maxRatioEnabled enables the global share ratio limit.
	// +optional
	MaxRatioEnabled *bool `json:"maxRatioEnabled,omitempty"`

	// maxRatio is the global share ratio limit (e.g., "2.0").
	// Represented as a string to preserve decimal precision in JSON. It applies
	// while maxRatioEnabled is true, here or in qBittorrent.
	// +optional
	MaxRatio *string `json:"maxRatio,omitempty"`

	// maxRatioAction is what qBittorrent does when a share limit is reached.
	// +optional
	// +kubebuilder:validation:Enum=stop;remove;enableSuperSeeding;removeWithContent
	MaxRatioAction string `json:"maxRatioAction,omitempty"`

	// maxSeedingTimeEnabled enables the global seeding time limit.
	// +optional
	MaxSeedingTimeEnabled *bool `json:"maxSeedingTimeEnabled,omitempty"`

	// maxSeedingTime is the global seeding time limit in minutes. It applies
	// while maxSeedingTimeEnabled is true, here or in qBittorrent.
	// +optional
	MaxSeedingTime *int32 `json:"maxSeedingTime,omitempty"`

	// preallocateAll enables disk space pre-allocation for all files.
	// +optional
	PreallocateAll *bool `json:"preallocateAll,omitempty"`

	// locale is the UI locale (e.g., "en").
	// +optional
	// +kubebuilder:validation:MinLength=1
	Locale string `json:"locale,omitempty"`

	// webUiPort is the Web UI listening port.
	// +optional
	WebUIPort *int32 `json:"webUiPort,omitempty"`

	// bypassAuthSubnetWhitelistEnabled lets clients in the whitelisted subnets skip
	// the WebUI login, so an authenticating proxy does not produce a second prompt.
	// +optional
	BypassAuthSubnetWhitelistEnabled *bool `json:"bypassAuthSubnetWhitelistEnabled,omitempty"`

	// bypassAuthSubnetWhitelist is the comma-separated list of subnets allowed to
	// skip the WebUI login.
	// +optional
	// +kubebuilder:validation:MinLength=1
	BypassAuthSubnetWhitelist string `json:"bypassAuthSubnetWhitelist,omitempty"`

	// bypassLocalAuth lets clients on localhost skip the WebUI login, such as a
	// VPN sidecar that sets listenPort to the port the VPN forwards.
	// +optional
	BypassLocalAuth *bool `json:"bypassLocalAuth,omitempty"`

	// webUiHostHeaderValidationEnabled has the Web UI refuse requests whose
	// Host header names another port than its own, or a host not in
	// webUiDomainList. List the Service name the operator connects through
	// there as well, and give the Service the Web UI's port, or the operator's
	// requests are refused too.
	// +optional
	WebUIHostHeaderValidationEnabled *bool `json:"webUiHostHeaderValidationEnabled,omitempty"`

	// webUiDomainList are the host names the Web UI accepts when Host header
	// validation is on; * accepts any.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	WebUIDomainList []string `json:"webUiDomainList,omitempty"`

	// webUiCsrfProtectionEnabled has the Web UI refuse requests made from
	// another site.
	// +optional
	WebUICsrfProtectionEnabled *bool `json:"webUiCsrfProtectionEnabled,omitempty"`

	// webUiClickjackingProtectionEnabled has the Web UI refuse to be shown in a
	// frame on another site.
	// +optional
	WebUIClickjackingProtectionEnabled *bool `json:"webUiClickjackingProtectionEnabled,omitempty"`

	// webUiSecureCookieEnabled marks the Web UI's session cookie Secure, so
	// browsers only send it over HTTPS.
	// +optional
	WebUISecureCookieEnabled *bool `json:"webUiSecureCookieEnabled,omitempty"`

	// webUiReverseProxyEnabled has the Web UI take client addresses from the
	// proxies in webUiReverseProxiesList, such as an ingress controller.
	// +optional
	WebUIReverseProxyEnabled *bool `json:"webUiReverseProxyEnabled,omitempty"`

	// webUiReverseProxiesList are the addresses or subnets of the proxies the
	// Web UI trusts when webUiReverseProxyEnabled is on.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	WebUIReverseProxiesList []string `json:"webUiReverseProxiesList,omitempty"`

	// tempPath is where incomplete downloads go while tempPathEnabled is on.
	// +optional
	// +kubebuilder:validation:MinLength=1
	TempPath string `json:"tempPath,omitempty"`

	// incompleteFilesExt adds .!qB to the names of files still downloading.
	// +optional
	IncompleteFilesExt *bool `json:"incompleteFilesExt,omitempty"`

	// autoTmmEnabled puts new torrents in automatic management, which saves
	// them under their category's path.
	// +optional
	AutoTMMEnabled *bool `json:"autoTmmEnabled,omitempty"`

	// torrentChangedTmmEnabled moves a torrent in automatic management when its
	// category changes. When off, the torrent switches to manual management.
	// +optional
	TorrentChangedTMMEnabled *bool `json:"torrentChangedTmmEnabled,omitempty"`

	// savePathChangedTmmEnabled moves torrents in automatic management when
	// savePath changes. When off, they switch to manual management.
	// +optional
	SavePathChangedTMMEnabled *bool `json:"savePathChangedTmmEnabled,omitempty"`

	// categoryChangedTmmEnabled moves a category's torrents in automatic
	// management when its save path changes. When off, they switch to manual
	// management.
	// +optional
	CategoryChangedTMMEnabled *bool `json:"categoryChangedTmmEnabled,omitempty"`

	// torrentContentLayout decides whether a torrent's files go in a folder of
	// their own.
	// +optional
	// +kubebuilder:validation:Enum=Original;Subfolder;NoSubfolder
	TorrentContentLayout string `json:"torrentContentLayout,omitempty"`

	// addStoppedEnabled adds new torrents stopped.
	// +optional
	AddStoppedEnabled *bool `json:"addStoppedEnabled,omitempty"`

	// torrentStopCondition stops a new torrent once it gets this far.
	// +optional
	// +kubebuilder:validation:Enum=None;MetadataReceived;FilesChecked
	TorrentStopCondition string `json:"torrentStopCondition,omitempty"`

	// excludedFileNamesEnabled skips files whose names match excludedFileNames.
	// +optional
	ExcludedFileNamesEnabled *bool `json:"excludedFileNamesEnabled,omitempty"`

	// excludedFileNames are file name patterns to skip, such as *.lnk.
	// +optional
	// +listType=set
	ExcludedFileNames []string `json:"excludedFileNames,omitempty"`

	// dlLimit is the global download speed limit in KiB/s. 0 means unlimited.
	// +optional
	// +kubebuilder:validation:Minimum=0
	DlLimit *int32 `json:"dlLimit,omitempty"`

	// upLimit is the global upload speed limit in KiB/s. 0 means unlimited.
	// +optional
	// +kubebuilder:validation:Minimum=0
	UpLimit *int32 `json:"upLimit,omitempty"`

	// altDlLimit is the download speed limit in KiB/s while the alternative
	// limits are on. 0 means unlimited.
	// +optional
	// +kubebuilder:validation:Minimum=0
	AltDlLimit *int32 `json:"altDlLimit,omitempty"`

	// altUpLimit is the upload speed limit in KiB/s while the alternative limits
	// are on. 0 means unlimited.
	// +optional
	// +kubebuilder:validation:Minimum=0
	AltUpLimit *int32 `json:"altUpLimit,omitempty"`

	// schedulerEnabled turns the alternative limits on and off on a schedule.
	// +optional
	SchedulerEnabled *bool `json:"schedulerEnabled,omitempty"`

	// scheduleFromHour is the hour the alternative limits start.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	ScheduleFromHour *int32 `json:"scheduleFromHour,omitempty"`

	// scheduleFromMin is the minute the alternative limits start.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=59
	ScheduleFromMin *int32 `json:"scheduleFromMin,omitempty"`

	// scheduleToHour is the hour the alternative limits end.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	ScheduleToHour *int32 `json:"scheduleToHour,omitempty"`

	// scheduleToMin is the minute the alternative limits end.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=59
	ScheduleToMin *int32 `json:"scheduleToMin,omitempty"`

	// schedulerDays is the days the schedule runs.
	// +optional
	// +kubebuilder:validation:Enum=everyDay;weekdays;weekends;monday;tuesday;wednesday;thursday;friday;saturday;sunday
	SchedulerDays string `json:"schedulerDays,omitempty"`

	// limitUtpRate applies the speed limits to µTP connections.
	// +optional
	LimitUTPRate *bool `json:"limitUtpRate,omitempty"`

	// limitTcpOverhead counts transport overhead against the speed limits.
	// +optional
	LimitTCPOverhead *bool `json:"limitTcpOverhead,omitempty"`

	// limitLanPeers applies the speed limits to peers on the local network.
	// +optional
	LimitLANPeers *bool `json:"limitLanPeers,omitempty"`

	// queueingEnabled limits how many torrents are active at once.
	// +optional
	QueueingEnabled *bool `json:"queueingEnabled,omitempty"`

	// maxActiveDownloads is how many torrents can download at once. -1 means unlimited.
	// +optional
	MaxActiveDownloads *int32 `json:"maxActiveDownloads,omitempty"`

	// maxActiveUploads is how many torrents can seed at once. -1 means unlimited.
	// +optional
	MaxActiveUploads *int32 `json:"maxActiveUploads,omitempty"`

	// maxActiveTorrents is how many torrents can be active at once. -1 means unlimited.
	// +optional
	MaxActiveTorrents *int32 `json:"maxActiveTorrents,omitempty"`

	// dontCountSlowTorrents leaves slow torrents out of the active limits.
	// +optional
	DontCountSlowTorrents *bool `json:"dontCountSlowTorrents,omitempty"`

	// slowTorrentDlRateThreshold is the download speed in KiB/s below which a
	// torrent counts as slow.
	// +optional
	// +kubebuilder:validation:Minimum=0
	SlowTorrentDlRateThreshold *int32 `json:"slowTorrentDlRateThreshold,omitempty"`

	// slowTorrentUlRateThreshold is the upload speed in KiB/s below which a
	// torrent counts as slow.
	// +optional
	// +kubebuilder:validation:Minimum=0
	SlowTorrentUlRateThreshold *int32 `json:"slowTorrentUlRateThreshold,omitempty"`

	// slowTorrentInactiveTimer is how many seconds a torrent stays under the
	// thresholds before it counts as slow.
	// +optional
	// +kubebuilder:validation:Minimum=0
	SlowTorrentInactiveTimer *int32 `json:"slowTorrentInactiveTimer,omitempty"`

	// listenPort is the port for incoming connections.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ListenPort int32 `json:"listenPort,omitempty"`

	// randomPort picks a different listening port on each start.
	// +optional
	RandomPort *bool `json:"randomPort,omitempty"`

	// upnp forwards the listening port on the router with UPnP or NAT-PMP.
	// +optional
	UPnP *bool `json:"upnp,omitempty"`

	// networkInterface binds connections to one network interface, such as a
	// VPN's tun0. Empty means any interface.
	// +optional
	NetworkInterface *string `json:"networkInterface,omitempty"`

	// networkInterfaceAddress binds connections to one address of
	// networkInterface. Empty means all of them.
	// +optional
	NetworkInterfaceAddress *string `json:"networkInterfaceAddress,omitempty"`

	// anonymousMode hides the client's identity from peers and trackers.
	// +optional
	AnonymousMode *bool `json:"anonymousMode,omitempty"`

	// maxInactiveSeedingTimeEnabled enables the global limit on seeding time
	// without uploads.
	// +optional
	MaxInactiveSeedingTimeEnabled *bool `json:"maxInactiveSeedingTimeEnabled,omitempty"`

	// maxInactiveSeedingTime is the global inactive seeding time limit in
	// minutes. It applies while maxInactiveSeedingTimeEnabled is true, here or
	// in qBittorrent.
	// +optional
	MaxInactiveSeedingTime *int32 `json:"maxInactiveSeedingTime,omitempty"`

	// proxyType is the kind of proxy qBittorrent connects through. None turns
	// the proxy off.
	// +optional
	// +kubebuilder:validation:Enum=None;HTTP;SOCKS5;SOCKS4
	ProxyType string `json:"proxyType,omitempty"`

	// proxyIp is the proxy's host name or address.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ProxyIP string `json:"proxyIp,omitempty"`

	// proxyPort is the proxy's port.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ProxyPort int32 `json:"proxyPort,omitempty"`

	// proxyAuthEnabled signs in to the proxy with proxyUsernameSecretRef and
	// proxyPasswordSecretRef. SOCKS4 has no sign-in.
	// +optional
	ProxyAuthEnabled *bool `json:"proxyAuthEnabled,omitempty"`

	// proxyUsernameSecretRef is the Secret key holding the proxy's user name.
	// +optional
	ProxyUsernameSecretRef *commonv1alpha1.SecretKeyRef `json:"proxyUsernameSecretRef,omitempty"`

	// proxyPasswordSecretRef is the Secret key holding the proxy's password.
	// +optional
	ProxyPasswordSecretRef *commonv1alpha1.SecretKeyRef `json:"proxyPasswordSecretRef,omitempty"`

	// proxyHostnameLookup has the proxy, rather than qBittorrent, resolve host
	// names.
	// +optional
	ProxyHostnameLookup *bool `json:"proxyHostnameLookup,omitempty"`

	// proxyBittorrent sends BitTorrent traffic through the proxy.
	// +optional
	ProxyBittorrent *bool `json:"proxyBittorrent,omitempty"`

	// proxyPeerConnections sends connections to peers, not only to trackers,
	// through the proxy.
	// +optional
	ProxyPeerConnections *bool `json:"proxyPeerConnections,omitempty"`

	// proxyRss sends RSS feed requests through the proxy.
	// +optional
	ProxyRSS *bool `json:"proxyRss,omitempty"`

	// proxyMisc sends qBittorrent's other requests, such as update checks and
	// tracker lists, through the proxy.
	// +optional
	ProxyMisc *bool `json:"proxyMisc,omitempty"`

	// rssProcessingEnabled has qBittorrent fetch its RSS feeds.
	// +optional
	RSSProcessingEnabled *bool `json:"rssProcessingEnabled,omitempty"`

	// rssRefreshInterval is how often, in minutes, qBittorrent fetches each
	// feed.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=999999
	RSSRefreshInterval int32 `json:"rssRefreshInterval,omitempty"`

	// rssFetchDelay is the delay, in seconds, between two requests to the same
	// host.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2147483646
	RSSFetchDelay *int32 `json:"rssFetchDelay,omitempty"`

	// rssMaxArticlesPerFeed is how many articles qBittorrent keeps per feed.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2147483646
	RSSMaxArticlesPerFeed *int32 `json:"rssMaxArticlesPerFeed,omitempty"`

	// rssAutoDownloadingEnabled has qBittorrent download what its RSS rules
	// match.
	// +optional
	RSSAutoDownloadingEnabled *bool `json:"rssAutoDownloadingEnabled,omitempty"`

	// rssDownloadRepackProperEpisodes downloads REPACK and PROPER releases of
	// episodes a rule with the smart filter already matched.
	// +optional
	RSSDownloadRepackProperEpisodes *bool `json:"rssDownloadRepackProperEpisodes,omitempty"`

	// rssSmartEpisodeFilters are the regular expressions the smart filter finds
	// season and episode numbers with.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:items:MinLength=1
	RSSSmartEpisodeFilters []string `json:"rssSmartEpisodeFilters,omitempty"`

	// ipFilterEnabled blocks the peers ipFilterPath lists.
	// +optional
	IPFilterEnabled *bool `json:"ipFilterEnabled,omitempty"`

	// ipFilterPath is the blocklist file, in .dat, .p2p or .p2b format, as
	// qBittorrent sees it.
	// +optional
	// +kubebuilder:validation:MinLength=1
	IPFilterPath string `json:"ipFilterPath,omitempty"`

	// ipFilterTrackers applies the IP filter to trackers as well as peers.
	// +optional
	IPFilterTrackers *bool `json:"ipFilterTrackers,omitempty"`

	// bannedIps are peer addresses qBittorrent refuses, IPv4 or IPv6.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1000
	// +kubebuilder:validation:items:MaxLength=45
	// +kubebuilder:validation:XValidation:rule="self.all(a, isIP(a))",message="bannedIps holds IP addresses"
	BannedIPs []string `json:"bannedIps,omitempty"`

	// autorunOnTorrentAddedEnabled runs autorunOnTorrentAddedProgram when a
	// torrent is added.
	// +optional
	AutorunOnTorrentAddedEnabled *bool `json:"autorunOnTorrentAddedEnabled,omitempty"`

	// autorunOnTorrentAddedProgram is the command run when a torrent is added,
	// with qBittorrent's parameters such as %N for its name and %I for its hash.
	// +optional
	// +kubebuilder:validation:MinLength=1
	AutorunOnTorrentAddedProgram string `json:"autorunOnTorrentAddedProgram,omitempty"`

	// autorunEnabled runs autorunProgram when a torrent finishes.
	// +optional
	AutorunEnabled *bool `json:"autorunEnabled,omitempty"`

	// autorunProgram is the command run when a torrent finishes, with the same
	// parameters as autorunOnTorrentAddedProgram.
	// +optional
	// +kubebuilder:validation:MinLength=1
	AutorunProgram string `json:"autorunProgram,omitempty"`

	// addTrackersEnabled adds addTrackers to every new torrent.
	// +optional
	AddTrackersEnabled *bool `json:"addTrackersEnabled,omitempty"`

	// addTrackers are tracker URLs added to new torrents, each in its own tier.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:Pattern=`^\S+$`
	AddTrackers []string `json:"addTrackers,omitempty"`

	// addTrackersFromUrlEnabled adds the trackers listed at addTrackersUrl to
	// every new torrent.
	// +optional
	AddTrackersFromURLEnabled *bool `json:"addTrackersFromUrlEnabled,omitempty"`

	// addTrackersUrl is a tracker list qBittorrent downloads, one tracker a
	// line.
	// +optional
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	AddTrackersURL string `json:"addTrackersUrl,omitempty"`
}

// QBittorrentCategory defines a download category with its save path.
type QBittorrentCategory struct {
	// name is the category name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// savePath is the save path for this category.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SavePath string `json:"savePath,omitempty"`
}

// QBittorrentRSSFeed is an RSS feed qBittorrent reads.
type QBittorrentRSSFeed struct {
	// name is the feed's path in qBittorrent's RSS list, with a backslash
	// between folders, such as Anime\SubsPlease.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[^\\]+(\\[^\\]+)*$`
	Name string `json:"name,omitempty"`

	// url is the feed's address.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^https?://`
	URL string `json:"url,omitempty"`
}

// QBittorrentRSSRule is an RSS download rule: qBittorrent downloads what its
// feeds publish that the rule matches.
type QBittorrentRSSRule struct {
	// name is the rule's name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enabled turns the rule on.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// priority orders the rules: higher goes first.
	// +optional
	Priority *int32 `json:"priority,omitempty"`

	// affectedFeeds are the URLs of the feeds the rule applies to.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:Pattern=`^https?://`
	AffectedFeeds []string `json:"affectedFeeds,omitempty"`

	// mustContain is what an article's title must contain: words, wildcards
	// and | for alternatives, or a regular expression with useRegex.
	// +optional
	MustContain *string `json:"mustContain,omitempty"`

	// mustNotContain is what an article's title must not contain.
	// +optional
	MustNotContain *string `json:"mustNotContain,omitempty"`

	// useRegex reads mustContain and mustNotContain as regular expressions.
	// +optional
	UseRegex *bool `json:"useRegex,omitempty"`

	// episodeFilter limits matches to seasons and episodes, such as 1x2;8-15;
	// or 2x25-;.
	// +optional
	EpisodeFilter *string `json:"episodeFilter,omitempty"`

	// smartFilter downloads each episode once, using the smart episode filters.
	// +optional
	SmartFilter *bool `json:"smartFilter,omitempty"`

	// ignoreDays ignores further matches for this many days after one; 0
	// disables it.
	// +optional
	// +kubebuilder:validation:Minimum=0
	IgnoreDays *int32 `json:"ignoreDays,omitempty"`

	// torrentParams sets how the torrents the rule adds are added.
	// +optional
	TorrentParams QBittorrentRSSTorrentParams `json:"torrentParams,omitzero"`
}

// QBittorrentRSSTorrentParams sets how an RSS rule adds torrents. Left out,
// qBittorrent's defaults apply.
// +kubebuilder:validation:MinProperties=1
type QBittorrentRSSTorrentParams struct {
	// category is the category the torrents get.
	// +optional
	Category *string `json:"category,omitempty"`

	// tags are the tags the torrents get.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	Tags []string `json:"tags,omitempty"`

	// savePath is where the torrents are saved, in place of the category's or
	// the default path.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SavePath string `json:"savePath,omitempty"`

	// useAutoTMM has automatic torrent management choose the torrents' paths
	// from their category.
	// +optional
	UseAutoTMM *bool `json:"useAutoTMM,omitempty"`

	// stopped adds the torrents stopped.
	// +optional
	Stopped *bool `json:"stopped,omitempty"`

	// addToTopOfQueue puts the torrents first in the queue.
	// +optional
	AddToTopOfQueue *bool `json:"addToTopOfQueue,omitempty"`

	// contentLayout arranges the torrents' files: Original, Subfolder, or
	// NoSubfolder.
	// +optional
	// +kubebuilder:validation:Enum=Original;Subfolder;NoSubfolder
	ContentLayout string `json:"contentLayout,omitempty"`
}

// QBittorrentStatus defines the observed state of QBittorrent.
// +kubebuilder:validation:MinProperties=1
type QBittorrentStatus struct {
	// conditions represent the current state of the QBittorrent resource.
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

// QBittorrent configures a QBittorrent instance the operator connects to.
type QBittorrent struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the QBittorrent.
	// +required
	Spec QBittorrentSpec `json:"spec,omitzero"`

	// status is the observed state of the QBittorrent.
	// +optional
	Status QBittorrentStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// QBittorrentList contains a list of QBittorrent.
type QBittorrentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []QBittorrent `json:"items"`
}

func (c *QBittorrent) GetConditions() *[]metav1.Condition { return &c.Status.Conditions }
func (c *QBittorrent) GetObservedGeneration() *int64      { return &c.Status.ObservedGeneration }
func (c *QBittorrent) GetLastSyncTime() **metav1.Time     { return &c.Status.LastSyncTime }
func (c *QBittorrent) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return c.Spec.Reconcile
}
func (c *QBittorrent) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *QBittorrent) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
