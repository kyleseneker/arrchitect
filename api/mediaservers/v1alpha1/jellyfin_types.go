package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JellyfinSpec defines the desired state of Jellyfin.
// +kubebuilder:validation:XValidation:rule="!has(self.reconcile) || !has(self.reconcile.prune) || !self.reconcile.prune",message="Jellyfin doesn't support prune"
// +kubebuilder:validation:XValidation:rule="!has(self.reconcile) || !has(self.reconcile.deletionPolicy) || self.reconcile.deletionPolicy != 'delete'",message="Jellyfin doesn't support deletionPolicy delete"
type JellyfinSpec struct {
	// connection defines how to connect to the Jellyfin instance.
	// +required
	Connection JellyfinConnection `json:"connection,omitzero"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// adminUser defines the admin credentials used for initial setup and API access.
	// +required
	AdminUser JellyfinAdminUser `json:"adminUser,omitzero"`

	// server configures Jellyfin server settings.
	// +optional
	Server *JellyfinServer `json:"server,omitempty"`

	// encoding configures hardware-accelerated transcoding settings.
	// +optional
	Encoding *JellyfinEncoding `json:"encoding,omitempty"`

	// network configures how Jellyfin tells local clients from remote ones and
	// which proxies it trusts.
	// +optional
	Network *JellyfinNetwork `json:"network,omitempty"`

	// branding configures the login page and the web client's look.
	// +optional
	Branding *JellyfinBranding `json:"branding,omitempty"`

	// scheduledTasks sets when Jellyfin runs its scheduled tasks. A listed
	// task's triggers replace the ones it has; tasks left out keep theirs.
	// +optional
	// +listType=map
	// +listMapKey=key
	ScheduledTasks []JellyfinScheduledTask `json:"scheduledTasks,omitempty"`

	// libraries defines media libraries, matched by name. An existing library
	// keeps the declared folders and options: folders it doesn't list are
	// removed. Libraries are never deleted, and a library's type can't change
	// in place.
	// +optional
	// +listType=map
	// +listMapKey=name
	Libraries []JellyfinLibrary `json:"libraries,omitempty"`
}

// JellyfinConnection defines how the operator connects to Jellyfin.
type JellyfinConnection struct {
	// url is the base URL of the Jellyfin instance (e.g., http://jellyfin.media.svc.cluster.local:8096).
	// +required
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url,omitempty"`

	// apiKeySecretRef references a Secret containing a Jellyfin API key, from
	// the API Keys page of Jellyfin's dashboard. Once Jellyfin is set up, the
	// operator uses it instead of signing in as adminUser, and an appRef to this
	// resource passes it to apps that connect with a key, such as Maintainerr.
	// +optional
	APIKeySecretRef *commonv1alpha1.SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// tls configures TLS settings for the connection.
	// Only needed when Jellyfin uses HTTPS with self-signed or private CA certificates.
	// +optional
	TLS *commonv1alpha1.TLSConfig `json:"tls,omitempty"`
}

// JellyfinAdminUser defines the admin user credentials for Jellyfin.
type JellyfinAdminUser struct {
	// usernameSecretRef references a Secret containing the admin username.
	// +required
	UsernameSecretRef commonv1alpha1.SecretKeyRef `json:"usernameSecretRef,omitempty"`

	// passwordSecretRef references a Secret containing the admin password.
	// +required
	PasswordSecretRef commonv1alpha1.SecretKeyRef `json:"passwordSecretRef,omitempty"`
}

// JellyfinServer configures Jellyfin server settings.
type JellyfinServer struct {
	// serverName is the friendly name of the server.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ServerName string `json:"serverName,omitempty"`

	// preferredMetadataLanguage is the preferred language for metadata (e.g., "en").
	// +optional
	// +kubebuilder:validation:MinLength=1
	PreferredMetadataLanguage string `json:"preferredMetadataLanguage,omitempty"`

	// metadataCountryCode is the country code for metadata lookups (e.g., "US").
	// +optional
	// +kubebuilder:validation:MinLength=1
	MetadataCountryCode string `json:"metadataCountryCode,omitempty"`

	// logFileRetentionDays is the number of days to retain log files.
	// +optional
	LogFileRetentionDays *int32 `json:"logFileRetentionDays,omitempty"`

	// enableGroupingMoviesIntoCollections enables automatic grouping of movies into collections.
	// +optional
	EnableGroupingMoviesIntoCollections *bool `json:"enableGroupingMoviesIntoCollections,omitempty"`

	// displaySpecialsWithinSeasons displays special episodes within their respective seasons.
	// +optional
	DisplaySpecialsWithinSeasons *bool `json:"displaySpecialsWithinSeasons,omitempty"`

	// libraryMonitorDelay is the delay in seconds before processing library changes.
	// +optional
	LibraryMonitorDelay *int32 `json:"libraryMonitorDelay,omitempty"`

	// enableMetrics serves Prometheus metrics at /metrics.
	// +optional
	EnableMetrics *bool `json:"enableMetrics,omitempty"`

	// quickConnectAvailable lets users sign in on a new device with a code.
	// +optional
	QuickConnectAvailable *bool `json:"quickConnectAvailable,omitempty"`

	// remoteClientBitrateLimit caps the bitrate of each remote stream, in bits
	// per second. 0 means no limit.
	// +optional
	// +kubebuilder:validation:Minimum=0
	RemoteClientBitrateLimit *int32 `json:"remoteClientBitrateLimit,omitempty"`

	// enableGroupingShowsIntoCollections shows series in collections instead of
	// on their own.
	// +optional
	EnableGroupingShowsIntoCollections *bool `json:"enableGroupingShowsIntoCollections,omitempty"`

	// enableFolderView shows a Folders view of the media folders.
	// +optional
	EnableFolderView *bool `json:"enableFolderView,omitempty"`

	// enableExternalContentInSuggestions includes internet trailers and live TV
	// in suggestions.
	// +optional
	EnableExternalContentInSuggestions *bool `json:"enableExternalContentInSuggestions,omitempty"`

	// minResumePct is how far, in percent, playback must get before it can be resumed.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	MinResumePct *int32 `json:"minResumePct,omitempty"`

	// maxResumePct is how far, in percent, playback must get to count as played.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	MaxResumePct *int32 `json:"maxResumePct,omitempty"`

	// minResumeDurationSeconds is the shortest item, in seconds, that can be resumed.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinResumeDurationSeconds *int32 `json:"minResumeDurationSeconds,omitempty"`

	// minAudiobookResume is how many minutes an audiobook must play before it
	// can be resumed.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinAudiobookResume *int32 `json:"minAudiobookResume,omitempty"`

	// maxAudiobookResume is how many minutes from the end an audiobook counts as played.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxAudiobookResume *int32 `json:"maxAudiobookResume,omitempty"`

	// activityLogRetentionDays is how many days the activity log keeps entries.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ActivityLogRetentionDays *int32 `json:"activityLogRetentionDays,omitempty"`

	// libraryScanFanoutConcurrency is how many library scan tasks run at once.
	// 0 picks one from the CPU count.
	// +optional
	// +kubebuilder:validation:Minimum=0
	LibraryScanFanoutConcurrency *int32 `json:"libraryScanFanoutConcurrency,omitempty"`

	// libraryMetadataRefreshConcurrency is how many metadata refreshes run at
	// once. 0 picks one from the CPU count.
	// +optional
	// +kubebuilder:validation:Minimum=0
	LibraryMetadataRefreshConcurrency *int32 `json:"libraryMetadataRefreshConcurrency,omitempty"`

	// parallelImageEncodingLimit is how many images are encoded at once. 0
	// picks one from the CPU count.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ParallelImageEncodingLimit *int32 `json:"parallelImageEncodingLimit,omitempty"`

	// enableSlowResponseWarning logs requests slower than slowResponseThresholdMs.
	// +optional
	EnableSlowResponseWarning *bool `json:"enableSlowResponseWarning,omitempty"`

	// slowResponseThresholdMs is how many milliseconds a request takes before
	// it's logged as slow.
	// +optional
	// +kubebuilder:validation:Minimum=0
	SlowResponseThresholdMs *int32 `json:"slowResponseThresholdMs,omitempty"`

	// imageSavingConvention is how Jellyfin names the artwork it saves with media.
	// +optional
	// +kubebuilder:validation:Enum=Legacy;Compatible
	ImageSavingConvention string `json:"imageSavingConvention,omitempty"`

	// trickplayOptions configures the preview thumbnails shown while seeking.
	// +optional
	TrickplayOptions *JellyfinTrickplay `json:"trickplayOptions,omitempty"`
}

// JellyfinTrickplay configures how Jellyfin makes trickplay thumbnails.
type JellyfinTrickplay struct {
	// enableHwAcceleration decodes video in hardware while making thumbnails.
	// +optional
	EnableHwAcceleration *bool `json:"enableHwAcceleration,omitempty"`

	// enableHwEncoding encodes thumbnails in hardware.
	// +optional
	EnableHwEncoding *bool `json:"enableHwEncoding,omitempty"`

	// enableKeyFrameOnlyExtraction takes thumbnails only at key frames, which is
	// faster and less precise.
	// +optional
	EnableKeyFrameOnlyExtraction *bool `json:"enableKeyFrameOnlyExtraction,omitempty"`

	// scanBehavior decides whether a library scan waits for thumbnails.
	// +optional
	// +kubebuilder:validation:Enum=Blocking;NonBlocking
	ScanBehavior string `json:"scanBehavior,omitempty"`

	// processPriority is the priority of the process making thumbnails.
	// +optional
	// +kubebuilder:validation:Enum=Idle;BelowNormal;Normal;AboveNormal;High
	ProcessPriority string `json:"processPriority,omitempty"`

	// interval is the time between thumbnails, in milliseconds.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Interval int32 `json:"interval,omitempty"`

	// widthResolutions are the thumbnail widths to make, in pixels.
	// +optional
	// +listType=set
	WidthResolutions []int32 `json:"widthResolutions,omitempty"`

	// tileWidth is how many thumbnails go across each tile image.
	// +optional
	// +kubebuilder:validation:Minimum=1
	TileWidth int32 `json:"tileWidth,omitempty"`

	// tileHeight is how many thumbnails go down each tile image.
	// +optional
	// +kubebuilder:validation:Minimum=1
	TileHeight int32 `json:"tileHeight,omitempty"`

	// qscale is the thumbnails' encoding quality, from 2 (best) to 31.
	// +optional
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=31
	Qscale int32 `json:"qscale,omitempty"`

	// jpegQuality is the JPEG quality of the tile images.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	JpegQuality int32 `json:"jpegQuality,omitempty"`

	// processThreads is how many threads making thumbnails uses.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ProcessThreads *int32 `json:"processThreads,omitempty"`
}

// JellyfinScheduledTask sets when one of Jellyfin's scheduled tasks runs.
type JellyfinScheduledTask struct {
	// key is the task's key as Jellyfin's /ScheduledTasks lists it, such as
	// RefreshLibrary (Scan Media Library) or RefreshChapterImages.
	// +required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key,omitempty"`

	// triggers are when the task runs. Left out or empty, the task runs only
	// when started by hand.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=16
	Triggers []JellyfinTaskTrigger `json:"triggers,omitempty"`
}

// JellyfinTaskTrigger is one time a scheduled task runs.
// +kubebuilder:validation:XValidation:rule="has(self.timeOfDay) == (self.type == 'DailyTrigger' || self.type == 'WeeklyTrigger')",message="timeOfDay goes with a DailyTrigger or WeeklyTrigger"
// +kubebuilder:validation:XValidation:rule="has(self.dayOfWeek) == (self.type == 'WeeklyTrigger')",message="dayOfWeek goes with a WeeklyTrigger"
// +kubebuilder:validation:XValidation:rule="has(self.intervalMinutes) == (self.type == 'IntervalTrigger')",message="intervalMinutes goes with an IntervalTrigger"
type JellyfinTaskTrigger struct {
	// type is DailyTrigger, WeeklyTrigger, IntervalTrigger, or StartupTrigger
	// to run when Jellyfin starts.
	// +required
	// +kubebuilder:validation:Enum=DailyTrigger;WeeklyTrigger;IntervalTrigger;StartupTrigger
	Type string `json:"type,omitempty"`

	// timeOfDay is when a daily or weekly trigger fires, as HH:MM in the
	// server's time zone.
	// +optional
	// +kubebuilder:validation:MinLength=5
	// +kubebuilder:validation:MaxLength=5
	// +kubebuilder:validation:Pattern=`^([01][0-9]|2[0-3]):[0-5][0-9]$`
	TimeOfDay string `json:"timeOfDay,omitempty"`

	// dayOfWeek is the day a weekly trigger fires.
	// +optional
	// +kubebuilder:validation:Enum=Sunday;Monday;Tuesday;Wednesday;Thursday;Friday;Saturday
	DayOfWeek string `json:"dayOfWeek,omitempty"`

	// intervalMinutes is how often an interval trigger fires.
	// +optional
	// +kubebuilder:validation:Minimum=1
	IntervalMinutes int32 `json:"intervalMinutes,omitempty"`

	// maxRuntimeHours stops the task after it has run this many hours.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxRuntimeHours int32 `json:"maxRuntimeHours,omitempty"`
}

// JellyfinBranding configures Jellyfin's branding. The splash screen image is
// uploaded in Jellyfin, so it stays out.
type JellyfinBranding struct {
	// loginDisclaimer is shown below the login form, and can hold HTML.
	// +optional
	LoginDisclaimer *string `json:"loginDisclaimer,omitempty"`

	// customCss is CSS the web client applies to every page.
	// +optional
	CustomCss *string `json:"customCss,omitempty"`

	// splashscreenEnabled shows the splash screen while the web client loads.
	// +optional
	SplashscreenEnabled *bool `json:"splashscreenEnabled,omitempty"`
}

// JellyfinNetwork configures Jellyfin's network settings. Ports, the base URL,
// HTTPS and bind addresses aren't here: changing them needs a restart and has
// to match the Service and connection.url.
type JellyfinNetwork struct {
	// enableRemoteAccess allows connections from outside localNetworkSubnets.
	// With it off, localNetworkSubnets must include the operator's pod network.
	// +optional
	EnableRemoteAccess *bool `json:"enableRemoteAccess,omitempty"`

	// localNetworkSubnets are the networks, in CIDR form, whose clients count as
	// local. Empty means the private address ranges.
	// +optional
	// +listType=set
	LocalNetworkSubnets []string `json:"localNetworkSubnets,omitempty"`

	// knownProxies are the reverse proxies, by address or host name, whose
	// forwarded headers Jellyfin trusts for the client's address.
	// +optional
	// +listType=set
	KnownProxies []string `json:"knownProxies,omitempty"`

	// autoDiscovery answers clients looking for servers on the local network.
	// +optional
	AutoDiscovery *bool `json:"autoDiscovery,omitempty"`

	// enableUPnP forwards Jellyfin's public ports on the router.
	// +optional
	EnableUPnP *bool `json:"enableUPnP,omitempty"`

	// ignoreVirtualInterfaces leaves the interfaces named in
	// virtualInterfaceNames out of auto-detection.
	// +optional
	IgnoreVirtualInterfaces *bool `json:"ignoreVirtualInterfaces,omitempty"`

	// virtualInterfaceNames are interface name prefixes, such as veth, that
	// ignoreVirtualInterfaces skips.
	// +optional
	// +listType=set
	VirtualInterfaceNames []string `json:"virtualInterfaceNames,omitempty"`

	// enablePublishedServerUriByRequest tells clients the address they reached
	// Jellyfin on, instead of one Jellyfin works out.
	// +optional
	EnablePublishedServerUriByRequest *bool `json:"enablePublishedServerUriByRequest,omitempty"`

	// publishedServerUriBySubnet overrides the address Jellyfin gives clients,
	// as subnet=URL entries, such as all=https://jellyfin.example.com.
	// +optional
	// +listType=set
	PublishedServerUriBySubnet []string `json:"publishedServerUriBySubnet,omitempty"`

	// remoteIPFilter lists remote addresses or subnets to allow, or to block when
	// isRemoteIPFilterBlacklist is on.
	// +optional
	// +listType=set
	RemoteIPFilter []string `json:"remoteIPFilter,omitempty"`

	// isRemoteIPFilterBlacklist blocks the addresses in remoteIPFilter instead
	// of allowing only them.
	// +optional
	IsRemoteIPFilterBlacklist *bool `json:"isRemoteIPFilterBlacklist,omitempty"`
}

// JellyfinEncoding configures hardware-accelerated transcoding settings.
type JellyfinEncoding struct {
	// hardwareAccelerationType is the hardware acceleration method to use.
	// +optional
	// +kubebuilder:validation:Enum=none;amf;qsv;nvenc;v4l2m2m;vaapi;videotoolbox;rkmpp
	HardwareAccelerationType string `json:"hardwareAccelerationType,omitempty"`

	// vaapiDevice is the VAAPI render device path (e.g., "/dev/dri/renderD128").
	// +optional
	// +kubebuilder:validation:MinLength=1
	VaapiDevice string `json:"vaapiDevice,omitempty"`

	// enableHardwareEncoding enables hardware-accelerated encoding.
	// +optional
	EnableHardwareEncoding *bool `json:"enableHardwareEncoding,omitempty"`

	// allowHevcEncoding allows HEVC/H.265 hardware encoding.
	// +optional
	AllowHevcEncoding *bool `json:"allowHevcEncoding,omitempty"`

	// enableTonemapping enables HDR to SDR tone mapping.
	// +optional
	EnableTonemapping *bool `json:"enableTonemapping,omitempty"`

	// enableVppTonemapping enables VPP-based tone mapping (Intel QSV).
	// +optional
	EnableVppTonemapping *bool `json:"enableVppTonemapping,omitempty"`

	// h264Crf is the CRF value for H.264 encoding.
	// +optional
	H264Crf *int32 `json:"h264Crf,omitempty"`

	// h265Crf is the CRF value for H.265/HEVC encoding.
	// +optional
	H265Crf *int32 `json:"h265Crf,omitempty"`

	// encoderPreset is the encoder speed preset (e.g., "auto", "fast", "medium", "slow").
	// +optional
	// +kubebuilder:validation:MinLength=1
	EncoderPreset string `json:"encoderPreset,omitempty"`

	// hardwareDecodingCodecs is the list of codecs to decode in hardware.
	// +optional
	// +listType=set
	HardwareDecodingCodecs []string `json:"hardwareDecodingCodecs,omitempty"`

	// enableIntelLowPowerH264HwEncoder enables the Intel low-power H.264 hardware encoder.
	// +optional
	EnableIntelLowPowerH264HwEncoder *bool `json:"enableIntelLowPowerH264HwEncoder,omitempty"`

	// enableIntelLowPowerHevcHwEncoder enables the Intel low-power HEVC hardware encoder.
	// +optional
	EnableIntelLowPowerHevcHwEncoder *bool `json:"enableIntelLowPowerHevcHwEncoder,omitempty"`

	// enableSubtitleExtraction enables extracting subtitles from media files.
	// +optional
	EnableSubtitleExtraction *bool `json:"enableSubtitleExtraction,omitempty"`

	// enableThrottling enables transcoding throttling.
	// +optional
	EnableThrottling *bool `json:"enableThrottling,omitempty"`

	// throttleDelaySeconds is the delay in seconds before throttling begins.
	// +optional
	ThrottleDelaySeconds *int32 `json:"throttleDelaySeconds,omitempty"`

	// qsvDevice is the Intel Quick Sync device, when there's more than one.
	// +optional
	// +kubebuilder:validation:MinLength=1
	QsvDevice string `json:"qsvDevice,omitempty"`

	// encodingThreadCount is how many threads ffmpeg uses. -1 picks one from the CPU count.
	// +optional
	// +kubebuilder:validation:Minimum=-1
	EncodingThreadCount *int32 `json:"encodingThreadCount,omitempty"`

	// enableDecodingColorDepth10Hevc decodes 10-bit HEVC in hardware.
	// +optional
	EnableDecodingColorDepth10Hevc *bool `json:"enableDecodingColorDepth10Hevc,omitempty"`

	// enableDecodingColorDepth10Vp9 decodes 10-bit VP9 in hardware.
	// +optional
	EnableDecodingColorDepth10Vp9 *bool `json:"enableDecodingColorDepth10Vp9,omitempty"`

	// enableDecodingColorDepth10HevcRext decodes 10-bit HEVC range extensions in hardware.
	// +optional
	EnableDecodingColorDepth10HevcRext *bool `json:"enableDecodingColorDepth10HevcRext,omitempty"`

	// enableDecodingColorDepth12HevcRext decodes 12-bit HEVC range extensions in hardware.
	// +optional
	EnableDecodingColorDepth12HevcRext *bool `json:"enableDecodingColorDepth12HevcRext,omitempty"`

	// preferSystemNativeHwDecoder uses the operating system's hardware decoder
	// where there is one.
	// +optional
	PreferSystemNativeHwDecoder *bool `json:"preferSystemNativeHwDecoder,omitempty"`

	// enableEnhancedNvdecDecoder uses NVIDIA's own decoder instead of CUVID.
	// +optional
	EnableEnhancedNvdecDecoder *bool `json:"enableEnhancedNvdecDecoder,omitempty"`

	// allowAv1Encoding allows AV1 hardware encoding.
	// +optional
	AllowAv1Encoding *bool `json:"allowAv1Encoding,omitempty"`

	// tonemappingAlgorithm is the algorithm for HDR to SDR tone mapping.
	// +optional
	// +kubebuilder:validation:Enum=none;clip;linear;gamma;reinhard;hable;mobius;bt2390
	TonemappingAlgorithm string `json:"tonemappingAlgorithm,omitempty"`

	// tonemappingMode is what tone mapping applies to.
	// +optional
	// +kubebuilder:validation:Enum=auto;max;rgb;lum;itp
	TonemappingMode string `json:"tonemappingMode,omitempty"`

	// tonemappingRange is the output color range of tone mapping.
	// +optional
	// +kubebuilder:validation:Enum=auto;tv;pc
	TonemappingRange string `json:"tonemappingRange,omitempty"`

	// tonemappingDesat desaturates highlights brighter than this level.
	// +optional
	TonemappingDesat *commonv1alpha1.Decimal `json:"tonemappingDesat,omitempty"`

	// tonemappingPeak is the signal peak to assume, in nits, when the video doesn't say.
	// +optional
	TonemappingPeak *commonv1alpha1.Decimal `json:"tonemappingPeak,omitempty"`

	// tonemappingParam tunes the tone mapping algorithm. 0 uses its default.
	// +optional
	TonemappingParam *commonv1alpha1.Decimal `json:"tonemappingParam,omitempty"`

	// vppTonemappingBrightness is the brightness gain of VPP tone mapping.
	// +optional
	VppTonemappingBrightness *commonv1alpha1.Decimal `json:"vppTonemappingBrightness,omitempty"`

	// vppTonemappingContrast is the contrast gain of VPP tone mapping.
	// +optional
	VppTonemappingContrast *commonv1alpha1.Decimal `json:"vppTonemappingContrast,omitempty"`

	// deinterlaceMethod is the deinterlacing filter.
	// +optional
	// +kubebuilder:validation:Enum=yadif;bwdif
	DeinterlaceMethod string `json:"deinterlaceMethod,omitempty"`

	// deinterlaceDoubleRate doubles the frame rate when deinterlacing.
	// +optional
	DeinterlaceDoubleRate *bool `json:"deinterlaceDoubleRate,omitempty"`

	// enableSegmentDeletion deletes transcoded segments once clients have them.
	// +optional
	EnableSegmentDeletion *bool `json:"enableSegmentDeletion,omitempty"`

	// segmentKeepSeconds is how many seconds of segments enableSegmentDeletion keeps.
	// +optional
	// +kubebuilder:validation:Minimum=0
	SegmentKeepSeconds *int32 `json:"segmentKeepSeconds,omitempty"`

	// enableAudioVbr encodes audio at a variable bitrate.
	// +optional
	EnableAudioVbr *bool `json:"enableAudioVbr,omitempty"`

	// downMixAudioBoost is the volume gain when downmixing to stereo.
	// +optional
	DownMixAudioBoost *commonv1alpha1.Decimal `json:"downMixAudioBoost,omitempty"`

	// downMixStereoAlgorithm is the algorithm for downmixing to stereo.
	// +optional
	// +kubebuilder:validation:Enum=None;Dave750;NightmodeDialogue;Rfc7845;Ac4
	DownMixStereoAlgorithm string `json:"downMixStereoAlgorithm,omitempty"`

	// maxMuxingQueueSize is the most packets ffmpeg buffers while muxing.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxMuxingQueueSize int32 `json:"maxMuxingQueueSize,omitempty"`

	// subtitleExtractionTimeoutMinutes is how long subtitle extraction may take.
	// +optional
	// +kubebuilder:validation:Minimum=1
	SubtitleExtractionTimeoutMinutes int32 `json:"subtitleExtractionTimeoutMinutes,omitempty"`

	// allowOnDemandMetadataBasedKeyframeExtractionForExtensions lists the file
	// extensions, such as mkv, whose key frames Jellyfin reads from metadata
	// when it needs them.
	// +optional
	// +listType=set
	AllowOnDemandMetadataBasedKeyframeExtractionForExtensions []string `json:"allowOnDemandMetadataBasedKeyframeExtractionForExtensions,omitempty"`
}

// JellyfinLibrary defines a media library in Jellyfin.
// +kubebuilder:validation:XValidation:rule="!has(self.typeOptions) || (self.collectionType == 'movies' ? self.typeOptions.all(t, t.type == 'Movie') : self.collectionType == 'tvshows' ? self.typeOptions.all(t, t.type in ['Series', 'Season', 'Episode']) : self.collectionType == 'music' ? self.typeOptions.all(t, t.type in ['MusicArtist', 'MusicAlbum', 'Audio', 'MusicVideo']) : self.collectionType == 'musicvideos' ? self.typeOptions.all(t, t.type == 'MusicVideo') : self.collectionType == 'homevideos' ? self.typeOptions.all(t, t.type in ['Video', 'Photo']) : self.collectionType == 'books' ? self.typeOptions.all(t, t.type == 'Book') : self.typeOptions.all(t, t.type in ['Series', 'Season', 'Episode', 'Movie']))",message="typeOptions names a type the library's collectionType doesn't hold"
type JellyfinLibrary struct {
	// name is the display name of the library.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// collectionType is the type of media in this library.
	// +required
	// +kubebuilder:validation:Enum=movies;tvshows;music;musicvideos;homevideos;books;mixed
	CollectionType string `json:"collectionType,omitempty"`

	// paths is the list of filesystem paths containing media for this library.
	// +required
	// +listType=set
	Paths []string `json:"paths,omitempty"`

	// enableRealtimeMonitor enables real-time monitoring of library paths for changes.
	// +optional
	EnableRealtimeMonitor *bool `json:"enableRealtimeMonitor,omitempty"`

	// enableTrickplayImageExtraction enables generation of trickplay (scrubbing preview) images.
	// +optional
	EnableTrickplayImageExtraction *bool `json:"enableTrickplayImageExtraction,omitempty"`

	// automaticRefreshIntervalDays is the number of days between automatic library scans.
	// +optional
	AutomaticRefreshIntervalDays *int32 `json:"automaticRefreshIntervalDays,omitempty"`

	// preferredMetadataLanguage overrides the server-level preferred metadata language for this library.
	// +optional
	// +kubebuilder:validation:MinLength=1
	PreferredMetadataLanguage string `json:"preferredMetadataLanguage,omitempty"`

	// metadataCountryCode overrides the server-level metadata country code for this library.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MetadataCountryCode string `json:"metadataCountryCode,omitempty"`

	// saveLocalMetadata saves artwork into the media folders.
	// +optional
	SaveLocalMetadata *bool `json:"saveLocalMetadata,omitempty"`

	// metadataSavers are the formats metadata is saved in next to the media,
	// such as Nfo for Kodi and other players.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	MetadataSavers []string `json:"metadataSavers,omitempty"`

	// enableEmbeddedTitles uses the titles embedded in files when there is no
	// other metadata.
	// +optional
	EnableEmbeddedTitles *bool `json:"enableEmbeddedTitles,omitempty"`

	// enableEmbeddedExtrasTitles uses the titles embedded in extras' files.
	// +optional
	EnableEmbeddedExtrasTitles *bool `json:"enableEmbeddedExtrasTitles,omitempty"`

	// enableEmbeddedEpisodeInfos uses the episode information embedded in files
	// when there is no other metadata.
	// +optional
	EnableEmbeddedEpisodeInfos *bool `json:"enableEmbeddedEpisodeInfos,omitempty"`

	// seasonZeroDisplayName is what season 0 is shown as, such as Specials.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SeasonZeroDisplayName string `json:"seasonZeroDisplayName,omitempty"`

	// enableChapterImageExtraction extracts chapter images from videos.
	// +optional
	EnableChapterImageExtraction *bool `json:"enableChapterImageExtraction,omitempty"`

	// extractChapterImagesDuringLibraryScan extracts chapter images while the
	// library is scanned rather than in the nightly task.
	// +optional
	ExtractChapterImagesDuringLibraryScan *bool `json:"extractChapterImagesDuringLibraryScan,omitempty"`

	// extractTrickplayImagesDuringLibraryScan extracts trickplay images while
	// the library is scanned rather than in the nightly task.
	// +optional
	ExtractTrickplayImagesDuringLibraryScan *bool `json:"extractTrickplayImagesDuringLibraryScan,omitempty"`

	// saveTrickplayWithMedia saves trickplay images next to the media.
	// +optional
	SaveTrickplayWithMedia *bool `json:"saveTrickplayWithMedia,omitempty"`

	// subtitleDownloadLanguages are the ISO 639-2 codes of the languages to
	// download subtitles in, such as eng.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	SubtitleDownloadLanguages []string `json:"subtitleDownloadLanguages,omitempty"`

	// saveSubtitlesWithMedia saves downloaded subtitles next to the media.
	// +optional
	SaveSubtitlesWithMedia *bool `json:"saveSubtitlesWithMedia,omitempty"`

	// requirePerfectSubtitleMatch downloads only subtitles that match the file
	// exactly.
	// +optional
	RequirePerfectSubtitleMatch *bool `json:"requirePerfectSubtitleMatch,omitempty"`

	// skipSubtitlesIfEmbeddedSubtitlesPresent skips the download when the file
	// already has subtitles in the language.
	// +optional
	SkipSubtitlesIfEmbeddedSubtitlesPresent *bool `json:"skipSubtitlesIfEmbeddedSubtitlesPresent,omitempty"`

	// skipSubtitlesIfAudioTrackMatches skips the download when the default
	// audio track is in the language.
	// +optional
	SkipSubtitlesIfAudioTrackMatches *bool `json:"skipSubtitlesIfAudioTrackMatches,omitempty"`

	// allowEmbeddedSubtitles is which embedded subtitles players may use.
	// +optional
	// +kubebuilder:validation:Enum=AllowAll;AllowText;AllowImage;AllowNone
	AllowEmbeddedSubtitles string `json:"allowEmbeddedSubtitles,omitempty"`

	// enableLUFSScan measures loudness so audio plays back at an even volume.
	// +optional
	EnableLUFSScan *bool `json:"enableLUFSScan,omitempty"`

	// automaticallyAddToCollection adds items to their box set collection.
	// +optional
	AutomaticallyAddToCollection *bool `json:"automaticallyAddToCollection,omitempty"`

	// enablePhotos picks up photos alongside the other media.
	// +optional
	EnablePhotos *bool `json:"enablePhotos,omitempty"`

	// enableAutomaticSeriesGrouping merges a series spread across several
	// folders into one.
	// +optional
	EnableAutomaticSeriesGrouping *bool `json:"enableAutomaticSeriesGrouping,omitempty"`

	// saveLyricsWithMedia saves downloaded lyrics next to the media.
	// +optional
	SaveLyricsWithMedia *bool `json:"saveLyricsWithMedia,omitempty"`

	// preferNonstandardArtistsTag reads artists from the ARTISTS tag, when a
	// file has one, instead of ARTIST.
	// +optional
	PreferNonstandardArtistsTag *bool `json:"preferNonstandardArtistsTag,omitempty"`

	// useCustomTagDelimiters splits artist and genre tags on
	// customTagDelimiters.
	// +optional
	UseCustomTagDelimiters *bool `json:"useCustomTagDelimiters,omitempty"`

	// customTagDelimiters are the characters artist and genre tags are split
	// on. A new library takes /, |, ; and \.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	CustomTagDelimiters []string `json:"customTagDelimiters,omitempty"`

	// delimiterWhitelist are artist and genre names never split, such as AC/DC.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	DelimiterWhitelist []string `json:"delimiterWhitelist,omitempty"`

	// typeOptions picks, for each kind of item in the library, the metadata and
	// image fetchers it uses, matched by type.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=4
	TypeOptions []JellyfinTypeOptions `json:"typeOptions,omitempty"`

	// subtitleFetchers are the subtitle downloaders used, in priority order,
	// such as Open Subtitles. The rest are disabled.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	SubtitleFetchers []string `json:"subtitleFetchers,omitempty"`

	// lyricFetchers are the lyric downloaders used, in priority order. The
	// rest are disabled.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	LyricFetchers []string `json:"lyricFetchers,omitempty"`

	// mediaSegmentProviders are the providers of intro, credits and other
	// segments used, in priority order. The rest are disabled.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	MediaSegmentProviders []string `json:"mediaSegmentProviders,omitempty"`

	// localMetadataReaders are the readers of metadata files next to the
	// media used, in priority order, such as Nfo. The rest are disabled.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	LocalMetadataReaders []string `json:"localMetadataReaders,omitempty"`
}

// JellyfinTypeOptions picks the fetchers one kind of item uses. Fetchers left
// out are disabled; a list left out stays as it is, or for a type the library
// has no options for yet, takes the fetchers Jellyfin enables by default.
// +kubebuilder:validation:XValidation:rule="has(self.metadataFetchers) || has(self.imageFetchers)",message="set metadataFetchers, imageFetchers or both"
type JellyfinTypeOptions struct {
	// type is the kind of item, which the library's collectionType must hold.
	// +required
	// +kubebuilder:validation:MaxLength=11
	// +kubebuilder:validation:Enum=Movie;Series;Season;Episode;MusicArtist;MusicAlbum;Audio;MusicVideo;Video;Photo;Book
	Type string `json:"type,omitempty"`

	// metadataFetchers are the metadata fetchers used, in priority order, such
	// as TheMovieDb.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	MetadataFetchers []string `json:"metadataFetchers,omitempty"`

	// imageFetchers are the image fetchers used, in priority order.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	ImageFetchers []string `json:"imageFetchers,omitempty"`
}

// JellyfinStatus defines the observed state of Jellyfin.
// +kubebuilder:validation:MinProperties=1
type JellyfinStatus struct {
	// conditions represent the current state of the Jellyfin resource.
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

	// initialized indicates whether the Jellyfin setup wizard has been completed.
	// +optional
	Initialized *bool `json:"initialized,omitempty"`

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

// Jellyfin configures a Jellyfin instance the operator connects to.
type Jellyfin struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Jellyfin.
	// +required
	Spec JellyfinSpec `json:"spec,omitzero"`

	// status is the observed state of the Jellyfin.
	// +optional
	Status JellyfinStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// JellyfinList contains a list of Jellyfin.
type JellyfinList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Jellyfin `json:"items"`
}

func (c *Jellyfin) GetConditions() *[]metav1.Condition { return &c.Status.Conditions }
func (c *Jellyfin) GetObservedGeneration() *int64      { return &c.Status.ObservedGeneration }
func (c *Jellyfin) GetLastSyncTime() **metav1.Time     { return &c.Status.LastSyncTime }
func (c *Jellyfin) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return c.Spec.Reconcile
}
func (c *Jellyfin) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Jellyfin) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
