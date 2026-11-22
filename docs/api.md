# API Reference

## Packages
- [arrchitect.dev/v1alpha1](#arrchitectdevv1alpha1)


## arrchitect.dev/v1alpha1


Package v1alpha1 contains the types the arrchitect.dev/v1alpha1 kinds share.
The API configures self-hosted media apps, such as Sonarr, Prowlarr and
Jellyfin, that already run in or alongside the cluster.

### Resource Types
- [Autobrr](#autobrr)
- [Bazarr](#bazarr)
- [FlareSolverr](#flaresolverr)
- [Jellyfin](#jellyfin)
- [Lidarr](#lidarr)
- [LidarrDownloadClient](#lidarrdownloadclient)
- [LidarrIndexer](#lidarrindexer)
- [Maintainerr](#maintainerr)
- [Plex](#plex)
- [Prowlarr](#prowlarr)
- [QBittorrent](#qbittorrent)
- [Radarr](#radarr)
- [RadarrDownloadClient](#radarrdownloadclient)
- [RadarrIndexer](#radarrindexer)
- [Readarr](#readarr)
- [ReadarrDownloadClient](#readarrdownloadclient)
- [ReadarrIndexer](#readarrindexer)
- [Sabnzbd](#sabnzbd)
- [Seerr](#seerr)
- [Sonarr](#sonarr)
- [SonarrDownloadClient](#sonarrdownloadclient)
- [SonarrIndexer](#sonarrindexer)
- [Tdarr](#tdarr)



#### AppConnection



AppConnection defines how the operator connects to an app's API.



_Appears in:_
- [AutobrrSpec](#autobrrspec)
- [BazarrSpec](#bazarrspec)
- [LidarrSpec](#lidarrspec)
- [ProwlarrSpec](#prowlarrspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SabnzbdSpec](#sabnzbdspec)
- [SonarrSpec](#sonarrspec)
- [TdarrSpec](#tdarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url is the base URL of the app (e.g., http://arr-sonarr.arr.svc.cluster.local:8989). |  | MinLength: 1 <br />Pattern: `^https?://` <br />Required: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the API key. |  | Required: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | tls configures TLS settings for the connection.<br />Only needed when the app uses HTTPS with self-signed or private CA certificates. |  | Optional: \{\} <br /> |


#### AppRef



AppRef names another app's resource in the same namespace, whose connection
is used in place of an address and credentials.



_Appears in:_
- [AutobrrDownloadClient](#autobrrdownloadclient)
- [BazarrAppConnection](#bazarrappconnection)
- [DownloadClient](#downloadclient)
- [LidarrDownloadClientSpec](#lidarrdownloadclientspec)
- [MaintainerrArrServer](#maintainerrarrserver)
- [MaintainerrJellyfin](#maintainerrjellyfin)
- [MaintainerrPlex](#maintainerrplex)
- [MaintainerrSeerrConnection](#maintainerrseerrconnection)
- [ProwlarrApplication](#prowlarrapplication)
- [RadarrDownloadClientSpec](#radarrdownloadclientspec)
- [ReadarrDownloadClientSpec](#readarrdownloadclientspec)
- [SeerrJellyfinAuth](#seerrjellyfinauth)
- [SeerrPlexAuth](#seerrplexauth)
- [SeerrServiceConnection](#seerrserviceconnection)
- [SonarrDownloadClientSpec](#sonarrdownloadclientspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `group` _string_ | group is the resource's API group. | arrchitect.dev | Enum: [arrchitect.dev] <br />Optional: \{\} <br /> |
| `kind` _string_ | kind is the resource's kind, such as Sonarr. |  | Enum: [Sonarr Radarr Lidarr Readarr Prowlarr Seerr Jellyfin Plex QBittorrent Sabnzbd] <br />Required: \{\} <br /> |
| `name` _string_ | name is the resource's name. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |


#### Autobrr



Autobrr configures an Autobrr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Autobrr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[AutobrrSpec](#autobrrspec)_ | spec is the desired state of the Autobrr. |  | Required: \{\} <br /> |
| `status` _[AutobrrStatus](#autobrrstatus)_ | status is the observed state of the Autobrr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### AutobrrDownloadClient



AutobrrDownloadClient defines a download client connection in Autobrr.



_Appears in:_
- [AutobrrSpec](#autobrrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the download client. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `appRef` _[AppRef](#appref)_ | appRef names the download client's resource, a QBittorrent, Sabnzbd,<br />Sonarr, Radarr, Lidarr or Readarr, whose kind and connection are used in<br />place of type, host, port, tls and the credentials. |  | Optional: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this download client is active. A new download<br />client is enabled unless this is false. |  | Optional: \{\} <br /> |
| `type` _string_ | type is the download client type. Deluge means Deluge 2 (Autobrr's DELUGE_V2). |  | Enum: [qBittorrent Deluge Transmission rTorrent SABnzbd Sonarr Radarr Lidarr Readarr Whisparr] <br />Optional: \{\} <br /> |
| `host` _string_ | host is the hostname or URL of the download client. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port of the download client. |  | Optional: \{\} <br /> |
| `tls` _boolean_ | tls enables TLS for the connection. |  | Optional: \{\} <br /> |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the username. |  | Optional: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the password. |  | Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the API key, for Sonarr, Radarr, Lidarr,<br />Readarr and SABnzbd clients. |  | Optional: \{\} <br /> |


#### AutobrrFeed



AutobrrFeed defines an RSS or Torznab feed source.



_Appears in:_
- [AutobrrSpec](#autobrrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the feed. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this feed is active. A new feed is enabled unless<br />this is false. |  | Optional: \{\} <br /> |
| `type` _string_ | type is the feed type. |  | Enum: [RSS Torznab Newznab] <br />Required: \{\} <br /> |
| `url` _string_ | url is the feed URL. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the feed API key (for Torznab/Newznab). |  | Optional: \{\} <br /> |
| `interval` _integer_ | interval is the feed polling interval in minutes. A new feed polls every<br />15 minutes unless this is set. |  | Optional: \{\} <br /> |
| `indexerRef` _string_ | indexerRef is the name of the indexer this feed belongs to: a torznab,<br />newznab or rss indexer declared under indexers. |  | MinLength: 1 <br />Required: \{\} <br /> |


#### AutobrrFilter



AutobrrFilter defines a release filter with match criteria and actions.



_Appears in:_
- [AutobrrSpec](#autobrrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the filter. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this filter is active. A new filter is enabled<br />unless this is false. |  | Optional: \{\} <br /> |
| `priority` _integer_ | priority controls filter evaluation order. Lower values are evaluated first. |  | Optional: \{\} <br /> |
| `matchReleases` _string_ | matchReleases is a comma-separated list of release name patterns to match (supports wildcards). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `exceptReleases` _string_ | exceptReleases is a comma-separated list of release name patterns to exclude. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `matchCategories` _string_ | matchCategories is a comma-separated list of categories to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `resolutions` _string array_ | resolutions is a list of acceptable resolutions. |  | Optional: \{\} <br /> |
| `sources` _string array_ | sources is a list of acceptable sources (e.g., "BluRay", "WEB-DL"). |  | Optional: \{\} <br /> |
| `codecs` _string array_ | codecs is a list of acceptable codecs (e.g., "x264", "x265", "HEVC"). |  | Optional: \{\} <br /> |
| `minSize` _string_ | minSize is the minimum release size (e.g., "100MB", "1GB"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `maxSize` _string_ | maxSize is the maximum release size (e.g., "50GB"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `delay` _integer_ | delay is the delay in seconds before grabbing a matched release. |  | Optional: \{\} <br /> |
| `useRegex` _boolean_ | useRegex enables regex matching for release patterns. |  | Optional: \{\} <br /> |
| `tags` _string array_ | tags is a list of tags for organizational purposes. |  | Optional: \{\} <br /> |
| `indexers` _string array_ | indexers lists the indexers, by name, whose releases this filter checks.<br />Autobrr checks a filter only against the indexers it lists. |  | Optional: \{\} <br /> |
| `matchReleaseGroups` _string_ | matchReleaseGroups is a comma-separated list of release groups to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `exceptReleaseGroups` _string_ | exceptReleaseGroups is a comma-separated list of release groups to skip. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `exceptCategories` _string_ | exceptCategories is a comma-separated list of categories to skip. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `matchUploaders` _string_ | matchUploaders is a comma-separated list of uploaders to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `exceptUploaders` _string_ | exceptUploaders is a comma-separated list of uploaders to skip. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `matchReleaseTags` _string_ | matchReleaseTags is a comma-separated list of release tags to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `exceptReleaseTags` _string_ | exceptReleaseTags is a comma-separated list of release tags to skip. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `useRegexReleaseTags` _boolean_ | useRegexReleaseTags reads the release tag patterns as regular expressions. |  | Optional: \{\} <br /> |
| `matchDescription` _string_ | matchDescription is a comma-separated list of description patterns to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `exceptDescription` _string_ | exceptDescription is a comma-separated list of description patterns to skip. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `useRegexDescription` _boolean_ | useRegexDescription reads the description patterns as regular expressions. |  | Optional: \{\} <br /> |
| `containers` _string array_ | containers is a list of acceptable containers, such as mkv. |  | Optional: \{\} <br /> |
| `matchHdr` _string array_ | matchHdr is a list of HDR formats to match, such as DV or HDR10+. |  | Optional: \{\} <br /> |
| `exceptHdr` _string array_ | exceptHdr is a list of HDR formats to skip. |  | Optional: \{\} <br /> |
| `matchOther` _string array_ | matchOther is a list of other release attributes to match, such as REMUX. |  | Optional: \{\} <br /> |
| `exceptOther` _string array_ | exceptOther is a list of other release attributes to skip. |  | Optional: \{\} <br /> |
| `origins` _string array_ | origins is a list of origins to match, such as P2P, INTERNAL or SCENE. |  | Optional: \{\} <br /> |
| `exceptOrigins` _string array_ | exceptOrigins is a list of origins to skip. |  | Optional: \{\} <br /> |
| `announceTypes` _string array_ | announceTypes is a list of announce types to match, such as NEW or PROMO. |  | Optional: \{\} <br /> |
| `matchLanguage` _string array_ | matchLanguage is a list of languages to match. |  | Optional: \{\} <br /> |
| `exceptLanguage` _string array_ | exceptLanguage is a list of languages to skip. |  | Optional: \{\} <br /> |
| `shows` _string_ | shows is a comma-separated list of shows to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `seasons` _string_ | seasons is a comma-separated list of seasons or ranges to match, such as 1,3-5. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `episodes` _string_ | episodes is a comma-separated list of episodes or ranges to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `smartEpisode` _boolean_ | smartEpisode skips episodes older than ones already grabbed. |  | Optional: \{\} <br /> |
| `years` _string_ | years is a comma-separated list of years or ranges to match, such as 2018-2024. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `months` _string_ | months is a comma-separated list of months to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `days` _string_ | days is a comma-separated list of days to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `freeleech` _boolean_ | freeleech matches only freeleech releases. |  | Optional: \{\} <br /> |
| `freeleechPercent` _string_ | freeleechPercent is the freeleech percentages to match, such as 50%,75%-100%. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `maxDownloads` _integer_ | maxDownloads caps how many releases the filter grabs per maxDownloadsUnit. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `maxDownloadsUnit` _string_ | maxDownloadsUnit is the period maxDownloads counts over. |  | Enum: [MINUTE HOUR DAY WEEK MONTH EVER] <br />Optional: \{\} <br /> |
| `minSeeders` _integer_ | minSeeders is the fewest seeders a release may have. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `maxSeeders` _integer_ | maxSeeders is the most seeders a release may have. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `minLeechers` _integer_ | minLeechers is the fewest leechers a release may have. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `maxLeechers` _integer_ | maxLeechers is the most leechers a release may have. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `exceptTags` _string array_ | exceptTags is a list of tags to skip. |  | Optional: \{\} <br /> |
| `tagsMatchLogic` _string_ | tagsMatchLogic decides whether a release needs any or all of tags. |  | Enum: [ANY ALL] <br />Optional: \{\} <br /> |
| `exceptTagsMatchLogic` _string_ | exceptTagsMatchLogic decides whether any or all of exceptTags skip a release. |  | Enum: [ANY ALL] <br />Optional: \{\} <br /> |
| `artists` _string_ | artists is a comma-separated list of artists to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `albums` _string_ | albums is a comma-separated list of albums to match. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `matchReleaseTypes` _string array_ | matchReleaseTypes is a list of music release types to match, such as Album or EP. |  | Optional: \{\} <br /> |
| `exceptReleaseTypes` _string_ | exceptReleaseTypes is a comma-separated list of music release types to skip. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `formats` _string array_ | formats is a list of audio formats to match, such as FLAC or MP3. |  | Optional: \{\} <br /> |
| `quality` _string array_ | quality is a list of audio qualities to match, such as Lossless or 320. |  | Optional: \{\} <br /> |
| `media` _string array_ | media is a list of music media to match, such as CD or WEB. |  | Optional: \{\} <br /> |
| `perfectFlac` _boolean_ | perfectFlac matches only lossless rips with a log of 100 and a cue. |  | Optional: \{\} <br /> |
| `cue` _boolean_ | cue matches only releases with a cue file. |  | Optional: \{\} <br /> |
| `log` _boolean_ | log matches only releases with a rip log. |  | Optional: \{\} <br /> |
| `logScore` _integer_ | logScore is the lowest rip log score to match. |  | Maximum: 100 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `actions` _[AutobrrFilterAction](#autobrrfilteraction) array_ | actions defines what to do when a release matches this filter. |  | Optional: \{\} <br /> |


#### AutobrrFilterAction



AutobrrFilterAction defines an action to take when a filter matches.



_Appears in:_
- [AutobrrFilter](#autobrrfilter)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the action. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this action is active. A new action is enabled<br />unless this is false. |  | Optional: \{\} <br /> |
| `type` _string_ | type is the action type. Deluge means Deluge 2 (Autobrr's DELUGE_V2). |  | Enum: [qBittorrent Deluge Transmission rTorrent SABnzbd Sonarr Radarr Lidarr Readarr Whisparr Webhook Exec WatchFolder] <br />Required: \{\} <br /> |
| `clientRef` _string_ | clientRef is the name of the download client to use (must match a downloadClients entry). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `category` _string_ | category overrides the download client's default category. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `savePath` _string_ | savePath overrides the download client's default save path. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `webhookURL` _string_ | webhookURL is the webhook URL (for Webhook action type). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `execCommand` _string_ | execCommand is the command to execute (for Exec action type). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `execArgs` _string_ | execArgs is the arguments for the exec command. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `watchFolder` _string_ | watchFolder is the folder path (for WatchFolder action type). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `tags` _string_ | tags is a comma-separated list of tags the download client gives the torrent. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `label` _string_ | label is the label the download client gives the torrent. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `downloadPath` _string_ | downloadPath is where the download client keeps the torrent while it downloads. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `paused` _boolean_ | paused adds the torrent paused. |  | Optional: \{\} <br /> |
| `ignoreRules` _boolean_ | ignoreRules sends the release even when the download client is past its limits. |  | Optional: \{\} <br /> |
| `skipHashCheck` _boolean_ | skipHashCheck adds the torrent without checking existing data. |  | Optional: \{\} <br /> |
| `firstLastPiecePrio` _boolean_ | firstLastPiecePrio downloads the first and last pieces first. |  | Optional: \{\} <br /> |
| `contentLayout` _string_ | contentLayout decides whether the torrent's files go in a folder of their own. |  | Enum: [ORIGINAL SUBFOLDER_CREATE SUBFOLDER_NONE] <br />Optional: \{\} <br /> |
| `priority` _string_ | priority is where the torrent goes in the download client's queue. |  | Enum: [MAX MIN LOW NORMAL HIGH] <br />Optional: \{\} <br /> |
| `limitUploadSpeed` _integer_ | limitUploadSpeed caps the torrent's upload speed, in KiB/s. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `limitDownloadSpeed` _integer_ | limitDownloadSpeed caps the torrent's download speed, in KiB/s. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `limitRatio` _[Decimal](#decimal)_ | limitRatio is the share ratio at which the torrent stops seeding. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |
| `limitSeedTime` _integer_ | limitSeedTime is how many minutes the torrent seeds. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `reannounceSkip` _boolean_ | reannounceSkip adds the torrent without waiting for it to announce. |  | Optional: \{\} <br /> |
| `reannounceDelete` _boolean_ | reannounceDelete removes the torrent if it never announces. |  | Optional: \{\} <br /> |
| `reannounceInterval` _integer_ | reannounceInterval is how many seconds to wait between reannounce attempts. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `reannounceMaxAttempts` _integer_ | reannounceMaxAttempts is how many times to reannounce. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `webhookType` _string_ | webhookType is the content type the webhook sends, such as JSON. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `webhookMethod` _string_ | webhookMethod is the HTTP method the webhook uses. |  | Enum: [GET POST PUT PATCH DELETE] <br />Optional: \{\} <br /> |
| `webhookData` _string_ | webhookData is the body the webhook sends; Autobrr fills in its macros. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `webhookHeaders` _string array_ | webhookHeaders are headers the webhook sends, as Name=Value. |  | Optional: \{\} <br /> |
| `externalDownloadClient` _string_ | externalDownloadClient is the download client, by name in Sonarr, Radarr, Lidarr or Readarr,<br />that the app sends the release to. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### AutobrrIRCChannel



AutobrrIRCChannel defines an IRC channel to monitor.



_Appears in:_
- [AutobrrIRCNetwork](#autobrrircnetwork)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the channel name (e.g., "#announce"). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the channel password/key. |  | Optional: \{\} <br /> |


#### AutobrrIRCNetwork



AutobrrIRCNetwork defines an IRC network connection for announce monitoring.



_Appears in:_
- [AutobrrSpec](#autobrrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the IRC network. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this IRC network connection is active. A new<br />network is enabled unless this is false. |  | Optional: \{\} <br /> |
| `server` _string_ | server is the IRC server address (e.g., "irc.example.com"). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `port` _integer_ | port is the IRC server port. |  | Maximum: 65535 <br />Minimum: 1 <br />Required: \{\} <br /> |
| `tls` _boolean_ | tls enables TLS for the IRC connection. |  | Optional: \{\} <br /> |
| `nick` _string_ | nick is the IRC nickname to use. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `authMechanism` _string_ | authMechanism is the authentication mechanism (SASL_PLAIN, NickServ, None).<br />A new network uses None unless this is set. |  | Enum: [SASL_PLAIN NickServ None] <br />Optional: \{\} <br /> |
| `authAccountSecretRef` _[SecretKeyRef](#secretkeyref)_ | authAccountSecretRef references a Secret containing the auth account name. |  | Optional: \{\} <br /> |
| `authPasswordSecretRef` _[SecretKeyRef](#secretkeyref)_ | authPasswordSecretRef references a Secret containing the auth password. |  | Optional: \{\} <br /> |
| `inviteCommand` _string_ | inviteCommand is the IRC command to send to request channel invites. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `channels` _[AutobrrIRCChannel](#autobrrircchannel) array_ | channels defines the IRC channels to join. |  | Optional: \{\} <br /> |


#### AutobrrIndexer



AutobrrIndexer defines an indexer/tracker connection in Autobrr.



_Appears in:_
- [AutobrrSpec](#autobrrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the indexer. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this indexer is active. A new indexer is enabled<br />unless this is false. |  | Optional: \{\} <br /> |
| `identifier` _string_ | identifier is the Autobrr indexer definition, such as "ipt" or "btn", or<br />"torznab", "newznab" or "rss" for an indexer that a feed reads from. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `baseURL` _string_ | baseURL is the tracker's site URL. It defaults to the definition's first URL. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `settings` _[ConfigField](#configfield) array_ | settings are the definition's settings, such as passkey or rsskey. |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### AutobrrSpec



AutobrrSpec defines the desired state of Autobrr.



_Appears in:_
- [Autobrr](#autobrr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[AppConnection](#appconnection)_ | connection defines how to connect to the Autobrr instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `downloadClients` _[AutobrrDownloadClient](#autobrrdownloadclient) array_ | downloadClients defines download client connections in Autobrr. |  | Optional: \{\} <br /> |
| `indexers` _[AutobrrIndexer](#autobrrindexer) array_ | indexers defines indexer/tracker connections. |  | Optional: \{\} <br /> |
| `ircNetworks` _[AutobrrIRCNetwork](#autobrrircnetwork) array_ | ircNetworks defines IRC network connections for announce channel monitoring. |  | Optional: \{\} <br /> |
| `feeds` _[AutobrrFeed](#autobrrfeed) array_ | feeds defines RSS/Torznab feed sources. |  | Optional: \{\} <br /> |
| `filters` _[AutobrrFilter](#autobrrfilter) array_ | filters defines release filters with match criteria and actions. |  | Optional: \{\} <br /> |


#### AutobrrStatus



AutobrrStatus defines the observed state of Autobrr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Autobrr](#autobrr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Autobrr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### Bazarr



Bazarr configures a Bazarr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Bazarr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[BazarrSpec](#bazarrspec)_ | spec is the desired state of the Bazarr. |  | Required: \{\} <br /> |
| `status` _[BazarrStatus](#bazarrstatus)_ | status is the observed state of the Bazarr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### BazarrAppConnection



BazarrAppConnection defines a connection to a Sonarr or Radarr instance from Bazarr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [BazarrSpec](#bazarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `appRef` _[AppRef](#appref)_ | appRef names the application's resource, whose connection is used in<br />place of host, port, basePath, ssl and apiKeySecretRef. |  | Optional: \{\} <br /> |
| `host` _string_ | host is the hostname or IP of the application. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port of the application. |  | Optional: \{\} <br /> |
| `basePath` _string_ | basePath is the URL base path of the application. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `ssl` _boolean_ | ssl enables HTTPS connections to the application. |  | Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the application's API key. |  | Optional: \{\} <br /> |
| `httpTimeout` _integer_ | httpTimeout is the HTTP request timeout in seconds. |  | Optional: \{\} <br /> |
| `fullUpdate` _string_ | fullUpdate controls how often a full library sync is performed. |  | Enum: [Daily Weekly Manually] <br />Optional: \{\} <br /> |
| `onlyMonitored` _boolean_ | onlyMonitored limits subtitle searches to monitored items only. |  | Optional: \{\} <br /> |
| `syncInterval` _integer_ | syncInterval is how often, in minutes, Bazarr syncs the library. |  | Enum: [15 60 180 360 720 1440 10080] <br />Optional: \{\} <br /> |
| `excludedTags` _string array_ | excludedTags leaves out items with any of these tags. |  | Optional: \{\} <br /> |
| `excludedSeriesTypes` _string array_ | excludedSeriesTypes leaves out series of these types. Only applicable<br />when used with Sonarr. |  | items:Enum: [standard anime daily] <br />Optional: \{\} <br /> |
| `excludeSeasonZero` _boolean_ | excludeSeasonZero leaves out specials. Only applicable when used with Sonarr. |  | Optional: \{\} <br /> |


#### BazarrGeneral



BazarrGeneral configures general Bazarr settings.



_Appears in:_
- [BazarrSpec](#bazarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `useSonarr` _boolean_ | useSonarr enables the Sonarr integration. |  | Optional: \{\} <br /> |
| `useRadarr` _boolean_ | useRadarr enables the Radarr integration. |  | Optional: \{\} <br /> |
| `minimumScore` _integer_ | minimumScore is the minimum subtitle match score for series. |  | Optional: \{\} <br /> |
| `minimumScoreMovie` _integer_ | minimumScoreMovie is the minimum subtitle match score for movies. |  | Optional: \{\} <br /> |
| `useScenename` _boolean_ | useScenename uses scene names for subtitle searching. |  | Optional: \{\} <br /> |
| `usePostprocessing` _boolean_ | usePostprocessing enables post-processing of downloaded subtitles. |  | Optional: \{\} <br /> |
| `adaptiveSearching` _boolean_ | adaptiveSearching enables adaptive searching for subtitles. |  | Optional: \{\} <br /> |
| `multithreading` _boolean_ | multithreading enables multithreaded subtitle downloading. |  | Optional: \{\} <br /> |
| `chmodEnabled` _boolean_ | chmodEnabled enables setting file permissions on subtitle files. |  | Optional: \{\} <br /> |
| `upgradeSubs` _boolean_ | upgradeSubs enables automatic subtitle upgrades when better ones are found. |  | Optional: \{\} <br /> |
| `upgradeFrequency` _integer_ | upgradeFrequency is the frequency in hours to check for subtitle upgrades. |  | Optional: \{\} <br /> |
| `wantedSearchFrequency` _integer_ | wantedSearchFrequency is the frequency in hours to search for wanted subtitles. |  | Optional: \{\} <br /> |
| `utf8Encode` _boolean_ | utf8Encode encodes subtitles to UTF-8. |  | Optional: \{\} <br /> |
| `ignorePgsSubs` _boolean_ | ignorePgsSubs ignores PGS (image-based) subtitles. |  | Optional: \{\} <br /> |
| `ignoreVobsubSubs` _boolean_ | ignoreVobsubSubs ignores VobSub (image-based) subtitles. |  | Optional: \{\} <br /> |
| `ignoreAssSubs` _boolean_ | ignoreAssSubs ignores ASS/SSA subtitles. |  | Optional: \{\} <br /> |
| `embeddedSubtitlesParser` _string_ | embeddedSubtitlesParser is the parser used for embedded subtitles. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `hiExtension` _string_ | hiExtension is the file extension used for hearing-impaired subtitle files. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `pathMappings` _[BazarrPathMapping](#bazarrpathmapping) array_ | pathMappings translate Sonarr's paths into the paths Bazarr sees the same<br />files at. Bazarr separates the two with a comma, so neither may have one. |  | Optional: \{\} <br /> |
| `pathMappingsMovie` _[BazarrPathMapping](#bazarrpathmapping) array_ | pathMappingsMovie translate Radarr's paths into the paths Bazarr sees the<br />same files at. |  | Optional: \{\} <br /> |
| `seriesDefaultProfile` _string_ | seriesDefaultProfile is the language profile, by name, that new series<br />get. Setting it turns the default on. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `movieDefaultProfile` _string_ | movieDefaultProfile is the language profile, by name, that new movies<br />get. Setting it turns the default on. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `useEmbeddedSubs` _boolean_ | useEmbeddedSubs counts subtitles embedded in the media file as present. |  | Optional: \{\} <br /> |
| `subfolder` _string_ | subfolder is where subtitles are saved: next to the media file, in a<br />folder relative to it, or in an absolute folder. |  | Enum: [current relative absolute] <br />Optional: \{\} <br /> |
| `subfolderCustom` _string_ | subfolderCustom is the folder subfolder relative or absolute saves to. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `chmod` _string_ | chmod is the octal permissions chmodEnabled sets on subtitle files, such as 0640. |  | MinLength: 1 <br />Pattern: `^0?[0-7]\{3\}$` <br />Optional: \{\} <br /> |
| `postprocessingCmd` _string_ | postprocessingCmd is the command usePostprocessing runs after a subtitle<br />is downloaded. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `usePostprocessingThreshold` _boolean_ | usePostprocessingThreshold runs postprocessingCmd for an episode subtitle<br />only when its score is below postprocessingThreshold. |  | Optional: \{\} <br /> |
| `postprocessingThreshold` _integer_ | postprocessingThreshold is the episode subtitle score, in percent, below<br />which postprocessingCmd runs. |  | Maximum: 100 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `usePostprocessingThresholdMovie` _boolean_ | usePostprocessingThresholdMovie runs postprocessingCmd for a movie<br />subtitle only when its score is below postprocessingThresholdMovie. |  | Optional: \{\} <br /> |
| `postprocessingThresholdMovie` _integer_ | postprocessingThresholdMovie is the movie subtitle score, in percent,<br />below which postprocessingCmd runs. |  | Maximum: 100 <br />Minimum: 0 <br />Optional: \{\} <br /> |


#### BazarrLanguageItem



BazarrLanguageItem defines a language entry within a language profile.



_Appears in:_
- [BazarrLanguageProfile](#bazarrlanguageprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `language` _string_ | language is the ISO 639-1 language code. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `hi` _boolean_ | hi indicates whether hearing-impaired subtitles are preferred. |  | Optional: \{\} <br /> |
| `forced` _boolean_ | forced indicates whether forced subtitles are preferred. |  | Optional: \{\} <br /> |
| `audioExclude` _boolean_ | audioExclude excludes this language if audio already matches. |  | Optional: \{\} <br /> |


#### BazarrLanguageProfile



BazarrLanguageProfile defines a language profile for subtitle selection.



_Appears in:_
- [BazarrLanguages](#bazarrlanguages)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `profileId` _integer_ | profileId is the unique identifier for this profile. |  | Required: \{\} <br /> |
| `name` _string_ | name is the display name of the language profile. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `items` _[BazarrLanguageItem](#bazarrlanguageitem) array_ | items defines the language items in this profile. |  | Optional: \{\} <br /> |


#### BazarrLanguages



BazarrLanguages configures language settings and profiles.



_Appears in:_
- [BazarrSpec](#bazarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _string array_ | enabled is the list of enabled subtitle languages as ISO 639-1 codes. |  | Optional: \{\} <br /> |
| `profiles` _[BazarrLanguageProfile](#bazarrlanguageprofile) array_ | profiles defines language profiles for subtitle selection. |  | Optional: \{\} <br /> |


#### BazarrPathMapping



BazarrPathMapping translates a path Sonarr or Radarr reports into the path
Bazarr reads the same file at.



_Appears in:_
- [BazarrGeneral](#bazarrgeneral)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `remotePath` _string_ | remotePath is the path as Sonarr or Radarr reports it. |  | MinLength: 1 <br />Pattern: `^[^,]+$` <br />Required: \{\} <br /> |
| `localPath` _string_ | localPath is the path inside the Bazarr container. |  | MinLength: 1 <br />Pattern: `^[^,]+$` <br />Required: \{\} <br /> |


#### BazarrProvider



BazarrProvider defines a subtitle provider configuration.



_Appears in:_
- [BazarrSpec](#bazarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the provider key (e.g., opensubtitlescom, addic7ed). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enabled` _boolean_ | enabled controls whether this provider is active. |  | Optional: \{\} <br /> |
| `settings` _[ConfigField](#configfield) array_ | settings are the provider's settings, each a text value or read from a<br />Secret in the resource's namespace, such as a password or API key. |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### BazarrSpec



BazarrSpec defines the desired state of Bazarr.



_Appears in:_
- [Bazarr](#bazarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[AppConnection](#appconnection)_ | connection defines how to connect to the Bazarr instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `general` _[BazarrGeneral](#bazarrgeneral)_ | general configures general Bazarr settings. |  | Optional: \{\} <br /> |
| `sonarrConnection` _[BazarrAppConnection](#bazarrappconnection)_ | sonarrConnection configures the connection to a Sonarr instance. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `radarrConnection` _[BazarrAppConnection](#bazarrappconnection)_ | radarrConnection configures the connection to a Radarr instance. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `subSync` _[BazarrSubSync](#bazarrsubsync)_ | subSync configures subtitle synchronization settings. |  | Optional: \{\} <br /> |
| `providers` _[BazarrProvider](#bazarrprovider) array_ | providers defines subtitle provider configurations. |  | Optional: \{\} <br /> |
| `languages` _[BazarrLanguages](#bazarrlanguages)_ | languages configures language settings and profiles. |  | Optional: \{\} <br /> |


#### BazarrStatus



BazarrStatus defines the observed state of Bazarr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Bazarr](#bazarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Bazarr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### BazarrSubSync



BazarrSubSync configures subtitle synchronization settings.



_Appears in:_
- [BazarrSpec](#bazarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `useSubsync` _boolean_ | useSubsync enables automatic subtitle synchronization. |  | Optional: \{\} <br /> |
| `useSubsyncThreshold` _boolean_ | useSubsyncThreshold enables a score threshold for series subtitle sync. |  | Optional: \{\} <br /> |
| `subsyncThreshold` _integer_ | subsyncThreshold is the minimum score threshold for series subtitle sync. |  | Optional: \{\} <br /> |
| `useSubsyncMovieThreshold` _boolean_ | useSubsyncMovieThreshold enables a score threshold for movie subtitle sync. |  | Optional: \{\} <br /> |
| `subsyncMovieThreshold` _integer_ | subsyncMovieThreshold is the minimum score threshold for movie subtitle sync. |  | Optional: \{\} <br /> |


#### ConfigField



ConfigField is a named field of a provider's configuration. It sets value
or valueFrom, not both.

_Validation:_
- MaxProperties: 2

_Appears in:_
- [AutobrrIndexer](#autobrrindexer)
- [BazarrProvider](#bazarrprovider)
- [CustomFormatSpecification](#customformatspecification)
- [DownloadClient](#downloadclient)
- [Indexer](#indexer)
- [LidarrDownloadClientSpec](#lidarrdownloadclientspec)
- [LidarrIndexerSpec](#lidarrindexerspec)
- [MaintainerrNotification](#maintainerrnotification)
- [Notification](#notification)
- [ProwlarrDownloadClient](#prowlarrdownloadclient)
- [ProwlarrIndexer](#prowlarrindexer)
- [RadarrDownloadClientSpec](#radarrdownloadclientspec)
- [RadarrIndexerSpec](#radarrindexerspec)
- [ReadarrDownloadClientSpec](#readarrdownloadclientspec)
- [ReadarrIndexerSpec](#readarrindexerspec)
- [SeerrNotificationAgent](#seerrnotificationagent)
- [SonarrDownloadClientSpec](#sonarrdownloadclientspec)
- [SonarrIndexerSpec](#sonarrindexerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the field name. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `value` _[FieldValue](#fieldvalue)_ | value is the field value: a string, number, boolean or list. |  | Type:  <br />Optional: \{\} <br /> |
| `valueFrom` _[ValueSource](#valuesource)_ | valueFrom reads the field value from a source. |  | Optional: \{\} <br /> |


#### CustomFormat



CustomFormat defines a custom format with matching specifications.
Custom formats score releases based on matching conditions and are used
within quality profiles to guide upgrade decisions.



_Appears in:_
- [LidarrSpec](#lidarrspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the custom format. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `includeCustomFormatWhenRenaming` _boolean_ | includeCustomFormatWhenRenaming includes this format's name in renamed filenames<br />via the \{Custom Formats\} naming token. |  | Optional: \{\} <br /> |
| `specifications` _[CustomFormatSpecification](#customformatspecification) array_ | specifications defines the matching conditions for this custom format.<br />A release matches if all required specs match and at least one non-required spec matches. |  | Optional: \{\} <br /> |


#### CustomFormatSpecification



CustomFormatSpecification defines a single matching condition within a custom format.



_Appears in:_
- [CustomFormat](#customformat)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is a human-readable label for this condition. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the specification type.<br />Common values: ReleaseTitleSpecification, SourceSpecification, ResolutionSpecification,<br />QualityModifierSpecification, LanguageSpecification, ReleaseGroupSpecification,<br />EditionSpecification (Radarr), ReleaseTypeSpecification (Sonarr), SizeSpecification,<br />IndexerFlagSpecification. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `negate` _boolean_ | negate inverts the match logic. If true, the spec matches when the condition is NOT met. |  | Optional: \{\} <br /> |
| `required` _boolean_ | required makes this spec mandatory. If true, the custom format only matches when<br />this spec matches. If false, this spec participates in OR logic with other non-required specs. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines the specification's configuration values.<br />Most specs have a single field named "value" (e.g., a regex pattern or enum int).<br />SizeSpecification has "min" and "max" fields (floats in GB). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### Decimal

_Underlying type:_ _string_

Decimal is a number that may have a fractional part, such as 17.1.

_Validation:_
- Minimum: 0
- Type: number

_Appears in:_
- [AutobrrFilterAction](#autobrrfilteraction)
- [JellyfinEncoding](#jellyfinencoding)
- [QualityDefinition](#qualitydefinition)



#### DelayProfile



DelayProfile holds a release back for a while, so a better one can turn up
before the app grabs it. A profile is matched to the app's by its tags, and
the app allows a tag in only one profile.



_Appears in:_
- [LidarrSpec](#lidarrspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `tagLabels` _string array_ | tagLabels limits the profile to items with one of these tags. The profile<br />without tags is the app's default, which covers everything the others don't. |  | Optional: \{\} <br /> |
| `enableUsenet` _boolean_ | enableUsenet allows Usenet releases. |  | Optional: \{\} <br /> |
| `enableTorrent` _boolean_ | enableTorrent allows torrent releases. |  | Optional: \{\} <br /> |
| `preferredProtocol` _string_ | preferredProtocol is the protocol the app picks when a release is on both. |  | Enum: [usenet torrent] <br />Optional: \{\} <br /> |
| `usenetDelay` _integer_ | usenetDelay is how many minutes the app waits before grabbing a Usenet release. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `torrentDelay` _integer_ | torrentDelay is how many minutes the app waits before grabbing a torrent release. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `bypassIfHighestQuality` _boolean_ | bypassIfHighestQuality skips the delay for a release at the quality<br />profile's highest allowed quality. |  | Optional: \{\} <br /> |
| `bypassIfAboveCustomFormatScore` _boolean_ | bypassIfAboveCustomFormatScore skips the delay for a release whose custom<br />format score reaches minimumCustomFormatScore. |  | Optional: \{\} <br /> |
| `minimumCustomFormatScore` _integer_ | minimumCustomFormatScore is the score bypassIfAboveCustomFormatScore needs. |  | Optional: \{\} <br /> |


#### DownloadClient



DownloadClient defines a download client connection.



_Appears in:_
- [LidarrDownloadClientSpec](#lidarrdownloadclientspec)
- [RadarrDownloadClientSpec](#radarrdownloadclientspec)
- [ReadarrDownloadClientSpec](#readarrdownloadclientspec)
- [SonarrDownloadClientSpec](#sonarrdownloadclientspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the download client. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `appRef` _[AppRef](#appref)_ | appRef names the download client's resource, a QBittorrent or Sabnzbd,<br />whose kind and connection are used in place of implementation, protocol,<br />host, port, useSsl, urlBase and the credentials. |  | Optional: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this download client is active. A new download<br />client is enabled unless this is false. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the download protocol. |  | Enum: [torrent usenet] <br />Optional: \{\} <br /> |
| `implementation` _string_ | implementation is the download client type (e.g., QBittorrent, Sabnzbd, NzbGet, Transmission, Deluge). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `configContract` _string_ | configContract overrides the default config contract name.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `host` _string_ | host is the hostname or IP of the download client. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port of the download client. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `useSsl` _boolean_ | useSsl enables SSL/TLS for the download client connection. |  | Optional: \{\} <br /> |
| `urlBase` _string_ | urlBase is the base URL path for the download client (e.g., "/qbittorrent"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `category` _string_ | category is the download category to use. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the download client priority. Lower values are higher priority.<br />A new download client gets 1. |  | Maximum: 50 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `removeCompletedDownloads` _boolean_ | removeCompletedDownloads controls whether completed downloads are removed.<br />A new download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `removeFailedDownloads` _boolean_ | removeFailedDownloads controls whether failed downloads are removed. A new<br />download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the username for authentication. |  | Optional: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the password for authentication. |  | Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the API key (for Usenet clients like SABnzbd/NzbGet). |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines additional implementation-specific configuration fields.<br />Use these for settings not covered by first-class fields (e.g., initialState, sequentialOrder). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### FieldValue



FieldValue wraps a value that can be a string, int, or bool.

_Validation:_
- Type: 

_Appears in:_
- [ConfigField](#configfield)



#### FlareSolverr



FlareSolverr configures a FlareSolverr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `FlareSolverr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[FlareSolverrSpec](#flaresolverrspec)_ | spec is the desired state of the FlareSolverr. |  | Required: \{\} <br /> |
| `status` _[FlareSolverrStatus](#flaresolverrstatus)_ | status is the observed state of the FlareSolverr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### FlareSolverrConnection



FlareSolverrConnection defines how the operator connects to a FlareSolverr instance.
FlareSolverr typically has no authentication; it is expected to be network-isolated.



_Appears in:_
- [FlareSolverrSpec](#flaresolverrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url is the base URL of the FlareSolverr instance (e.g., http://flaresolverr:8191). |  | MinLength: 1 <br />Pattern: `^https?://` <br />Required: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | tls configures TLS settings for the connection.<br />Only needed when FlareSolverr uses HTTPS with self-signed or private CA certificates. |  | Optional: \{\} <br /> |


#### FlareSolverrSession



FlareSolverrSession defines a named browser session to maintain in FlareSolverr.



_Appears in:_
- [FlareSolverrSpec](#flaresolverrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the session identifier. Must be unique across sessions. A client<br />reuses the session by sending this name with its requests; Prowlarr's<br />FlareSolverr proxy doesn't. |  | MinLength: 1 <br />Required: \{\} <br /> |


#### FlareSolverrSpec



FlareSolverrSpec defines the desired state of FlareSolverr.



_Appears in:_
- [FlareSolverr](#flaresolverr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[FlareSolverrConnection](#flaresolverrconnection)_ | connection defines how to connect to the FlareSolverr instance.<br />FlareSolverr does not require authentication by default. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `sessions` _[FlareSolverrSession](#flaresolverrsession) array_ | sessions defines persistent browser sessions to maintain.<br />Named sessions are reused across requests and survive between solves,<br />improving performance for repeated requests to the same site. |  | Optional: \{\} <br /> |


#### FlareSolverrStatus



FlareSolverrStatus defines the observed state of FlareSolverr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [FlareSolverr](#flaresolverr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the FlareSolverr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `activeSessions` _integer_ | activeSessions is the number of active sessions managed by this resource. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### Indexer



Indexer defines an indexer in Sonarr, Radarr, Lidarr or Readarr.



_Appears in:_
- [LidarrIndexerSpec](#lidarrindexerspec)
- [RadarrIndexerSpec](#radarrindexerspec)
- [ReadarrIndexerSpec](#readarrindexerspec)
- [SonarrIndexerSpec](#sonarrindexerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the indexer. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this indexer is active. False turns off RSS,<br />automatic and interactive search; true turns on those not set below. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the indexer protocol. |  | Enum: [torrent usenet] <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the indexer type (e.g., Newznab, Torznab). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name for the implementation.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the indexer priority. Lower values are higher priority. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `enableRss` _boolean_ | enableRss enables RSS feed polling for this indexer. A new indexer<br />polls unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableAutomaticSearch` _boolean_ | enableAutomaticSearch enables automatic search for this indexer. A new<br />indexer searches unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableInteractiveSearch` _boolean_ | enableInteractiveSearch enables interactive (manual) search for this<br />indexer. A new indexer allows it unless this or enable is false. |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines the indexer's configuration fields (e.g., baseUrl, apiKey, categories). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### Jellyfin



Jellyfin configures a Jellyfin instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Jellyfin` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[JellyfinSpec](#jellyfinspec)_ | spec is the desired state of the Jellyfin. |  | Required: \{\} <br /> |
| `status` _[JellyfinStatus](#jellyfinstatus)_ | status is the observed state of the Jellyfin. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### JellyfinAdminUser



JellyfinAdminUser defines the admin user credentials for Jellyfin.



_Appears in:_
- [JellyfinSpec](#jellyfinspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the admin username. |  | Required: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the admin password. |  | Required: \{\} <br /> |


#### JellyfinConnection



JellyfinConnection defines how the operator connects to Jellyfin.
Jellyfin uses admin credentials rather than an API key.



_Appears in:_
- [JellyfinSpec](#jellyfinspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url is the base URL of the Jellyfin instance (e.g., http://jellyfin.media.svc.cluster.local:8096). |  | MinLength: 1 <br />Pattern: `^https?://` <br />Required: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing a Jellyfin API key, from<br />the API Keys page of Jellyfin's dashboard. Once Jellyfin is set up, the<br />operator uses it instead of signing in as adminUser, and an appRef to this<br />resource passes it to apps that connect with a key, such as Maintainerr. |  | Optional: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | tls configures TLS settings for the connection.<br />Only needed when Jellyfin uses HTTPS with self-signed or private CA certificates. |  | Optional: \{\} <br /> |


#### JellyfinEncoding



JellyfinEncoding configures hardware-accelerated transcoding settings.



_Appears in:_
- [JellyfinSpec](#jellyfinspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `hardwareAccelerationType` _string_ | hardwareAccelerationType is the hardware acceleration method to use. |  | Enum: [none amf qsv nvenc v4l2m2m vaapi videotoolbox rkmpp] <br />Optional: \{\} <br /> |
| `vaapiDevice` _string_ | vaapiDevice is the VAAPI render device path (e.g., "/dev/dri/renderD128"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `enableHardwareEncoding` _boolean_ | enableHardwareEncoding enables hardware-accelerated encoding. |  | Optional: \{\} <br /> |
| `allowHevcEncoding` _boolean_ | allowHevcEncoding allows HEVC/H.265 hardware encoding. |  | Optional: \{\} <br /> |
| `enableTonemapping` _boolean_ | enableTonemapping enables HDR to SDR tone mapping. |  | Optional: \{\} <br /> |
| `enableVppTonemapping` _boolean_ | enableVppTonemapping enables VPP-based tone mapping (Intel QSV). |  | Optional: \{\} <br /> |
| `h264Crf` _integer_ | h264Crf is the CRF value for H.264 encoding. |  | Optional: \{\} <br /> |
| `h265Crf` _integer_ | h265Crf is the CRF value for H.265/HEVC encoding. |  | Optional: \{\} <br /> |
| `encoderPreset` _string_ | encoderPreset is the encoder speed preset (e.g., "auto", "fast", "medium", "slow"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `hardwareDecodingCodecs` _string array_ | hardwareDecodingCodecs is the list of codecs to decode in hardware. |  | Optional: \{\} <br /> |
| `enableIntelLowPowerH264HwEncoder` _boolean_ | enableIntelLowPowerH264HwEncoder enables the Intel low-power H.264 hardware encoder. |  | Optional: \{\} <br /> |
| `enableIntelLowPowerHevcHwEncoder` _boolean_ | enableIntelLowPowerHevcHwEncoder enables the Intel low-power HEVC hardware encoder. |  | Optional: \{\} <br /> |
| `enableSubtitleExtraction` _boolean_ | enableSubtitleExtraction enables extracting subtitles from media files. |  | Optional: \{\} <br /> |
| `enableThrottling` _boolean_ | enableThrottling enables transcoding throttling. |  | Optional: \{\} <br /> |
| `throttleDelaySeconds` _integer_ | throttleDelaySeconds is the delay in seconds before throttling begins. |  | Optional: \{\} <br /> |
| `qsvDevice` _string_ | qsvDevice is the Intel Quick Sync device, when there's more than one. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `encodingThreadCount` _integer_ | encodingThreadCount is how many threads ffmpeg uses. -1 picks one from the CPU count. |  | Minimum: -1 <br />Optional: \{\} <br /> |
| `enableDecodingColorDepth10Hevc` _boolean_ | enableDecodingColorDepth10Hevc decodes 10-bit HEVC in hardware. |  | Optional: \{\} <br /> |
| `enableDecodingColorDepth10Vp9` _boolean_ | enableDecodingColorDepth10Vp9 decodes 10-bit VP9 in hardware. |  | Optional: \{\} <br /> |
| `enableDecodingColorDepth10HevcRext` _boolean_ | enableDecodingColorDepth10HevcRext decodes 10-bit HEVC range extensions in hardware. |  | Optional: \{\} <br /> |
| `enableDecodingColorDepth12HevcRext` _boolean_ | enableDecodingColorDepth12HevcRext decodes 12-bit HEVC range extensions in hardware. |  | Optional: \{\} <br /> |
| `preferSystemNativeHwDecoder` _boolean_ | preferSystemNativeHwDecoder uses the operating system's hardware decoder<br />where there is one. |  | Optional: \{\} <br /> |
| `enableEnhancedNvdecDecoder` _boolean_ | enableEnhancedNvdecDecoder uses NVIDIA's own decoder instead of CUVID. |  | Optional: \{\} <br /> |
| `allowAv1Encoding` _boolean_ | allowAv1Encoding allows AV1 hardware encoding. |  | Optional: \{\} <br /> |
| `tonemappingAlgorithm` _string_ | tonemappingAlgorithm is the algorithm for HDR to SDR tone mapping. |  | Enum: [none clip linear gamma reinhard hable mobius bt2390] <br />Optional: \{\} <br /> |
| `tonemappingMode` _string_ | tonemappingMode is what tone mapping applies to. |  | Enum: [auto max rgb lum itp] <br />Optional: \{\} <br /> |
| `tonemappingRange` _string_ | tonemappingRange is the output color range of tone mapping. |  | Enum: [auto tv pc] <br />Optional: \{\} <br /> |
| `tonemappingDesat` _[Decimal](#decimal)_ | tonemappingDesat desaturates highlights brighter than this level. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |
| `tonemappingPeak` _[Decimal](#decimal)_ | tonemappingPeak is the signal peak to assume, in nits, when the video doesn't say. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |
| `tonemappingParam` _[Decimal](#decimal)_ | tonemappingParam tunes the tone mapping algorithm. 0 uses its default. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |
| `vppTonemappingBrightness` _[Decimal](#decimal)_ | vppTonemappingBrightness is the brightness gain of VPP tone mapping. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |
| `vppTonemappingContrast` _[Decimal](#decimal)_ | vppTonemappingContrast is the contrast gain of VPP tone mapping. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |
| `deinterlaceMethod` _string_ | deinterlaceMethod is the deinterlacing filter. |  | Enum: [yadif bwdif] <br />Optional: \{\} <br /> |
| `deinterlaceDoubleRate` _boolean_ | deinterlaceDoubleRate doubles the frame rate when deinterlacing. |  | Optional: \{\} <br /> |
| `enableSegmentDeletion` _boolean_ | enableSegmentDeletion deletes transcoded segments once clients have them. |  | Optional: \{\} <br /> |
| `segmentKeepSeconds` _integer_ | segmentKeepSeconds is how many seconds of segments enableSegmentDeletion keeps. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `enableAudioVbr` _boolean_ | enableAudioVbr encodes audio at a variable bitrate. |  | Optional: \{\} <br /> |
| `downMixAudioBoost` _[Decimal](#decimal)_ | downMixAudioBoost is the volume gain when downmixing to stereo. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |
| `downMixStereoAlgorithm` _string_ | downMixStereoAlgorithm is the algorithm for downmixing to stereo. |  | Enum: [None Dave750 NightmodeDialogue Rfc7845 Ac4] <br />Optional: \{\} <br /> |
| `maxMuxingQueueSize` _integer_ | maxMuxingQueueSize is the most packets ffmpeg buffers while muxing. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `subtitleExtractionTimeoutMinutes` _integer_ | subtitleExtractionTimeoutMinutes is how long subtitle extraction may take. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `allowOnDemandMetadataBasedKeyframeExtractionForExtensions` _string array_ | allowOnDemandMetadataBasedKeyframeExtractionForExtensions lists the file<br />extensions, such as mkv, whose key frames Jellyfin reads from metadata<br />when it needs them. |  | Optional: \{\} <br /> |


#### JellyfinLibrary



JellyfinLibrary defines a media library in Jellyfin.



_Appears in:_
- [JellyfinSpec](#jellyfinspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the library. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `collectionType` _string_ | collectionType is the type of media in this library. |  | Enum: [movies tvshows music musicvideos homevideos books mixed] <br />Required: \{\} <br /> |
| `paths` _string array_ | paths is the list of filesystem paths containing media for this library. |  | Required: \{\} <br /> |
| `enableRealtimeMonitor` _boolean_ | enableRealtimeMonitor enables real-time monitoring of library paths for changes. |  | Optional: \{\} <br /> |
| `enableTrickplayImageExtraction` _boolean_ | enableTrickplayImageExtraction enables generation of trickplay (scrubbing preview) images. |  | Optional: \{\} <br /> |
| `automaticRefreshIntervalDays` _integer_ | automaticRefreshIntervalDays is the number of days between automatic library scans. |  | Optional: \{\} <br /> |
| `preferredMetadataLanguage` _string_ | preferredMetadataLanguage overrides the server-level preferred metadata language for this library. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `metadataCountryCode` _string_ | metadataCountryCode overrides the server-level metadata country code for this library. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### JellyfinNetwork



JellyfinNetwork configures Jellyfin's network settings. Ports, the base URL,
HTTPS and bind addresses aren't here: changing them needs a restart and has
to match the Service and connection.url.



_Appears in:_
- [JellyfinSpec](#jellyfinspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enableRemoteAccess` _boolean_ | enableRemoteAccess allows connections from outside localNetworkSubnets.<br />With it off, localNetworkSubnets must include the operator's pod network. |  | Optional: \{\} <br /> |
| `localNetworkSubnets` _string array_ | localNetworkSubnets are the networks, in CIDR form, whose clients count as<br />local. Empty means the private address ranges. |  | Optional: \{\} <br /> |
| `knownProxies` _string array_ | knownProxies are the reverse proxies, by address or host name, whose<br />forwarded headers Jellyfin trusts for the client's address. |  | Optional: \{\} <br /> |
| `autoDiscovery` _boolean_ | autoDiscovery answers clients looking for servers on the local network. |  | Optional: \{\} <br /> |
| `enableUPnP` _boolean_ | enableUPnP forwards Jellyfin's public ports on the router. |  | Optional: \{\} <br /> |
| `ignoreVirtualInterfaces` _boolean_ | ignoreVirtualInterfaces leaves the interfaces named in<br />virtualInterfaceNames out of auto-detection. |  | Optional: \{\} <br /> |
| `virtualInterfaceNames` _string array_ | virtualInterfaceNames are interface name prefixes, such as veth, that<br />ignoreVirtualInterfaces skips. |  | Optional: \{\} <br /> |
| `enablePublishedServerUriByRequest` _boolean_ | enablePublishedServerUriByRequest tells clients the address they reached<br />Jellyfin on, instead of one Jellyfin works out. |  | Optional: \{\} <br /> |
| `publishedServerUriBySubnet` _string array_ | publishedServerUriBySubnet overrides the address Jellyfin gives clients,<br />as subnet=URL entries, such as all=https://jellyfin.example.com. |  | Optional: \{\} <br /> |
| `remoteIPFilter` _string array_ | remoteIPFilter lists remote addresses or subnets to allow, or to block when<br />isRemoteIPFilterBlacklist is on. |  | Optional: \{\} <br /> |
| `isRemoteIPFilterBlacklist` _boolean_ | isRemoteIPFilterBlacklist blocks the addresses in remoteIPFilter instead<br />of allowing only them. |  | Optional: \{\} <br /> |


#### JellyfinServer



JellyfinServer configures Jellyfin server settings.



_Appears in:_
- [JellyfinSpec](#jellyfinspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serverName` _string_ | serverName is the friendly name of the server. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `preferredMetadataLanguage` _string_ | preferredMetadataLanguage is the preferred language for metadata (e.g., "en"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `metadataCountryCode` _string_ | metadataCountryCode is the country code for metadata lookups (e.g., "US"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `logFileRetentionDays` _integer_ | logFileRetentionDays is the number of days to retain log files. |  | Optional: \{\} <br /> |
| `enableGroupingMoviesIntoCollections` _boolean_ | enableGroupingMoviesIntoCollections enables automatic grouping of movies into collections. |  | Optional: \{\} <br /> |
| `displaySpecialsWithinSeasons` _boolean_ | displaySpecialsWithinSeasons displays special episodes within their respective seasons. |  | Optional: \{\} <br /> |
| `libraryMonitorDelay` _integer_ | libraryMonitorDelay is the delay in seconds before processing library changes. |  | Optional: \{\} <br /> |
| `enableMetrics` _boolean_ | enableMetrics serves Prometheus metrics at /metrics. |  | Optional: \{\} <br /> |
| `quickConnectAvailable` _boolean_ | quickConnectAvailable lets users sign in on a new device with a code. |  | Optional: \{\} <br /> |
| `remoteClientBitrateLimit` _integer_ | remoteClientBitrateLimit caps the bitrate of each remote stream, in bits<br />per second. 0 means no limit. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `enableGroupingShowsIntoCollections` _boolean_ | enableGroupingShowsIntoCollections shows series in collections instead of<br />on their own. |  | Optional: \{\} <br /> |
| `enableFolderView` _boolean_ | enableFolderView shows a Folders view of the media folders. |  | Optional: \{\} <br /> |
| `enableExternalContentInSuggestions` _boolean_ | enableExternalContentInSuggestions includes internet trailers and live TV<br />in suggestions. |  | Optional: \{\} <br /> |
| `minResumePct` _integer_ | minResumePct is how far, in percent, playback must get before it can be resumed. |  | Maximum: 100 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `maxResumePct` _integer_ | maxResumePct is how far, in percent, playback must get to count as played. |  | Maximum: 100 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `minResumeDurationSeconds` _integer_ | minResumeDurationSeconds is the shortest item, in seconds, that can be resumed. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `minAudiobookResume` _integer_ | minAudiobookResume is how many minutes an audiobook must play before it<br />can be resumed. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `maxAudiobookResume` _integer_ | maxAudiobookResume is how many minutes from the end an audiobook counts as played. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `activityLogRetentionDays` _integer_ | activityLogRetentionDays is how many days the activity log keeps entries. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `libraryScanFanoutConcurrency` _integer_ | libraryScanFanoutConcurrency is how many library scan tasks run at once.<br />0 picks one from the CPU count. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `libraryMetadataRefreshConcurrency` _integer_ | libraryMetadataRefreshConcurrency is how many metadata refreshes run at<br />once. 0 picks one from the CPU count. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `parallelImageEncodingLimit` _integer_ | parallelImageEncodingLimit is how many images are encoded at once. 0<br />picks one from the CPU count. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `enableSlowResponseWarning` _boolean_ | enableSlowResponseWarning logs requests slower than slowResponseThresholdMs. |  | Optional: \{\} <br /> |
| `slowResponseThresholdMs` _integer_ | slowResponseThresholdMs is how many milliseconds a request takes before<br />it's logged as slow. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `imageSavingConvention` _string_ | imageSavingConvention is how Jellyfin names the artwork it saves with media. |  | Enum: [Legacy Compatible] <br />Optional: \{\} <br /> |
| `trickplayOptions` _[JellyfinTrickplay](#jellyfintrickplay)_ | trickplayOptions configures the preview thumbnails shown while seeking. |  | Optional: \{\} <br /> |


#### JellyfinSpec



JellyfinSpec defines the desired state of Jellyfin.



_Appears in:_
- [Jellyfin](#jellyfin)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[JellyfinConnection](#jellyfinconnection)_ | connection defines how to connect to the Jellyfin instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `adminUser` _[JellyfinAdminUser](#jellyfinadminuser)_ | adminUser defines the admin credentials used for initial setup and API access. |  | Required: \{\} <br /> |
| `server` _[JellyfinServer](#jellyfinserver)_ | server configures Jellyfin server settings. |  | Optional: \{\} <br /> |
| `encoding` _[JellyfinEncoding](#jellyfinencoding)_ | encoding configures hardware-accelerated transcoding settings. |  | Optional: \{\} <br /> |
| `network` _[JellyfinNetwork](#jellyfinnetwork)_ | network configures how Jellyfin tells local clients from remote ones and<br />which proxies it trusts. |  | Optional: \{\} <br /> |
| `libraries` _[JellyfinLibrary](#jellyfinlibrary) array_ | libraries defines media libraries, matched by name. An existing library<br />keeps the declared folders and options: folders it doesn't list are<br />removed. Libraries are never deleted, and a library's type can't change<br />in place. |  | Optional: \{\} <br /> |


#### JellyfinStatus



JellyfinStatus defines the observed state of Jellyfin.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Jellyfin](#jellyfin)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Jellyfin resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `initialized` _boolean_ | initialized indicates whether the Jellyfin setup wizard has been completed. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### JellyfinTrickplay



JellyfinTrickplay configures how Jellyfin makes trickplay thumbnails.



_Appears in:_
- [JellyfinServer](#jellyfinserver)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enableHwAcceleration` _boolean_ | enableHwAcceleration decodes video in hardware while making thumbnails. |  | Optional: \{\} <br /> |
| `enableHwEncoding` _boolean_ | enableHwEncoding encodes thumbnails in hardware. |  | Optional: \{\} <br /> |
| `enableKeyFrameOnlyExtraction` _boolean_ | enableKeyFrameOnlyExtraction takes thumbnails only at key frames, which is<br />faster and less precise. |  | Optional: \{\} <br /> |
| `scanBehavior` _string_ | scanBehavior decides whether a library scan waits for thumbnails. |  | Enum: [Blocking NonBlocking] <br />Optional: \{\} <br /> |
| `processPriority` _string_ | processPriority is the priority of the process making thumbnails. |  | Enum: [Idle BelowNormal Normal AboveNormal High] <br />Optional: \{\} <br /> |
| `interval` _integer_ | interval is the time between thumbnails, in milliseconds. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `widthResolutions` _integer array_ | widthResolutions are the thumbnail widths to make, in pixels. |  | Optional: \{\} <br /> |
| `tileWidth` _integer_ | tileWidth is how many thumbnails go across each tile image. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `tileHeight` _integer_ | tileHeight is how many thumbnails go down each tile image. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `qscale` _integer_ | qscale is the thumbnails' encoding quality, from 2 (best) to 31. |  | Maximum: 31 <br />Minimum: 2 <br />Optional: \{\} <br /> |
| `jpegQuality` _integer_ | jpegQuality is the JPEG quality of the tile images. |  | Maximum: 100 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `processThreads` _integer_ | processThreads is how many threads making thumbnails uses. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### Lidarr



Lidarr configures a Lidarr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Lidarr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[LidarrSpec](#lidarrspec)_ | spec is the desired state of the Lidarr. |  | Required: \{\} <br /> |
| `status` _[LidarrStatus](#lidarrstatus)_ | status is the observed state of the Lidarr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### LidarrDownloadClient



LidarrDownloadClient is one download client in a Lidarr instance.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `LidarrDownloadClient` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[LidarrDownloadClientSpec](#lidarrdownloadclientspec)_ | spec is the desired state of the LidarrDownloadClient. |  | Required: \{\} <br /> |
| `status` _[LidarrDownloadClientStatus](#lidarrdownloadclientstatus)_ | status is the observed state of the LidarrDownloadClient. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### LidarrDownloadClientConfig



LidarrDownloadClientConfig configures global download client behavior.



_Appears in:_
- [LidarrSpec](#lidarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enableCompletedDownloadHandling` _boolean_ | enableCompletedDownloadHandling auto-imports completed downloads. |  | Optional: \{\} <br /> |
| `autoRedownloadFailed` _boolean_ | autoRedownloadFailed automatically searches when a download fails. |  | Optional: \{\} <br /> |


#### LidarrDownloadClientSpec



LidarrDownloadClientSpec is one download client in a Lidarr instance.



_Appears in:_
- [LidarrDownloadClient](#lidarrdownloadclient)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `lidarrRef` _[LocalObjectReference](#localobjectreference)_ | lidarrRef names the Lidarr this download client belongs to, in the same namespace. |  | Required: \{\} <br /> |
| `deletionPolicy` _string_ | deletionPolicy controls what deleting this resource does in Lidarr:<br />"delete" removes the download client, "orphan" leaves it. One that existed before<br />the operator adopted it is never deleted. | delete | Enum: [delete orphan] <br />Optional: \{\} <br /> |
| `name` _string_ | name is the display name of the download client. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `appRef` _[AppRef](#appref)_ | appRef names the download client's resource, a QBittorrent or Sabnzbd,<br />whose kind and connection are used in place of implementation, protocol,<br />host, port, useSsl, urlBase and the credentials. |  | Optional: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this download client is active. A new download<br />client is enabled unless this is false. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the download protocol. |  | Enum: [torrent usenet] <br />Optional: \{\} <br /> |
| `implementation` _string_ | implementation is the download client type (e.g., QBittorrent, Sabnzbd, NzbGet, Transmission, Deluge). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `configContract` _string_ | configContract overrides the default config contract name.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `host` _string_ | host is the hostname or IP of the download client. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port of the download client. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `useSsl` _boolean_ | useSsl enables SSL/TLS for the download client connection. |  | Optional: \{\} <br /> |
| `urlBase` _string_ | urlBase is the base URL path for the download client (e.g., "/qbittorrent"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `category` _string_ | category is the download category to use. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the download client priority. Lower values are higher priority.<br />A new download client gets 1. |  | Maximum: 50 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `removeCompletedDownloads` _boolean_ | removeCompletedDownloads controls whether completed downloads are removed.<br />A new download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `removeFailedDownloads` _boolean_ | removeFailedDownloads controls whether failed downloads are removed. A new<br />download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the username for authentication. |  | Optional: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the password for authentication. |  | Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the API key (for Usenet clients like SABnzbd/NzbGet). |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines additional implementation-specific configuration fields.<br />Use these for settings not covered by first-class fields (e.g., initialState, sequentialOrder). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### LidarrDownloadClientStatus



LidarrDownloadClientStatus is the observed state of a LidarrDownloadClient.

_Validation:_
- MinProperties: 1

_Appears in:_
- [LidarrDownloadClient](#lidarrdownloadclient)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the download client. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `id` _integer_ | id is the download client's ID in Lidarr. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records what the operator created for this resource,<br />so deletion only removes what it created. |  | Optional: \{\} <br /> |


#### LidarrIndexer



LidarrIndexer is one indexer in a Lidarr instance.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `LidarrIndexer` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[LidarrIndexerSpec](#lidarrindexerspec)_ | spec is the desired state of the LidarrIndexer. |  | Required: \{\} <br /> |
| `status` _[LidarrIndexerStatus](#lidarrindexerstatus)_ | status is the observed state of the LidarrIndexer. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### LidarrIndexerConfig



LidarrIndexerConfig configures global indexer settings.



_Appears in:_
- [LidarrSpec](#lidarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minimumAge` _integer_ | minimumAge is the minimum age in minutes before downloading. |  | Optional: \{\} <br /> |
| `retention` _integer_ | retention is the usenet retention in days. |  | Optional: \{\} <br /> |
| `maximumSize` _integer_ | maximumSize is the maximum release size in MB. |  | Optional: \{\} <br /> |
| `rssSyncInterval` _integer_ | rssSyncInterval is the RSS sync interval in minutes. |  | Optional: \{\} <br /> |


#### LidarrIndexerSpec



LidarrIndexerSpec is one indexer in a Lidarr instance.



_Appears in:_
- [LidarrIndexer](#lidarrindexer)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `lidarrRef` _[LocalObjectReference](#localobjectreference)_ | lidarrRef names the Lidarr this indexer belongs to, in the same namespace. |  | Required: \{\} <br /> |
| `deletionPolicy` _string_ | deletionPolicy controls what deleting this resource does in Lidarr:<br />"delete" removes the indexer, "orphan" leaves it. One that existed before<br />the operator adopted it is never deleted. | delete | Enum: [delete orphan] <br />Optional: \{\} <br /> |
| `name` _string_ | name is the display name of the indexer. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this indexer is active. False turns off RSS,<br />automatic and interactive search; true turns on those not set below. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the indexer protocol. |  | Enum: [torrent usenet] <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the indexer type (e.g., Newznab, Torznab). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name for the implementation.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the indexer priority. Lower values are higher priority. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `enableRss` _boolean_ | enableRss enables RSS feed polling for this indexer. A new indexer<br />polls unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableAutomaticSearch` _boolean_ | enableAutomaticSearch enables automatic search for this indexer. A new<br />indexer searches unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableInteractiveSearch` _boolean_ | enableInteractiveSearch enables interactive (manual) search for this<br />indexer. A new indexer allows it unless this or enable is false. |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines the indexer's configuration fields (e.g., baseUrl, apiKey, categories). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### LidarrIndexerStatus



LidarrIndexerStatus is the observed state of a LidarrIndexer.

_Validation:_
- MinProperties: 1

_Appears in:_
- [LidarrIndexer](#lidarrindexer)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the indexer. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `id` _integer_ | id is the indexer's ID in Lidarr. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records what the operator created for this resource,<br />so deletion only removes what it created. |  | Optional: \{\} <br /> |


#### LidarrMediaManagement



LidarrMediaManagement configures how Lidarr handles media files.



_Appears in:_
- [LidarrSpec](#lidarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `copyUsingHardlinks` _boolean_ | copyUsingHardlinks uses hardlinks instead of copies when importing. |  | Optional: \{\} <br /> |
| `deleteEmptyFolders` _boolean_ | deleteEmptyFolders removes empty artist folders after cleanup. |  | Optional: \{\} <br /> |
| `importExtraFiles` _boolean_ | importExtraFiles imports additional files alongside media. |  | Optional: \{\} <br /> |
| `extraFileExtensions` _string_ | extraFileExtensions is a comma-separated list of extra file extensions to import. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `downloadPropersAndRepacks` _string_ | downloadPropersAndRepacks controls how proper/repack releases are handled. |  | Enum: [preferAndUpgrade doNotPrefer doNotUpgrade] <br />Optional: \{\} <br /> |
| `recycleBin` _string_ | recycleBin is the path to the recycling bin folder. An empty string<br />turns the recycling bin off. |  | Optional: \{\} <br /> |
| `recycleBinCleanupDays` _integer_ | recycleBinCleanupDays is how many days files stay in the recycling bin. |  | Optional: \{\} <br /> |
| `minimumFreeSpaceWhenImporting` _integer_ | minimumFreeSpaceWhenImporting is the minimum free space in MB required. |  | Optional: \{\} <br /> |
| `rescanAfterRefresh` _string_ | rescanAfterRefresh controls when to rescan artist folders. |  | Enum: [always afterManual never] <br />Optional: \{\} <br /> |
| `setPermissionsLinux` _boolean_ | setPermissionsLinux enables setting file permissions on import. |  | Optional: \{\} <br /> |
| `createEmptyArtistFolders` _boolean_ | createEmptyArtistFolders creates folders for artists with no files. |  | Optional: \{\} <br /> |


#### LidarrNaming



LidarrNaming configures artist and track naming conventions.



_Appears in:_
- [LidarrSpec](#lidarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `renameTracks` _boolean_ | renameTracks enables automatic track file renaming. |  | Optional: \{\} <br /> |
| `replaceIllegalCharacters` _boolean_ | replaceIllegalCharacters replaces illegal characters in filenames. |  | Optional: \{\} <br /> |
| `standardTrackFormat` _string_ | standardTrackFormat is the naming format for standard tracks. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `multiDiscTrackFormat` _string_ | multiDiscTrackFormat is the naming format for multi-disc tracks. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `artistFolderFormat` _string_ | artistFolderFormat is the naming format for artist folders. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### LidarrSpec



LidarrSpec is a Lidarr instance's connection and app-wide settings. Indexers
and download clients are resources of their own that refer to it.



_Appears in:_
- [Lidarr](#lidarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[AppConnection](#appconnection)_ | connection defines how to connect to the Lidarr instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `mediaManagement` _[LidarrMediaManagement](#lidarrmediamanagement)_ | mediaManagement configures media management settings. |  | Optional: \{\} <br /> |
| `naming` _[LidarrNaming](#lidarrnaming)_ | naming configures artist and track naming formats. |  | Optional: \{\} <br /> |
| `indexerConfig` _[LidarrIndexerConfig](#lidarrindexerconfig)_ | indexerConfig configures global indexer settings. |  | Optional: \{\} <br /> |
| `downloadClientConfig` _[LidarrDownloadClientConfig](#lidarrdownloadclientconfig)_ | downloadClientConfig configures global download client behavior. |  | Optional: \{\} <br /> |
| `ui` _[LidarrUI](#lidarrui)_ | ui configures UI display settings. |  | Optional: \{\} <br /> |
| `rootFolders` _[RootFolder](#rootfolder) array_ | rootFolders defines music library root folder paths. |  | Optional: \{\} <br /> |
| `remotePathMappings` _[RemotePathMapping](#remotepathmapping) array_ | remotePathMappings translates paths reported by remote download clients.<br />Entries are matched by host and remotePath. Only operator-created entries can be pruned. |  | Optional: \{\} <br /> |
| `qualityProfiles` _[QualityProfile](#qualityprofile) array_ | qualityProfiles defines quality profiles for controlling download quality preferences. |  | Optional: \{\} <br /> |
| `customFormats` _[CustomFormat](#customformat) array_ | customFormats defines custom format scoring rules for release selection. |  | Optional: \{\} <br /> |
| `qualityDefinitions` _[QualityDefinition](#qualitydefinition) array_ | qualityDefinitions sets the size limits of the app's qualities. The app<br />has a fixed set of qualities, so none is created or pruned, and qualities<br />left out keep their limits. |  | Optional: \{\} <br /> |
| `delayProfiles` _[DelayProfile](#delayprofile) array_ | delayProfiles defines delay profiles. The app checks tagged profiles in<br />the order listed, and the default profile, the one without tags, last.<br />Only operator-created profiles can be pruned, so the default never is. |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `tags` _[Tag](#tag) array_ | tags defines tags for linking resources together. |  | Optional: \{\} <br /> |
| `notifications` _[Notification](#notification) array_ | notifications defines notification/connection configurations. |  | Optional: \{\} <br /> |


#### LidarrStatus



LidarrStatus defines the observed state of Lidarr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Lidarr](#lidarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Lidarr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### LidarrUI



LidarrUI configures UI display settings.



_Appears in:_
- [LidarrSpec](#lidarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `firstDayOfWeek` _integer_ | firstDayOfWeek is the first day of the week. 0=Sunday, 1=Monday. |  | Optional: \{\} <br /> |
| `shortDateFormat` _string_ | shortDateFormat is the short date display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `longDateFormat` _string_ | longDateFormat is the long date display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `timeFormat` _string_ | timeFormat is the time display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `theme` _string_ | theme is the UI theme. |  | Enum: [auto dark light] <br />Optional: \{\} <br /> |
| `uiLanguage` _integer_ | uiLanguage is the UI language ID. |  | Optional: \{\} <br /> |


#### LocalObjectReference



LocalObjectReference names another resource of a known kind in the same
namespace.



_Appears in:_
- [LidarrDownloadClientSpec](#lidarrdownloadclientspec)
- [LidarrIndexerSpec](#lidarrindexerspec)
- [RadarrDownloadClientSpec](#radarrdownloadclientspec)
- [RadarrIndexerSpec](#radarrindexerspec)
- [ReadarrDownloadClientSpec](#readarrdownloadclientspec)
- [ReadarrIndexerSpec](#readarrindexerspec)
- [SonarrDownloadClientSpec](#sonarrdownloadclientspec)
- [SonarrIndexerSpec](#sonarrindexerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the referenced resource's name. |  | MinLength: 1 <br />Required: \{\} <br /> |


#### Maintainerr



Maintainerr configures a Maintainerr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Maintainerr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[MaintainerrSpec](#maintainerrspec)_ | spec is the desired state of the Maintainerr. |  | Required: \{\} <br /> |
| `status` _[MaintainerrStatus](#maintainerrstatus)_ | status is the observed state of the Maintainerr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MaintainerrArrServer



MaintainerrArrServer is a Sonarr or Radarr server in Maintainerr.



_Appears in:_
- [MaintainerrSpec](#maintainerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name identifies the server here and in ruleGroups[].sonarrServer and<br />ruleGroups[].radarrServer. It is the server name Maintainerr shows. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `appRef` _[AppRef](#appref)_ | appRef names the server's resource, a Sonarr in sonarr or a Radarr in<br />radarr, whose connection is used in place of url and apiKeySecretRef. |  | Optional: \{\} <br /> |
| `url` _string_ | url is the server URL as Maintainerr reaches it (e.g., http://radarr:7878). |  | MinLength: 1 <br />Pattern: `^https?://` <br />Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the server's API key. |  | Optional: \{\} <br /> |
| `tagExclusions` _boolean_ | tagExclusions excludes media carrying exclusionTag in this server from<br />every rule group. |  | Optional: \{\} <br /> |
| `exclusionTag` _string_ | exclusionTag is the tag tagExclusions looks for. Maintainerr uses "dnd"<br />when it is unset. |  | MinLength: 1 <br />Pattern: `^[a-z0-9-]+$` <br />Optional: \{\} <br /> |
| `untagOnUnexclude` _boolean_ | untagOnUnexclude removes exclusionTag from media when it is no longer<br />excluded in Maintainerr. |  | Optional: \{\} <br /> |


#### MaintainerrConnection



MaintainerrConnection defines how the operator reaches Maintainerr. Maintainerr
has no API authentication; restrict access to it at the network level.



_Appears in:_
- [MaintainerrSpec](#maintainerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url is the base URL of Maintainerr, including any BASE_PATH<br />(e.g., http://maintainerr:6246). |  | MinLength: 1 <br />Pattern: `^https?://` <br />Required: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | tls configures TLS settings for the connection.<br />Only needed when Maintainerr uses HTTPS with self-signed or private CA certificates. |  | Optional: \{\} <br /> |


#### MaintainerrCustomValue



MaintainerrCustomValue is a literal rule value.



_Appears in:_
- [MaintainerrRule](#maintainerrrule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | type is how value is read. custom_days is a number of days, which date<br />properties compare relative to now. date takes an ISO date. boolean takes<br />"true" or "false". text_list takes a JSON array or a single value. |  | Enum: [number custom_days date text text_list boolean] <br />Required: \{\} <br /> |
| `value` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#intorstring-intstr-util)_ | value is the literal. Quote booleans: value: "true". |  | Required: \{\} <br /> |


#### MaintainerrJellyfin



MaintainerrJellyfin connects Maintainerr to a Jellyfin server.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MaintainerrMediaServer](#maintainerrmediaserver)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `appRef` _[AppRef](#appref)_ | appRef names the Jellyfin resource whose connection URL and API key are<br />used in place of url and apiKeySecretRef. That resource must set its<br />connection's apiKeySecretRef. |  | Optional: \{\} <br /> |
| `url` _string_ | url is the Jellyfin URL as Maintainerr reaches it (e.g., http://jellyfin:8096). |  | MinLength: 1 <br />Pattern: `^https?://` <br />Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing a Jellyfin API key. |  | Optional: \{\} <br /> |
| `userId` _string_ | userId is the Jellyfin administrator Maintainerr acts as. Maintainerr<br />picks the first administrator when it is unset. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### MaintainerrJobs



MaintainerrJobs sets Maintainerr's job schedules as cron expressions.



_Appears in:_
- [MaintainerrSpec](#maintainerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `rules` _string_ | rules is when Maintainerr evaluates rule groups (e.g., "0 0-23/8 * * *"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `collections` _string_ | collections is when Maintainerr acts on media whose time in a collection<br />has run out (e.g., "0 0-23/12 * * *"). |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### MaintainerrMediaServer



MaintainerrMediaServer selects the media server Maintainerr manages.



_Appears in:_
- [MaintainerrSpec](#maintainerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `plex` _[MaintainerrPlex](#maintainerrplex)_ | plex connects Maintainerr to a Plex server. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `jellyfin` _[MaintainerrJellyfin](#maintainerrjellyfin)_ | jellyfin connects Maintainerr to a Jellyfin server. Maintainerr tests the<br />connection before saving it, so Jellyfin must be reachable from Maintainerr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MaintainerrNotification



MaintainerrNotification is one place Maintainerr sends notifications to.



_Appears in:_
- [MaintainerrSpec](#maintainerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name identifies the notification. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `agent` _string_ | agent is the service notifications go to. |  | Enum: [discord email gotify ntfy pushbullet pushover slack telegram webhook lunasea] <br />Required: \{\} <br /> |
| `enabled` _boolean_ | enabled turns the notification on. A new one is on unless this says otherwise. |  | Optional: \{\} <br /> |
| `types` _string array_ | types are the events that send a notification. |  | items:Enum: [mediaAddedToCollection mediaRemovedFromCollection mediaAboutToBeHandled mediaHandled ruleHandlingFailed collectionHandlingFailed overlayApplied overlayReverted updateAvailable] <br />Optional: \{\} <br /> |
| `aboutScale` _integer_ | aboutScale is how many days before media is handled mediaAboutToBeHandled<br />is sent. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `options` _[ConfigField](#configfield) array_ | options are the agent's settings, such as botUsername or jsonPayload,<br />each a value or read from a Secret, such as webhookUrl or authPass.<br />Values keep their type. |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### MaintainerrPlex



MaintainerrPlex connects Maintainerr to a Plex server. The operator turns on
Maintainerr's manual mode, so Maintainerr does not replace these values with
another address it discovers through plex.tv.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MaintainerrMediaServer](#maintainerrmediaserver)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `appRef` _[AppRef](#appref)_ | appRef names the Plex resource whose connection URL and token are used in<br />place of hostname, port, ssl and tokenSecretRef. |  | Optional: \{\} <br /> |
| `hostname` _string_ | hostname is the Plex host as Maintainerr reaches it, without a scheme<br />(e.g., plex.media.svc.cluster.local). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the Plex port. Defaults to 32400. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `ssl` _boolean_ | ssl connects to Plex over HTTPS. |  | Optional: \{\} <br /> |
| `tokenSecretRef` _[SecretKeyRef](#secretkeyref)_ | tokenSecretRef references a Secret containing a Plex token for an account<br />with access to the server. |  | Optional: \{\} <br /> |


#### MaintainerrRule



MaintainerrRule compares a property of the media with a value or with another
property. It uses the names Maintainerr's rule YAML export uses.



_Appears in:_
- [MaintainerrRuleSection](#maintainerrrulesection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `operator` _string_ | operator combines this rule with the rules before it in the section.<br />Required on every rule but a section's first, and not allowed on the first. |  | Enum: [AND OR] <br />Optional: \{\} <br /> |
| `firstValue` _string_ | firstValue is the property compared, as Application.property (e.g.,<br />Plex.addDate, Jellyfin.viewCount, Radarr.fileSize). Plex and Jellyfin<br />properties must belong to the configured media server. A name Maintainerr<br />lists twice resolves to the one not marked deprecated. |  | MinLength: 1 <br />Pattern: `^[A-Za-z]+\.[A-Za-z0-9_]+$` <br />Required: \{\} <br /> |
| `action` _string_ | action is the comparison. |  | Enum: [BIGGER SMALLER EQUALS NOT_EQUALS CONTAINS BEFORE AFTER IN_LAST IN_NEXT NOT_CONTAINS CONTAINS_PARTIAL NOT_CONTAINS_PARTIAL CONTAINS_ALL NOT_CONTAINS_ALL COUNT_EQUALS COUNT_NOT_EQUALS COUNT_BIGGER COUNT_SMALLER EXISTS NOT_EXISTS] <br />Required: \{\} <br /> |
| `lastValue` _string_ | lastValue is a second property to compare against, as Application.property.<br />Set lastValue or customValue, not both. |  | MinLength: 1 <br />Pattern: `^[A-Za-z]+\.[A-Za-z0-9_]+$` <br />Optional: \{\} <br /> |
| `customValue` _[MaintainerrCustomValue](#maintainerrcustomvalue)_ | customValue is a literal value to compare against. |  | Optional: \{\} <br /> |
| `arrDiskPath` _string_ | arrDiskPath limits disk-space properties to one Sonarr or Radarr root folder. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `username` _string_ | username is the media server user that per-user properties (viewCountByUser,<br />watchTimeByUser, lastViewedAtByUser) are read for. Required by those<br />properties and not allowed on others. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### MaintainerrRuleGroup



MaintainerrRuleGroup is a set of rules, the collection the matching media goes
into and what Maintainerr does with it.



_Appears in:_
- [MaintainerrSpec](#maintainerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name identifies the rule group and names its collection. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `description` _string_ | description is shown on the collection. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `library` _string_ | library is the title of the media server library the rules run against. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `dataType` _string_ | dataType is what a show library's rules match: whole shows, seasons or<br />episodes. Movie libraries always match movies. Changing it empties the<br />collection. |  | Enum: [movie show season episode] <br />Optional: \{\} <br /> |
| `active` _boolean_ | active runs the rule group. Inactive groups keep their collection but are<br />not evaluated or acted on. A new group is active unless this is false. |  | Optional: \{\} <br /> |
| `arrAction` _string_ | arrAction is what Maintainerr does with media once actionAfterDays have<br />passed. Without a Sonarr or Radarr server, deleting actions delete through<br />the media server. changeQualityProfile needs qualityProfile. |  | Enum: [delete unmonitorDeleteAll unmonitorDeleteExisting unmonitor doNothing deleteShowIfEmpty unmonitorShowIfEmpty changeQualityProfile] <br />Required: \{\} <br /> |
| `actionAfterDays` _integer_ | actionAfterDays is how long media stays in the collection before arrAction<br />runs. A new group without it fills its collection but never acts. Removing<br />it later leaves Maintainerr's value in place: Maintainerr cannot clear it. |  | Maximum: 36500 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `radarrServer` _string_ | radarrServer names the spec.radarr server to act through. Required when a<br />rule reads Radarr. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `sonarrServer` _string_ | sonarrServer names the spec.sonarr server to act through. Required when a<br />rule reads Sonarr. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `qualityProfile` _string_ | qualityProfile is the name of the Sonarr or Radarr quality profile that<br />changeQualityProfile switches media to. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `listExclusions` _boolean_ | listExclusions adds deleted media to the Sonarr or Radarr import list<br />exclusions, so lists do not add it again. |  | Optional: \{\} <br /> |
| `cleanupLeftoverFolders` _boolean_ | cleanupLeftoverFolders deletes folders a deleting action leaves behind.<br />Maintainerr supports it for unmonitorDeleteAll on movies and shows,<br />unmonitorDeleteExisting on shows and seasons, and delete or<br />deleteShowIfEmpty on seasons. |  | Optional: \{\} <br /> |
| `forceSeerr` _boolean_ | forceSeerr removes the media's requests from Seerr when it is acted on,<br />even if Seerr is not otherwise involved. Not supported for episodes. |  | Optional: \{\} <br /> |
| `tagInArr` _boolean_ | tagInArr tags collection members in Sonarr or Radarr. |  | Optional: \{\} <br /> |
| `keepInMaintainerrOnly` _boolean_ | keepInMaintainerrOnly keeps the collection in Maintainerr without creating<br />it on the media server. |  | Optional: \{\} <br /> |
| `visibleOnHome` _boolean_ | visibleOnHome shows the collection on the media server's home screen. |  | Optional: \{\} <br /> |
| `visibleOnRecommended` _boolean_ | visibleOnRecommended shows the collection in the library's recommendations. |  | Optional: \{\} <br /> |
| `schedule` _string_ | schedule is a cron expression for this group's rule runs. Unset, the group<br />runs on spec.jobs.rules. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `notifications` _string array_ | notifications names the spec.notifications, or notifications made in<br />Maintainerr, that report on this group. Unset, the group keeps the ones<br />it has. |  | Optional: \{\} <br /> |
| `sections` _[MaintainerrRuleSection](#maintainerrrulesection) array_ | sections are evaluated in order. Rules within a section combine with each<br />other first; each section then combines with the result of the ones before<br />it. |  | MinItems: 1 <br />Required: \{\} <br /> |


#### MaintainerrRuleSection



MaintainerrRuleSection is a group of rules evaluated together.



_Appears in:_
- [MaintainerrRuleGroup](#maintainerrrulegroup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `operator` _string_ | operator combines this section with the sections before it. Required on<br />every section but the first, and not allowed on the first. |  | Enum: [AND OR] <br />Optional: \{\} <br /> |
| `rules` _[MaintainerrRule](#maintainerrrule) array_ | rules are combined in order, each with the result of the ones before it. |  | MinItems: 1 <br />Required: \{\} <br /> |


#### MaintainerrSeerrConnection



MaintainerrSeerrConnection connects Maintainerr to Seerr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MaintainerrSpec](#maintainerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `appRef` _[AppRef](#appref)_ | appRef names the Seerr resource whose connection URL and API key are used<br />in place of url and apiKeySecretRef. That resource must set its<br />connection's apiKeySecretRef. |  | Optional: \{\} <br /> |
| `url` _string_ | url is Seerr's URL as Maintainerr reaches it. |  | MinLength: 1 <br />Pattern: `^https?://` <br />Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing Seerr's API key. |  | Optional: \{\} <br /> |


#### MaintainerrServiceConnection



MaintainerrServiceConnection connects Maintainerr to a service by URL and API key.



_Appears in:_
- [MaintainerrSpec](#maintainerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url is the service URL as Maintainerr reaches it. |  | MinLength: 1 <br />Pattern: `^https?://` <br />Required: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the service's API key. |  | Required: \{\} <br /> |


#### MaintainerrSpec



MaintainerrSpec defines the desired state of Maintainerr.



_Appears in:_
- [Maintainerr](#maintainerr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[MaintainerrConnection](#maintainerrconnection)_ | connection defines how to connect to the Maintainerr instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `mediaServer` _[MaintainerrMediaServer](#maintainerrmediaserver)_ | mediaServer connects Maintainerr to Plex or Jellyfin. The operator sets the<br />media server on a fresh Maintainerr only: switching an existing one deletes<br />every collection, so a different server already in use is reported as an<br />error instead. |  | Optional: \{\} <br /> |
| `sonarr` _[MaintainerrArrServer](#maintainerrarrserver) array_ | sonarr lists the Sonarr servers rule groups can act through. |  | Optional: \{\} <br /> |
| `radarr` _[MaintainerrArrServer](#maintainerrarrserver) array_ | radarr lists the Radarr servers rule groups can act through. |  | Optional: \{\} <br /> |
| `seerr` _[MaintainerrSeerrConnection](#maintainerrseerrconnection)_ | seerr connects Maintainerr to Seerr (or Overseerr or Jellyseerr), which<br />rules can read and which Maintainerr clears requests in. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `tautulli` _[MaintainerrServiceConnection](#maintainerrserviceconnection)_ | tautulli connects Maintainerr to Tautulli. Rules can read it only when<br />Plex is the media server. |  | Optional: \{\} <br /> |
| `jobs` _[MaintainerrJobs](#maintainerrjobs)_ | jobs sets when Maintainerr runs its rules and acts on its collections. |  | Optional: \{\} <br /> |
| `ruleGroups` _[MaintainerrRuleGroup](#maintainerrrulegroup) array_ | ruleGroups are the rules that put media into a collection and the action<br />Maintainerr takes on it. Each group is matched by name. |  | Optional: \{\} <br /> |
| `notifications` _[MaintainerrNotification](#maintainerrnotification) array_ | notifications defines where Maintainerr sends notifications, matched by<br />name. A notification reports only on the rule groups that name it. |  | Optional: \{\} <br /> |


#### MaintainerrStatus



MaintainerrStatus defines the observed state of Maintainerr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Maintainerr](#maintainerr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Maintainerr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### ManagedResource



ManagedResource lists the items of one type the operator created.



_Appears in:_
- [AutobrrStatus](#autobrrstatus)
- [BazarrStatus](#bazarrstatus)
- [FlareSolverrStatus](#flaresolverrstatus)
- [JellyfinStatus](#jellyfinstatus)
- [LidarrDownloadClientStatus](#lidarrdownloadclientstatus)
- [LidarrIndexerStatus](#lidarrindexerstatus)
- [LidarrStatus](#lidarrstatus)
- [MaintainerrStatus](#maintainerrstatus)
- [PlexStatus](#plexstatus)
- [ProwlarrStatus](#prowlarrstatus)
- [QBittorrentStatus](#qbittorrentstatus)
- [RadarrDownloadClientStatus](#radarrdownloadclientstatus)
- [RadarrIndexerStatus](#radarrindexerstatus)
- [RadarrStatus](#radarrstatus)
- [ReadarrDownloadClientStatus](#readarrdownloadclientstatus)
- [ReadarrIndexerStatus](#readarrindexerstatus)
- [ReadarrStatus](#readarrstatus)
- [SabnzbdStatus](#sabnzbdstatus)
- [SeerrStatus](#seerrstatus)
- [SonarrDownloadClientStatus](#sonarrdownloadclientstatus)
- [SonarrIndexerStatus](#sonarrindexerstatus)
- [SonarrStatus](#sonarrstatus)
- [TdarrStatus](#tdarrstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | type is the kind of item, such as downloadClients. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `names` _string array_ | names are the items of that type the operator created. |  | Required: \{\} <br /> |


#### Notification



Notification defines a notification (a "Connect" entry) in Sonarr, Radarr, Lidarr, Readarr or Prowlarr.



_Appears in:_
- [LidarrSpec](#lidarrspec)
- [ProwlarrSpec](#prowlarrspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the notification. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the notification type (e.g., Discord, Slack, Webhook, Email). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name for the implementation.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines the notification's configuration fields (e.g., webhook URL, API token). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |
| `onGrab` _boolean_ | onGrab fires when a release is grabbed (sent to download client). |  | Optional: \{\} <br /> |
| `onDownload` _boolean_ | onDownload fires when a download is imported. |  | Optional: \{\} <br /> |
| `onUpgrade` _boolean_ | onUpgrade fires when an existing file is upgraded. |  | Optional: \{\} <br /> |
| `onRename` _boolean_ | onRename fires when files are renamed. |  | Optional: \{\} <br /> |
| `onHealthIssue` _boolean_ | onHealthIssue fires when a health check fails. |  | Optional: \{\} <br /> |
| `onHealthRestored` _boolean_ | onHealthRestored fires when a health check recovers. |  | Optional: \{\} <br /> |
| `onApplicationUpdate` _boolean_ | onApplicationUpdate fires when the application is updated. |  | Optional: \{\} <br /> |
| `onManualInteractionRequired` _boolean_ | onManualInteractionRequired fires when manual interaction is needed. |  | Optional: \{\} <br /> |
| `includeHealthWarnings` _boolean_ | includeHealthWarnings includes health warnings (not just errors) in notifications. |  | Optional: \{\} <br /> |
| `onSeriesAdd` _boolean_ | onSeriesAdd fires when a new series is added (Sonarr). |  | Optional: \{\} <br /> |
| `onSeriesDelete` _boolean_ | onSeriesDelete fires when a series is deleted (Sonarr). |  | Optional: \{\} <br /> |
| `onEpisodeFileDelete` _boolean_ | onEpisodeFileDelete fires when an episode file is deleted (Sonarr). |  | Optional: \{\} <br /> |
| `onEpisodeFileDeleteForUpgrade` _boolean_ | onEpisodeFileDeleteForUpgrade fires when an episode file is deleted for upgrade (Sonarr). |  | Optional: \{\} <br /> |
| `onMovieAdded` _boolean_ | onMovieAdded fires when a new movie is added (Radarr). |  | Optional: \{\} <br /> |
| `onMovieDelete` _boolean_ | onMovieDelete fires when a movie is deleted (Radarr). |  | Optional: \{\} <br /> |
| `onMovieFileDelete` _boolean_ | onMovieFileDelete fires when a movie file is deleted (Radarr). |  | Optional: \{\} <br /> |
| `onMovieFileDeleteForUpgrade` _boolean_ | onMovieFileDeleteForUpgrade fires when a movie file is deleted for upgrade (Radarr). |  | Optional: \{\} <br /> |
| `onImportComplete` _boolean_ | onImportComplete fires when every file of a download has been imported (Sonarr). |  | Optional: \{\} <br /> |
| `onReleaseImport` _boolean_ | onReleaseImport fires when a release is imported (Lidarr, Readarr; Sonarr<br />and Radarr call this onDownload). |  | Optional: \{\} <br /> |
| `onDownloadFailure` _boolean_ | onDownloadFailure fires when a download fails (Lidarr, Readarr). |  | Optional: \{\} <br /> |
| `onImportFailure` _boolean_ | onImportFailure fires when an import fails (Lidarr, Readarr). |  | Optional: \{\} <br /> |
| `onArtistAdd` _boolean_ | onArtistAdd fires when an artist is added (Lidarr). |  | Optional: \{\} <br /> |
| `onArtistDelete` _boolean_ | onArtistDelete fires when an artist is deleted (Lidarr). |  | Optional: \{\} <br /> |
| `onAlbumDelete` _boolean_ | onAlbumDelete fires when an album is deleted (Lidarr). |  | Optional: \{\} <br /> |
| `onTrackRetag` _boolean_ | onTrackRetag fires when track tags are rewritten (Lidarr). |  | Optional: \{\} <br /> |
| `onAuthorAdded` _boolean_ | onAuthorAdded fires when an author is added (Readarr). |  | Optional: \{\} <br /> |
| `onAuthorDelete` _boolean_ | onAuthorDelete fires when an author is deleted (Readarr). |  | Optional: \{\} <br /> |
| `onBookDelete` _boolean_ | onBookDelete fires when a book is deleted (Readarr). |  | Optional: \{\} <br /> |
| `onBookFileDelete` _boolean_ | onBookFileDelete fires when a book file is deleted (Readarr). |  | Optional: \{\} <br /> |
| `onBookFileDeleteForUpgrade` _boolean_ | onBookFileDeleteForUpgrade fires when a book file is deleted for an upgrade (Readarr). |  | Optional: \{\} <br /> |
| `onBookRetag` _boolean_ | onBookRetag fires when book file tags are rewritten (Readarr). |  | Optional: \{\} <br /> |


#### Plex



Plex configures a Plex instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Plex` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[PlexSpec](#plexspec)_ | spec is the desired state of the Plex. |  | Required: \{\} <br /> |
| `status` _[PlexStatus](#plexstatus)_ | status is the observed state of the Plex. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### PlexConnection



PlexConnection defines how the operator connects to Plex.



_Appears in:_
- [PlexSpec](#plexspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url is the base URL of the Plex server (e.g., http://plex:32400). |  | MinLength: 1 <br />Pattern: `^https?://` <br />Required: \{\} <br /> |
| `tokenSecretRef` _[SecretKeyRef](#secretkeyref)_ | tokenSecretRef references a Secret containing the X-Plex-Token. |  | Required: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | tls configures TLS settings for the connection.<br />Only needed when Plex uses HTTPS with self-signed or private CA certificates. |  | Optional: \{\} <br /> |


#### PlexLibrary



PlexLibrary defines a Plex media library.



_Appears in:_
- [PlexSpec](#plexspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the library. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `type` _string_ | type is the library type. |  | Enum: [movie show artist photo] <br />Required: \{\} <br /> |
| `paths` _string array_ | paths is the list of media folder paths. |  | Required: \{\} <br /> |
| `language` _string_ | language is the library language (ISO 639-1). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `scanner` _string_ | scanner is the scanner type. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `agent` _string_ | agent is the metadata agent to use. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### PlexNetwork



PlexNetwork configures Plex network settings.



_Appears in:_
- [PlexSpec](#plexspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secureConnections` _string_ | secureConnections controls whether clients must, should or can't use<br />secure connections. |  | Enum: [required preferred disabled] <br />Optional: \{\} <br /> |
| `customServerAccessUrls` _string_ | customServerAccessUrls is a comma-separated list of custom access URLs. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `allowedNetworks` _string_ | allowedNetworks is a comma-separated list of networks allowed without auth. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `enableIPv6` _boolean_ | enableIPv6 enables IPv6 support. |  | Optional: \{\} <br /> |
| `manualPortMappingMode` _boolean_ | manualPortMappingMode uses manualPortMappingPort as the public port for<br />remote access instead of finding one. |  | Optional: \{\} <br /> |
| `manualPortMappingPort` _integer_ | manualPortMappingPort is the public port for remote access. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `relayEnabled` _boolean_ | relayEnabled lets clients that can't connect directly go through Plex's relay. |  | Optional: \{\} <br /> |
| `gdmEnabled` _boolean_ | gdmEnabled announces the server to clients on the local network. |  | Optional: \{\} <br /> |
| `webhooksEnabled` _boolean_ | webhooksEnabled sends events to the webhooks set up in the Plex account. |  | Optional: \{\} <br /> |
| `preferredNetworkInterface` _string_ | preferredNetworkInterface is the interface Plex uses, such as eth0. An<br />empty string means any. |  | Optional: \{\} <br /> |


#### PlexServer



PlexServer configures Plex server settings.



_Appears in:_
- [PlexSpec](#plexspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `friendlyName` _string_ | friendlyName is the server's display name. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `enableRemoteAccess` _boolean_ | enableRemoteAccess allows access from outside the local network. |  | Optional: \{\} <br /> |
| `logDebug` _boolean_ | logDebug enables debug logging. |  | Optional: \{\} <br /> |
| `autoEmptyTrash` _boolean_ | autoEmptyTrash automatically empties the trash. |  | Optional: \{\} <br /> |
| `scanMyLibraryAutomatically` _boolean_ | scanMyLibraryAutomatically enables automatic library scanning. |  | Optional: \{\} <br /> |
| `scanMyLibraryPeriodically` _boolean_ | scanMyLibraryPeriodically enables periodic library scanning. |  | Optional: \{\} <br /> |
| `scanMyLibraryPartially` _boolean_ | scanMyLibraryPartially scans only the folders that changed when changes<br />are detected. |  | Optional: \{\} <br /> |
| `libraryScanInterval` _integer_ | libraryScanInterval is how often, in seconds, scanMyLibraryPeriodically scans. |  | Enum: [900 1800 3600 7200 21600 43200 86400] <br />Optional: \{\} <br /> |
| `scannerLowPriority` _boolean_ | scannerLowPriority runs scans at a lower priority. |  | Optional: \{\} <br /> |
| `allowMediaDeletion` _boolean_ | allowMediaDeletion lets the server owner delete media from Plex apps. |  | Optional: \{\} <br /> |
| `videoPlayedThreshold` _integer_ | videoPlayedThreshold is how far, in percent, a video must play to count as watched. |  | Enum: [70 75 80 85 90 95] <br />Optional: \{\} <br /> |
| `generateVideoPreviewThumbnails` _string_ | generateVideoPreviewThumbnails is when Plex makes the thumbnails shown<br />while seeking: never, as a scheduled task, or also as media is added. |  | Enum: [never scheduled asap] <br />Optional: \{\} <br /> |
| `generateChapterThumbnails` _string_ | generateChapterThumbnails is when Plex makes chapter thumbnails. |  | Enum: [never scheduled asap] <br />Optional: \{\} <br /> |
| `loudnessAnalysis` _string_ | loudnessAnalysis is when Plex analyzes audio loudness. |  | Enum: [never scheduled asap] <br />Optional: \{\} <br /> |
| `butlerStartHour` _integer_ | butlerStartHour is the hour scheduled maintenance tasks may start. |  | Maximum: 23 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `butlerEndHour` _integer_ | butlerEndHour is the hour scheduled maintenance tasks must stop. |  | Maximum: 23 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `butlerTaskBackupDatabase` _boolean_ | butlerTaskBackupDatabase backs up the database every three days. |  | Optional: \{\} <br /> |
| `butlerDatabaseBackupPath` _string_ | butlerDatabaseBackupPath is where database backups go. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `butlerTaskOptimizeDatabase` _boolean_ | butlerTaskOptimizeDatabase optimizes the database every week. |  | Optional: \{\} <br /> |
| `butlerTaskCleanOldBundles` _boolean_ | butlerTaskCleanOldBundles removes old metadata bundles every week. |  | Optional: \{\} <br /> |
| `butlerTaskCleanOldCacheFiles` _boolean_ | butlerTaskCleanOldCacheFiles removes old cache files every week. |  | Optional: \{\} <br /> |
| `butlerTaskRefreshLocalMedia` _boolean_ | butlerTaskRefreshLocalMedia refreshes local metadata every three days. |  | Optional: \{\} <br /> |
| `butlerTaskRefreshLibraries` _boolean_ | butlerTaskRefreshLibraries scans the libraries during maintenance. |  | Optional: \{\} <br /> |
| `butlerTaskUpgradeMediaAnalysis` _boolean_ | butlerTaskUpgradeMediaAnalysis upgrades media analysis during maintenance. |  | Optional: \{\} <br /> |
| `butlerTaskRefreshPeriodicMetadata` _boolean_ | butlerTaskRefreshPeriodicMetadata refreshes metadata periodically. |  | Optional: \{\} <br /> |
| `butlerTaskDeepMediaAnalysis` _boolean_ | butlerTaskDeepMediaAnalysis analyzes media in depth during maintenance. |  | Optional: \{\} <br /> |
| `dlnaEnabled` _boolean_ | dlnaEnabled runs the DLNA server. |  | Optional: \{\} <br /> |
| `sendCrashReports` _boolean_ | sendCrashReports sends crash reports to Plex. |  | Optional: \{\} <br /> |
| `logVerbose` _boolean_ | logVerbose enables verbose logging. |  | Optional: \{\} <br /> |


#### PlexSpec



PlexSpec defines the desired state of Plex.



_Appears in:_
- [Plex](#plex)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[PlexConnection](#plexconnection)_ | connection defines how to connect to the Plex instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `server` _[PlexServer](#plexserver)_ | server configures Plex server settings. |  | Optional: \{\} <br /> |
| `transcoder` _[PlexTranscoder](#plextranscoder)_ | transcoder configures transcoding and hardware acceleration. |  | Optional: \{\} <br /> |
| `libraries` _[PlexLibrary](#plexlibrary) array_ | libraries defines media libraries, matched by name. An existing library<br />keeps the declared agent, scanner, language and folders: folders it<br />doesn't list are removed. Libraries are never deleted, and a library's<br />type can't change in place. |  | Optional: \{\} <br /> |
| `network` _[PlexNetwork](#plexnetwork)_ | network configures network settings. |  | Optional: \{\} <br /> |


#### PlexStatus



PlexStatus defines the observed state of Plex.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Plex](#plex)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Plex resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### PlexTranscoder



PlexTranscoder configures transcoding settings.



_Appears in:_
- [PlexSpec](#plexspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `transcodeHwRequested` _boolean_ | transcodeHwRequested enables hardware-accelerated transcoding. |  | Optional: \{\} <br /> |
| `hardwareAccelerationType` _string_ | hardwareAccelerationType sets the HW acceleration type. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `maxSimultaneousVideoTranscodes` _integer_ | maxSimultaneousVideoTranscodes limits concurrent transcodes. 0 = unlimited. |  | Optional: \{\} <br /> |
| `transcodeHwEncodingEnabled` _boolean_ | transcodeHwEncodingEnabled enables hardware encoding. |  | Optional: \{\} <br /> |
| `transcoderTempDirectory` _string_ | transcoderTempDirectory sets the transcoder temporary directory. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `transcoderQuality` _string_ | transcoderQuality trades encoding speed for quality. maximumQuality is<br />what Plex calls "make my CPU hurt". |  | Enum: [automatic preferSpeed preferQuality maximumQuality] <br />Optional: \{\} <br /> |
| `transcoderThrottleBuffer` _integer_ | transcoderThrottleBuffer is how many seconds ahead of playback the<br />transcoder works before slowing down. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `transcoderH264BackgroundPreset` _string_ | transcoderH264BackgroundPreset is the encoder preset for background<br />transcodes such as optimizing and downloads. |  | Enum: [ultrafast superfast veryfast faster fast medium slow slower veryslow] <br />Optional: \{\} <br /> |
| `transcoderHevcEncoding` _boolean_ | transcoderHevcEncoding allows HEVC encoding. |  | Optional: \{\} <br /> |
| `downloadsTempDirectory` _string_ | downloadsTempDirectory is where transcodes for downloads are kept. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### Prowlarr



Prowlarr configures a Prowlarr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Prowlarr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[ProwlarrSpec](#prowlarrspec)_ | spec is the desired state of the Prowlarr. |  | Required: \{\} <br /> |
| `status` _[ProwlarrStatus](#prowlarrstatus)_ | status is the observed state of the Prowlarr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### ProwlarrAppProfile



ProwlarrAppProfile configures which searches an indexer supports.



_Appears in:_
- [ProwlarrSpec](#prowlarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name uniquely identifies the sync profile. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enableRss` _boolean_ | enableRss enables RSS sync. |  | Optional: \{\} <br /> |
| `enableAutomaticSearch` _boolean_ | enableAutomaticSearch enables automatic searches. |  | Optional: \{\} <br /> |
| `enableInteractiveSearch` _boolean_ | enableInteractiveSearch enables interactive searches. |  | Optional: \{\} <br /> |
| `minimumSeeders` _integer_ | minimumSeeders is the minimum number of seeders for torrent results. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### ProwlarrApplication



ProwlarrApplication defines an application connection managed by Prowlarr.



_Appears in:_
- [ProwlarrSpec](#prowlarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the application. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `syncLevel` _string_ | syncLevel controls how indexers are synced to this application. |  | Enum: [disabled addOnly fullSync] <br />Optional: \{\} <br /> |
| `appRef` _[AppRef](#appref)_ | appRef names the application's resource, a Sonarr, Radarr, Lidarr or<br />Readarr, whose kind and connection are used in place of implementation,<br />baseUrl and apiKeySecretRef. prowlarrUrl then defaults to this resource's<br />connection URL. |  | Optional: \{\} <br /> |
| `implementation` _string_ | implementation is the application type (e.g., Sonarr, Radarr). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name for the implementation.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `prowlarrUrl` _string_ | prowlarrUrl is the URL Prowlarr uses to reach itself (for callbacks). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `baseUrl` _string_ | baseUrl is the base URL of the application. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the application's API key. |  | Optional: \{\} <br /> |
| `syncCategories` _integer array_ | syncCategories is the list of category IDs to sync to this application. |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against Prowlarr on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |


#### ProwlarrDownloadClient



ProwlarrDownloadClient defines a download client for Prowlarr manual grabs.



_Appears in:_
- [ProwlarrSpec](#prowlarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the download client. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this download client is active. A new download<br />client is enabled unless this is false. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the download protocol. |  | Enum: [torrent usenet] <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the download client type (e.g., QBittorrent, Sabnzbd). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the download client priority. Lower values are higher priority. |  | Optional: \{\} <br /> |
| `categories` _[ProwlarrDownloadClientCategory](#prowlarrdownloadclientcategory) array_ | categories defines the category mappings for this download client. |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against Prowlarr on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines the download client's configuration fields (e.g., host, port, apiKey). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### ProwlarrDownloadClientCategory



ProwlarrDownloadClientCategory maps Prowlarr categories to download client categories.



_Appears in:_
- [ProwlarrDownloadClient](#prowlarrdownloadclient)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clientCategory` _string_ | clientCategory is the category name in the download client. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `categories` _integer array_ | categories is the list of Prowlarr category IDs to map. |  | Optional: \{\} <br /> |


#### ProwlarrIndexer



ProwlarrIndexer defines an indexer configuration in Prowlarr.



_Appears in:_
- [ProwlarrSpec](#prowlarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the indexer. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the indexer type (e.g., Newznab, Torznab). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this indexer is active. A new indexer is enabled<br />unless this is false. |  | Optional: \{\} <br /> |
| `appProfileId` _integer_ | appProfileId is the application profile ID for this indexer. |  | Optional: \{\} <br /> |
| `appProfileName` _string_ | appProfileName resolves the current sync profile ID by name on each reconciliation.<br />Takes precedence over appProfileId. Missing or ambiguous names block indexer writes. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the indexer priority. Lower values are higher priority. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines configuration fields for the indexer. |  | MaxProperties: 2 <br />Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against Prowlarr on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |


#### ProwlarrProxy



ProwlarrProxy defines a proxy configuration for Prowlarr.



_Appears in:_
- [ProwlarrSpec](#prowlarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the proxy. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the proxy type (e.g., Http, Socks4, Socks5). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name for the implementation. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `host` _string_ | host is the hostname or IP of the proxy. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `requestTimeout` _integer_ | requestTimeout is the request timeout in seconds. |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against Prowlarr on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |


#### ProwlarrSpec



ProwlarrSpec defines the desired state of Prowlarr.



_Appears in:_
- [Prowlarr](#prowlarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[AppConnection](#appconnection)_ | connection defines how to connect to the Prowlarr instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `applications` _[ProwlarrApplication](#prowlarrapplication) array_ | applications defines application connections managed by Prowlarr. |  | Optional: \{\} <br /> |
| `appProfiles` _[ProwlarrAppProfile](#prowlarrappprofile) array_ | appProfiles defines indexer sync profiles, applied before indexers.<br />Profiles are not pruned because indexers outside this CR may reference them. |  | Optional: \{\} <br /> |
| `indexers` _[ProwlarrIndexer](#prowlarrindexer) array_ | indexers defines indexer configurations managed by Prowlarr. |  | Optional: \{\} <br /> |
| `proxies` _[ProwlarrProxy](#prowlarrproxy) array_ | proxies defines proxy configurations for Prowlarr. |  | Optional: \{\} <br /> |
| `tags` _[Tag](#tag) array_ | tags defines tags for linking resources together. |  | Optional: \{\} <br /> |
| `downloadClients` _[ProwlarrDownloadClient](#prowlarrdownloadclient) array_ | downloadClients defines download client connections for manual grabs. |  | Optional: \{\} <br /> |
| `notifications` _[Notification](#notification) array_ | notifications defines notification/connection configurations. |  | Optional: \{\} <br /> |


#### ProwlarrStatus



ProwlarrStatus defines the observed state of Prowlarr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Prowlarr](#prowlarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Prowlarr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### QBittorrent



QBittorrent configures a QBittorrent instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `QBittorrent` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[QBittorrentSpec](#qbittorrentspec)_ | spec is the desired state of the QBittorrent. |  | Required: \{\} <br /> |
| `status` _[QBittorrentStatus](#qbittorrentstatus)_ | status is the observed state of the QBittorrent. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### QBittorrentCategory



QBittorrentCategory defines a download category with its save path.



_Appears in:_
- [QBittorrentSpec](#qbittorrentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the category name. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `savePath` _string_ | savePath is the save path for this category. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### QBittorrentConnection



QBittorrentConnection defines how the operator connects to qBittorrent.
Unlike AppConnection, qBittorrent uses username/password authentication.



_Appears in:_
- [QBittorrentSpec](#qbittorrentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url is the base URL of the qBittorrent Web UI (e.g., http://qbittorrent.media.svc.cluster.local:8080). |  | MinLength: 1 <br />Pattern: `^https?://` <br />Required: \{\} <br /> |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the username. |  | Required: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the password. |  | Required: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | tls configures TLS settings for the connection.<br />Only needed when qBittorrent uses HTTPS with self-signed or private CA certificates. |  | Optional: \{\} <br /> |


#### QBittorrentPreferences



QBittorrentPreferences configures qBittorrent application preferences.



_Appears in:_
- [QBittorrentSpec](#qbittorrentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `savePath` _string_ | savePath is the default save path for downloads. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `tempPathEnabled` _boolean_ | tempPathEnabled enables using a temporary path for incomplete downloads. |  | Optional: \{\} <br /> |
| `dht` _boolean_ | dht enables the DHT (Distributed Hash Table) network. |  | Optional: \{\} <br /> |
| `pex` _boolean_ | pex enables Peer Exchange. |  | Optional: \{\} <br /> |
| `lsd` _boolean_ | lsd enables Local Service Discovery. |  | Optional: \{\} <br /> |
| `encryption` _string_ | encryption controls whether peers may, must or must not encrypt connections. |  | Enum: [allow require disable] <br />Optional: \{\} <br /> |
| `maxConnec` _integer_ | maxConnec is the global maximum number of connections. -1 means unlimited. |  | Optional: \{\} <br /> |
| `maxConnecPerTorrent` _integer_ | maxConnecPerTorrent is the maximum connections per torrent. -1 means unlimited. |  | Optional: \{\} <br /> |
| `maxUploads` _integer_ | maxUploads is the global maximum number of upload slots. -1 means unlimited. |  | Optional: \{\} <br /> |
| `maxUploadsPerTorrent` _integer_ | maxUploadsPerTorrent is the maximum upload slots per torrent. -1 means unlimited. |  | Optional: \{\} <br /> |
| `maxRatioEnabled` _boolean_ | maxRatioEnabled enables the global share ratio limit. |  | Optional: \{\} <br /> |
| `maxRatio` _string_ | maxRatio is the global share ratio limit (e.g., "2.0").<br />Represented as a string to preserve decimal precision in JSON. It applies<br />while maxRatioEnabled is true, here or in qBittorrent. |  | Optional: \{\} <br /> |
| `maxRatioAction` _string_ | maxRatioAction is what qBittorrent does when a share limit is reached. |  | Enum: [stop remove enableSuperSeeding removeWithContent] <br />Optional: \{\} <br /> |
| `maxSeedingTimeEnabled` _boolean_ | maxSeedingTimeEnabled enables the global seeding time limit. |  | Optional: \{\} <br /> |
| `maxSeedingTime` _integer_ | maxSeedingTime is the global seeding time limit in minutes. It applies<br />while maxSeedingTimeEnabled is true, here or in qBittorrent. |  | Optional: \{\} <br /> |
| `preallocateAll` _boolean_ | preallocateAll enables disk space pre-allocation for all files. |  | Optional: \{\} <br /> |
| `locale` _string_ | locale is the UI locale (e.g., "en"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `webUiPort` _integer_ | webUiPort is the Web UI listening port. |  | Optional: \{\} <br /> |
| `bypassAuthSubnetWhitelistEnabled` _boolean_ | bypassAuthSubnetWhitelistEnabled lets clients in the whitelisted subnets skip<br />the WebUI login, so an authenticating proxy does not produce a second prompt. |  | Optional: \{\} <br /> |
| `bypassAuthSubnetWhitelist` _string_ | bypassAuthSubnetWhitelist is the comma-separated list of subnets allowed to<br />skip the WebUI login. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `tempPath` _string_ | tempPath is where incomplete downloads go while tempPathEnabled is on. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `incompleteFilesExt` _boolean_ | incompleteFilesExt adds .!qB to the names of files still downloading. |  | Optional: \{\} <br /> |
| `autoTmmEnabled` _boolean_ | autoTmmEnabled puts new torrents in automatic management, which saves<br />them under their category's path. |  | Optional: \{\} <br /> |
| `torrentChangedTmmEnabled` _boolean_ | torrentChangedTmmEnabled moves a torrent in automatic management when its<br />category changes. When off, the torrent switches to manual management. |  | Optional: \{\} <br /> |
| `savePathChangedTmmEnabled` _boolean_ | savePathChangedTmmEnabled moves torrents in automatic management when<br />savePath changes. When off, they switch to manual management. |  | Optional: \{\} <br /> |
| `categoryChangedTmmEnabled` _boolean_ | categoryChangedTmmEnabled moves a category's torrents in automatic<br />management when its save path changes. When off, they switch to manual<br />management. |  | Optional: \{\} <br /> |
| `torrentContentLayout` _string_ | torrentContentLayout decides whether a torrent's files go in a folder of<br />their own. |  | Enum: [Original Subfolder NoSubfolder] <br />Optional: \{\} <br /> |
| `addStoppedEnabled` _boolean_ | addStoppedEnabled adds new torrents stopped. |  | Optional: \{\} <br /> |
| `torrentStopCondition` _string_ | torrentStopCondition stops a new torrent once it gets this far. |  | Enum: [None MetadataReceived FilesChecked] <br />Optional: \{\} <br /> |
| `excludedFileNamesEnabled` _boolean_ | excludedFileNamesEnabled skips files whose names match excludedFileNames. |  | Optional: \{\} <br /> |
| `excludedFileNames` _string array_ | excludedFileNames are file name patterns to skip, such as *.lnk. |  | Optional: \{\} <br /> |
| `dlLimit` _integer_ | dlLimit is the global download speed limit in KiB/s. 0 means unlimited. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `upLimit` _integer_ | upLimit is the global upload speed limit in KiB/s. 0 means unlimited. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `altDlLimit` _integer_ | altDlLimit is the download speed limit in KiB/s while the alternative<br />limits are on. 0 means unlimited. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `altUpLimit` _integer_ | altUpLimit is the upload speed limit in KiB/s while the alternative limits<br />are on. 0 means unlimited. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `schedulerEnabled` _boolean_ | schedulerEnabled turns the alternative limits on and off on a schedule. |  | Optional: \{\} <br /> |
| `scheduleFromHour` _integer_ | scheduleFromHour is the hour the alternative limits start. |  | Maximum: 23 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `scheduleFromMin` _integer_ | scheduleFromMin is the minute the alternative limits start. |  | Maximum: 59 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `scheduleToHour` _integer_ | scheduleToHour is the hour the alternative limits end. |  | Maximum: 23 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `scheduleToMin` _integer_ | scheduleToMin is the minute the alternative limits end. |  | Maximum: 59 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `schedulerDays` _string_ | schedulerDays is the days the schedule runs. |  | Enum: [everyDay weekdays weekends monday tuesday wednesday thursday friday saturday sunday] <br />Optional: \{\} <br /> |
| `limitUtpRate` _boolean_ | limitUtpRate applies the speed limits to µTP connections. |  | Optional: \{\} <br /> |
| `limitTcpOverhead` _boolean_ | limitTcpOverhead counts transport overhead against the speed limits. |  | Optional: \{\} <br /> |
| `limitLanPeers` _boolean_ | limitLanPeers applies the speed limits to peers on the local network. |  | Optional: \{\} <br /> |
| `queueingEnabled` _boolean_ | queueingEnabled limits how many torrents are active at once. |  | Optional: \{\} <br /> |
| `maxActiveDownloads` _integer_ | maxActiveDownloads is how many torrents can download at once. -1 means unlimited. |  | Optional: \{\} <br /> |
| `maxActiveUploads` _integer_ | maxActiveUploads is how many torrents can seed at once. -1 means unlimited. |  | Optional: \{\} <br /> |
| `maxActiveTorrents` _integer_ | maxActiveTorrents is how many torrents can be active at once. -1 means unlimited. |  | Optional: \{\} <br /> |
| `dontCountSlowTorrents` _boolean_ | dontCountSlowTorrents leaves slow torrents out of the active limits. |  | Optional: \{\} <br /> |
| `slowTorrentDlRateThreshold` _integer_ | slowTorrentDlRateThreshold is the download speed in KiB/s below which a<br />torrent counts as slow. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `slowTorrentUlRateThreshold` _integer_ | slowTorrentUlRateThreshold is the upload speed in KiB/s below which a<br />torrent counts as slow. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `slowTorrentInactiveTimer` _integer_ | slowTorrentInactiveTimer is how many seconds a torrent stays under the<br />thresholds before it counts as slow. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `listenPort` _integer_ | listenPort is the port for incoming connections. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `randomPort` _boolean_ | randomPort picks a different listening port on each start. |  | Optional: \{\} <br /> |
| `upnp` _boolean_ | upnp forwards the listening port on the router with UPnP or NAT-PMP. |  | Optional: \{\} <br /> |
| `networkInterface` _string_ | networkInterface binds connections to one network interface, such as a<br />VPN's tun0. Empty means any interface. |  | Optional: \{\} <br /> |
| `networkInterfaceAddress` _string_ | networkInterfaceAddress binds connections to one address of<br />networkInterface. Empty means all of them. |  | Optional: \{\} <br /> |
| `anonymousMode` _boolean_ | anonymousMode hides the client's identity from peers and trackers. |  | Optional: \{\} <br /> |
| `maxInactiveSeedingTimeEnabled` _boolean_ | maxInactiveSeedingTimeEnabled enables the global limit on seeding time<br />without uploads. |  | Optional: \{\} <br /> |
| `maxInactiveSeedingTime` _integer_ | maxInactiveSeedingTime is the global inactive seeding time limit in<br />minutes. It applies while maxInactiveSeedingTimeEnabled is true, here or<br />in qBittorrent. |  | Optional: \{\} <br /> |


#### QBittorrentSpec



QBittorrentSpec defines the desired state of QBittorrent.



_Appears in:_
- [QBittorrent](#qbittorrent)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[QBittorrentConnection](#qbittorrentconnection)_ | connection defines how to connect to the qBittorrent instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `preferences` _[QBittorrentPreferences](#qbittorrentpreferences)_ | preferences configures qBittorrent application preferences. |  | Optional: \{\} <br /> |
| `categories` _[QBittorrentCategory](#qbittorrentcategory) array_ | categories defines download categories and their save paths. |  | Optional: \{\} <br /> |


#### QBittorrentStatus



QBittorrentStatus defines the observed state of QBittorrent.

_Validation:_
- MinProperties: 1

_Appears in:_
- [QBittorrent](#qbittorrent)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the QBittorrent resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### QualityDefinition



QualityDefinition sets the size limits of one of the app's qualities. Sizes
use the units of the app's quality settings: megabytes per minute of runtime
in Sonarr and Radarr, kilobits per second in Lidarr.



_Appears in:_
- [LidarrSpec](#lidarrspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `quality` _string_ | quality is the quality's name in the app, such as WEBDL-1080p. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `title` _string_ | title is the name the app shows for the quality. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `minSize` _[Decimal](#decimal)_ | minSize is the smallest release the app grabs at this quality. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |
| `maxSize` _[Decimal](#decimal)_ | maxSize is the largest release the app grabs at this quality. 0 means unlimited. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |
| `preferredSize` _[Decimal](#decimal)_ | preferredSize is the size the app prefers among acceptable releases. 0<br />means unlimited, so the largest release wins. Readarr has no preferred size. |  | Minimum: 0 <br />Type: number <br />Optional: \{\} <br /> |


#### QualityProfile



QualityProfile defines a quality profile that controls which qualities are acceptable
and when upgrades should occur.



_Appears in:_
- [LidarrSpec](#lidarrspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the quality profile. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `upgradeAllowed` _boolean_ | upgradeAllowed controls whether upgrades to better qualities are allowed. |  | Optional: \{\} <br /> |
| `cutoff` _string_ | cutoff is the quality or group name at which quality upgrades stop.<br />Must match a quality name or group name in the items list. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `minFormatScore` _integer_ | minFormatScore is the minimum custom format score required to grab a release. |  | Optional: \{\} <br /> |
| `cutoffFormatScore` _integer_ | cutoffFormatScore is the custom format score at which upgrades stop. |  | Optional: \{\} <br /> |
| `minUpgradeFormatScore` _integer_ | minUpgradeFormatScore is the minimum score improvement required for a format-based upgrade. |  | Optional: \{\} <br /> |
| `items` _[QualityProfileItem](#qualityprofileitem) array_ | items defines the quality ordering and allowed states.<br />Items at the end of the list are higher priority. Each item is either<br />an individual quality (with quality set) or a quality group (with name and items set). |  | ExactlyOneOf: [name quality] <br />Optional: \{\} <br /> |
| `formatItems` _[QualityProfileFormatItem](#qualityprofileformatitem) array_ | formatItems assigns custom format scores within this profile.<br />Reference custom formats by name; IDs are resolved automatically. |  | Optional: \{\} <br /> |


#### QualityProfileFormatItem



QualityProfileFormatItem assigns a score to a custom format within a quality profile.



_Appears in:_
- [QualityProfile](#qualityprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the custom format name. Must match an existing custom format. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `score` _integer_ | score is the score to assign. Positive values prefer, negative values penalize. |  | Required: \{\} <br /> |


#### QualityProfileGroupItem



QualityProfileGroupItem represents an individual quality within a quality group.



_Appears in:_
- [QualityProfileItem](#qualityprofileitem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `quality` _[QualityReference](#qualityreference)_ | quality identifies the quality level. |  | Required: \{\} <br /> |
| `allowed` _boolean_ | allowed controls whether this quality is enabled. |  | Optional: \{\} <br /> |


#### QualityProfileItem



QualityProfileItem represents a quality or quality group in a profile.
Set quality for an individual quality, or set name+items for a group.

_Validation:_
- ExactlyOneOf: [name quality]

_Appears in:_
- [QualityProfile](#qualityprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the group name. Set only for quality groups, not individual qualities. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `quality` _[QualityReference](#qualityreference)_ | quality identifies a specific quality level.<br />Set for individual qualities, omit for groups. |  | Optional: \{\} <br /> |
| `allowed` _boolean_ | allowed controls whether this quality or group is enabled in the profile. |  | Optional: \{\} <br /> |
| `items` _[QualityProfileGroupItem](#qualityprofilegroupitem) array_ | items contains the child qualities when this is a group.<br />Only set for quality groups. |  | Optional: \{\} <br /> |


#### QualityReference



QualityReference identifies a specific quality level by ID and name.



_Appears in:_
- [QualityProfileGroupItem](#qualityprofilegroupitem)
- [QualityProfileItem](#qualityprofileitem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _integer_ | id is the quality ID (e.g., 7 for Bluray-1080p). IDs are app-specific,<br />and 0 is Unknown. |  | Minimum: 0 <br />Required: \{\} <br /> |
| `name` _string_ | name is the quality display name (e.g., "Bluray-1080p"). |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### Radarr



Radarr configures a Radarr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Radarr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[RadarrSpec](#radarrspec)_ | spec is the desired state of the Radarr. |  | Required: \{\} <br /> |
| `status` _[RadarrStatus](#radarrstatus)_ | status is the observed state of the Radarr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### RadarrDownloadClient



RadarrDownloadClient is one download client in a Radarr instance.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `RadarrDownloadClient` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[RadarrDownloadClientSpec](#radarrdownloadclientspec)_ | spec is the desired state of the RadarrDownloadClient. |  | Required: \{\} <br /> |
| `status` _[RadarrDownloadClientStatus](#radarrdownloadclientstatus)_ | status is the observed state of the RadarrDownloadClient. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### RadarrDownloadClientConfig



RadarrDownloadClientConfig configures global download client behavior.



_Appears in:_
- [RadarrSpec](#radarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enableCompletedDownloadHandling` _boolean_ | enableCompletedDownloadHandling auto-imports completed downloads. |  | Optional: \{\} <br /> |
| `autoRedownloadFailed` _boolean_ | autoRedownloadFailed automatically searches when a download fails. |  | Optional: \{\} <br /> |
| `autoRedownloadFailedFromInteractiveSearch` _boolean_ | autoRedownloadFailedFromInteractiveSearch auto-searches failed interactive searches. |  | Optional: \{\} <br /> |
| `checkForFinishedDownloadInterval` _integer_ | checkForFinishedDownloadInterval is the interval in minutes to check for finished downloads. |  | Optional: \{\} <br /> |


#### RadarrDownloadClientSpec



RadarrDownloadClientSpec is one download client in a Radarr instance.



_Appears in:_
- [RadarrDownloadClient](#radarrdownloadclient)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `radarrRef` _[LocalObjectReference](#localobjectreference)_ | radarrRef names the Radarr this download client belongs to, in the same namespace. |  | Required: \{\} <br /> |
| `deletionPolicy` _string_ | deletionPolicy controls what deleting this resource does in Radarr:<br />"delete" removes the download client, "orphan" leaves it. One that existed before<br />the operator adopted it is never deleted. | delete | Enum: [delete orphan] <br />Optional: \{\} <br /> |
| `name` _string_ | name is the display name of the download client. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `appRef` _[AppRef](#appref)_ | appRef names the download client's resource, a QBittorrent or Sabnzbd,<br />whose kind and connection are used in place of implementation, protocol,<br />host, port, useSsl, urlBase and the credentials. |  | Optional: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this download client is active. A new download<br />client is enabled unless this is false. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the download protocol. |  | Enum: [torrent usenet] <br />Optional: \{\} <br /> |
| `implementation` _string_ | implementation is the download client type (e.g., QBittorrent, Sabnzbd, NzbGet, Transmission, Deluge). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `configContract` _string_ | configContract overrides the default config contract name.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `host` _string_ | host is the hostname or IP of the download client. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port of the download client. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `useSsl` _boolean_ | useSsl enables SSL/TLS for the download client connection. |  | Optional: \{\} <br /> |
| `urlBase` _string_ | urlBase is the base URL path for the download client (e.g., "/qbittorrent"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `category` _string_ | category is the download category to use. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the download client priority. Lower values are higher priority.<br />A new download client gets 1. |  | Maximum: 50 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `removeCompletedDownloads` _boolean_ | removeCompletedDownloads controls whether completed downloads are removed.<br />A new download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `removeFailedDownloads` _boolean_ | removeFailedDownloads controls whether failed downloads are removed. A new<br />download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the username for authentication. |  | Optional: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the password for authentication. |  | Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the API key (for Usenet clients like SABnzbd/NzbGet). |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines additional implementation-specific configuration fields.<br />Use these for settings not covered by first-class fields (e.g., initialState, sequentialOrder). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### RadarrDownloadClientStatus



RadarrDownloadClientStatus is the observed state of a RadarrDownloadClient.

_Validation:_
- MinProperties: 1

_Appears in:_
- [RadarrDownloadClient](#radarrdownloadclient)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the download client. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `id` _integer_ | id is the download client's ID in Radarr. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records what the operator created for this resource,<br />so deletion only removes what it created. |  | Optional: \{\} <br /> |


#### RadarrIndexer



RadarrIndexer is one indexer in a Radarr instance.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `RadarrIndexer` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[RadarrIndexerSpec](#radarrindexerspec)_ | spec is the desired state of the RadarrIndexer. |  | Required: \{\} <br /> |
| `status` _[RadarrIndexerStatus](#radarrindexerstatus)_ | status is the observed state of the RadarrIndexer. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### RadarrIndexerConfig



RadarrIndexerConfig configures global indexer settings.



_Appears in:_
- [RadarrSpec](#radarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minimumAge` _integer_ | minimumAge is the minimum age in minutes before downloading. |  | Optional: \{\} <br /> |
| `retention` _integer_ | retention is the usenet retention in days. 0 means unlimited. |  | Optional: \{\} <br /> |
| `maximumSize` _integer_ | maximumSize is the maximum release size in MB. 0 means unlimited. |  | Optional: \{\} <br /> |
| `rssSyncInterval` _integer_ | rssSyncInterval is the RSS sync interval in minutes. 0 disables RSS. |  | Optional: \{\} <br /> |
| `preferIndexerFlags` _boolean_ | preferIndexerFlags prefers releases with indexer flags. |  | Optional: \{\} <br /> |
| `availabilityDelay` _integer_ | availabilityDelay is the number of days to wait after a movie is available before searching. |  | Optional: \{\} <br /> |
| `allowHardcodedSubs` _boolean_ | allowHardcodedSubs allows importing releases with hardcoded subtitles. |  | Optional: \{\} <br /> |
| `whitelistedHardcodedSubs` _string_ | whitelistedHardcodedSubs is a comma-separated list of subtitle languages to allow when hardcoded. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### RadarrIndexerSpec



RadarrIndexerSpec is one indexer in a Radarr instance.



_Appears in:_
- [RadarrIndexer](#radarrindexer)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `radarrRef` _[LocalObjectReference](#localobjectreference)_ | radarrRef names the Radarr this indexer belongs to, in the same namespace. |  | Required: \{\} <br /> |
| `deletionPolicy` _string_ | deletionPolicy controls what deleting this resource does in Radarr:<br />"delete" removes the indexer, "orphan" leaves it. One that existed before<br />the operator adopted it is never deleted. | delete | Enum: [delete orphan] <br />Optional: \{\} <br /> |
| `name` _string_ | name is the display name of the indexer. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this indexer is active. False turns off RSS,<br />automatic and interactive search; true turns on those not set below. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the indexer protocol. |  | Enum: [torrent usenet] <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the indexer type (e.g., Newznab, Torznab). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name for the implementation.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the indexer priority. Lower values are higher priority. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `enableRss` _boolean_ | enableRss enables RSS feed polling for this indexer. A new indexer<br />polls unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableAutomaticSearch` _boolean_ | enableAutomaticSearch enables automatic search for this indexer. A new<br />indexer searches unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableInteractiveSearch` _boolean_ | enableInteractiveSearch enables interactive (manual) search for this<br />indexer. A new indexer allows it unless this or enable is false. |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines the indexer's configuration fields (e.g., baseUrl, apiKey, categories). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### RadarrIndexerStatus



RadarrIndexerStatus is the observed state of a RadarrIndexer.

_Validation:_
- MinProperties: 1

_Appears in:_
- [RadarrIndexer](#radarrindexer)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the indexer. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `id` _integer_ | id is the indexer's ID in Radarr. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records what the operator created for this resource,<br />so deletion only removes what it created. |  | Optional: \{\} <br /> |


#### RadarrMediaManagement



RadarrMediaManagement configures how Radarr handles media files.



_Appears in:_
- [RadarrSpec](#radarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `autoUnmonitorPreviouslyDownloadedMovies` _boolean_ | autoUnmonitorPreviouslyDownloadedMovies unmonitors movies that have been downloaded. |  | Optional: \{\} <br /> |
| `createEmptyMovieFolders` _boolean_ | createEmptyMovieFolders creates folders for movies with no files. |  | Optional: \{\} <br /> |
| `autoRenameFolders` _boolean_ | autoRenameFolders automatically renames movie folders when the movie title changes. |  | Optional: \{\} <br /> |
| `pathsDefaultStatic` _boolean_ | pathsDefaultStatic prevents Radarr from updating paths when the root folder changes. |  | Optional: \{\} <br /> |
| `copyUsingHardlinks` _boolean_ | copyUsingHardlinks uses hardlinks instead of copies when importing. |  | Optional: \{\} <br /> |
| `deleteEmptyFolders` _boolean_ | deleteEmptyFolders removes empty movie folders after cleanup. |  | Optional: \{\} <br /> |
| `importExtraFiles` _boolean_ | importExtraFiles imports additional files (subtitles, etc.) alongside media. |  | Optional: \{\} <br /> |
| `extraFileExtensions` _string_ | extraFileExtensions is a comma-separated list of extra file extensions to import. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `downloadPropersAndRepacks` _string_ | downloadPropersAndRepacks controls how proper/repack releases are handled. |  | Enum: [preferAndUpgrade doNotPrefer doNotUpgrade] <br />Optional: \{\} <br /> |
| `recycleBin` _string_ | recycleBin is the path to the recycling bin folder. An empty string<br />turns the recycling bin off. |  | Optional: \{\} <br /> |
| `recycleBinCleanupDays` _integer_ | recycleBinCleanupDays is how many days files stay in the recycling bin. |  | Optional: \{\} <br /> |
| `minimumFreeSpaceWhenImporting` _integer_ | minimumFreeSpaceWhenImporting is the minimum free space in MB required to import. |  | Optional: \{\} <br /> |
| `enableMediaInfo` _boolean_ | enableMediaInfo enables media info scanning. |  | Optional: \{\} <br /> |
| `rescanAfterRefresh` _string_ | rescanAfterRefresh controls when to rescan movie folders. |  | Enum: [always afterManual never] <br />Optional: \{\} <br /> |
| `setPermissionsLinux` _boolean_ | setPermissionsLinux enables setting file permissions on import. |  | Optional: \{\} <br /> |
| `skipFreeSpaceCheckWhenImporting` _boolean_ | skipFreeSpaceCheckWhenImporting skips free space check during import. |  | Optional: \{\} <br /> |


#### RadarrNaming



RadarrNaming configures movie and folder naming conventions.



_Appears in:_
- [RadarrSpec](#radarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `renameMovies` _boolean_ | renameMovies enables automatic movie file renaming. |  | Optional: \{\} <br /> |
| `replaceIllegalCharacters` _boolean_ | replaceIllegalCharacters replaces illegal characters in filenames. |  | Optional: \{\} <br /> |
| `colonReplacementFormat` _string_ | colonReplacementFormat defines how colons are replaced in filenames. |  | Enum: [delete dash spaceDash spaceDashSpace smart] <br />Optional: \{\} <br /> |
| `standardMovieFormat` _string_ | standardMovieFormat is the naming format for movies. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `movieFolderFormat` _string_ | movieFolderFormat is the naming format for movie folders. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### RadarrSpec



RadarrSpec is a Radarr instance's connection and app-wide settings. Indexers
and download clients are resources of their own that refer to it.



_Appears in:_
- [Radarr](#radarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[AppConnection](#appconnection)_ | connection defines how to connect to the Radarr instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `mediaManagement` _[RadarrMediaManagement](#radarrmediamanagement)_ | mediaManagement configures media management settings. |  | Optional: \{\} <br /> |
| `naming` _[RadarrNaming](#radarrnaming)_ | naming configures movie and folder naming formats. |  | Optional: \{\} <br /> |
| `indexerConfig` _[RadarrIndexerConfig](#radarrindexerconfig)_ | indexerConfig configures global indexer settings. |  | Optional: \{\} <br /> |
| `downloadClientConfig` _[RadarrDownloadClientConfig](#radarrdownloadclientconfig)_ | downloadClientConfig configures global download client behavior. |  | Optional: \{\} <br /> |
| `ui` _[RadarrUI](#radarrui)_ | ui configures UI display settings. |  | Optional: \{\} <br /> |
| `rootFolders` _[RootFolder](#rootfolder) array_ | rootFolders defines media library root folder paths. |  | Optional: \{\} <br /> |
| `remotePathMappings` _[RemotePathMapping](#remotepathmapping) array_ | remotePathMappings translates paths reported by remote download clients.<br />Entries are matched by host and remotePath. Only operator-created entries can be pruned. |  | Optional: \{\} <br /> |
| `qualityProfiles` _[QualityProfile](#qualityprofile) array_ | qualityProfiles defines quality profiles for controlling download quality preferences. |  | Optional: \{\} <br /> |
| `customFormats` _[CustomFormat](#customformat) array_ | customFormats defines custom format scoring rules for release selection. |  | Optional: \{\} <br /> |
| `qualityDefinitions` _[QualityDefinition](#qualitydefinition) array_ | qualityDefinitions sets the size limits of the app's qualities. The app<br />has a fixed set of qualities, so none is created or pruned, and qualities<br />left out keep their limits. |  | Optional: \{\} <br /> |
| `delayProfiles` _[DelayProfile](#delayprofile) array_ | delayProfiles defines delay profiles. The app checks tagged profiles in<br />the order listed, and the default profile, the one without tags, last.<br />Only operator-created profiles can be pruned, so the default never is. |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `tags` _[Tag](#tag) array_ | tags defines tags for linking resources together. |  | Optional: \{\} <br /> |
| `notifications` _[Notification](#notification) array_ | notifications defines notification/connection configurations. |  | Optional: \{\} <br /> |


#### RadarrStatus



RadarrStatus defines the observed state of Radarr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Radarr](#radarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Radarr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### RadarrUI



RadarrUI configures UI display settings.



_Appears in:_
- [RadarrSpec](#radarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `firstDayOfWeek` _integer_ | firstDayOfWeek is the first day of the week. 0=Sunday, 1=Monday. |  | Maximum: 1 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `shortDateFormat` _string_ | shortDateFormat is the short date display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `longDateFormat` _string_ | longDateFormat is the long date display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `timeFormat` _string_ | timeFormat is the time display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `showRelativeDates` _boolean_ | showRelativeDates shows relative dates (today, yesterday). |  | Optional: \{\} <br /> |
| `theme` _string_ | theme is the UI theme. |  | Enum: [auto dark light] <br />Optional: \{\} <br /> |
| `enableColorImpairedMode` _boolean_ | enableColorImpairedMode enables color-impaired accessibility mode. |  | Optional: \{\} <br /> |
| `uiLanguage` _integer_ | uiLanguage is the UI language ID. 1=English. |  | Optional: \{\} <br /> |
| `movieRuntimeFormat` _string_ | movieRuntimeFormat controls how movie runtimes are displayed. |  | Enum: [hoursMinutes minutes] <br />Optional: \{\} <br /> |
| `movieInfoLanguage` _integer_ | movieInfoLanguage is the language ID for movie information. -1=Original. |  | Optional: \{\} <br /> |


#### Readarr



Readarr configures a Readarr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Readarr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[ReadarrSpec](#readarrspec)_ | spec is the desired state of the Readarr. |  | Required: \{\} <br /> |
| `status` _[ReadarrStatus](#readarrstatus)_ | status is the observed state of the Readarr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### ReadarrDownloadClient



ReadarrDownloadClient is one download client in a Readarr instance.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `ReadarrDownloadClient` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[ReadarrDownloadClientSpec](#readarrdownloadclientspec)_ | spec is the desired state of the ReadarrDownloadClient. |  | Required: \{\} <br /> |
| `status` _[ReadarrDownloadClientStatus](#readarrdownloadclientstatus)_ | status is the observed state of the ReadarrDownloadClient. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### ReadarrDownloadClientConfig



ReadarrDownloadClientConfig configures global download client behavior.



_Appears in:_
- [ReadarrSpec](#readarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enableCompletedDownloadHandling` _boolean_ | enableCompletedDownloadHandling auto-imports completed downloads. |  | Optional: \{\} <br /> |
| `autoRedownloadFailed` _boolean_ | autoRedownloadFailed automatically searches when a download fails. |  | Optional: \{\} <br /> |


#### ReadarrDownloadClientSpec



ReadarrDownloadClientSpec is one download client in a Readarr instance.



_Appears in:_
- [ReadarrDownloadClient](#readarrdownloadclient)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `readarrRef` _[LocalObjectReference](#localobjectreference)_ | readarrRef names the Readarr this download client belongs to, in the same namespace. |  | Required: \{\} <br /> |
| `deletionPolicy` _string_ | deletionPolicy controls what deleting this resource does in Readarr:<br />"delete" removes the download client, "orphan" leaves it. One that existed before<br />the operator adopted it is never deleted. | delete | Enum: [delete orphan] <br />Optional: \{\} <br /> |
| `name` _string_ | name is the display name of the download client. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `appRef` _[AppRef](#appref)_ | appRef names the download client's resource, a QBittorrent or Sabnzbd,<br />whose kind and connection are used in place of implementation, protocol,<br />host, port, useSsl, urlBase and the credentials. |  | Optional: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this download client is active. A new download<br />client is enabled unless this is false. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the download protocol. |  | Enum: [torrent usenet] <br />Optional: \{\} <br /> |
| `implementation` _string_ | implementation is the download client type (e.g., QBittorrent, Sabnzbd, NzbGet, Transmission, Deluge). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `configContract` _string_ | configContract overrides the default config contract name.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `host` _string_ | host is the hostname or IP of the download client. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port of the download client. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `useSsl` _boolean_ | useSsl enables SSL/TLS for the download client connection. |  | Optional: \{\} <br /> |
| `urlBase` _string_ | urlBase is the base URL path for the download client (e.g., "/qbittorrent"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `category` _string_ | category is the download category to use. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the download client priority. Lower values are higher priority.<br />A new download client gets 1. |  | Maximum: 50 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `removeCompletedDownloads` _boolean_ | removeCompletedDownloads controls whether completed downloads are removed.<br />A new download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `removeFailedDownloads` _boolean_ | removeFailedDownloads controls whether failed downloads are removed. A new<br />download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the username for authentication. |  | Optional: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the password for authentication. |  | Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the API key (for Usenet clients like SABnzbd/NzbGet). |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines additional implementation-specific configuration fields.<br />Use these for settings not covered by first-class fields (e.g., initialState, sequentialOrder). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### ReadarrDownloadClientStatus



ReadarrDownloadClientStatus is the observed state of a ReadarrDownloadClient.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ReadarrDownloadClient](#readarrdownloadclient)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the download client. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `id` _integer_ | id is the download client's ID in Readarr. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records what the operator created for this resource,<br />so deletion only removes what it created. |  | Optional: \{\} <br /> |


#### ReadarrIndexer



ReadarrIndexer is one indexer in a Readarr instance.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `ReadarrIndexer` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[ReadarrIndexerSpec](#readarrindexerspec)_ | spec is the desired state of the ReadarrIndexer. |  | Required: \{\} <br /> |
| `status` _[ReadarrIndexerStatus](#readarrindexerstatus)_ | status is the observed state of the ReadarrIndexer. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### ReadarrIndexerConfig



ReadarrIndexerConfig configures global indexer settings.



_Appears in:_
- [ReadarrSpec](#readarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minimumAge` _integer_ | minimumAge is the minimum age in minutes before downloading. |  | Optional: \{\} <br /> |
| `retention` _integer_ | retention is the usenet retention in days. |  | Optional: \{\} <br /> |
| `maximumSize` _integer_ | maximumSize is the maximum release size in MB. |  | Optional: \{\} <br /> |
| `rssSyncInterval` _integer_ | rssSyncInterval is the RSS sync interval in minutes. |  | Optional: \{\} <br /> |


#### ReadarrIndexerSpec



ReadarrIndexerSpec is one indexer in a Readarr instance.



_Appears in:_
- [ReadarrIndexer](#readarrindexer)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `readarrRef` _[LocalObjectReference](#localobjectreference)_ | readarrRef names the Readarr this indexer belongs to, in the same namespace. |  | Required: \{\} <br /> |
| `deletionPolicy` _string_ | deletionPolicy controls what deleting this resource does in Readarr:<br />"delete" removes the indexer, "orphan" leaves it. One that existed before<br />the operator adopted it is never deleted. | delete | Enum: [delete orphan] <br />Optional: \{\} <br /> |
| `name` _string_ | name is the display name of the indexer. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this indexer is active. False turns off RSS,<br />automatic and interactive search; true turns on those not set below. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the indexer protocol. |  | Enum: [torrent usenet] <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the indexer type (e.g., Newznab, Torznab). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name for the implementation.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the indexer priority. Lower values are higher priority. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `enableRss` _boolean_ | enableRss enables RSS feed polling for this indexer. A new indexer<br />polls unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableAutomaticSearch` _boolean_ | enableAutomaticSearch enables automatic search for this indexer. A new<br />indexer searches unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableInteractiveSearch` _boolean_ | enableInteractiveSearch enables interactive (manual) search for this<br />indexer. A new indexer allows it unless this or enable is false. |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines the indexer's configuration fields (e.g., baseUrl, apiKey, categories). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### ReadarrIndexerStatus



ReadarrIndexerStatus is the observed state of a ReadarrIndexer.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ReadarrIndexer](#readarrindexer)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the indexer. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `id` _integer_ | id is the indexer's ID in Readarr. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records what the operator created for this resource,<br />so deletion only removes what it created. |  | Optional: \{\} <br /> |


#### ReadarrMediaManagement



ReadarrMediaManagement configures how Readarr handles media files.



_Appears in:_
- [ReadarrSpec](#readarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `copyUsingHardlinks` _boolean_ | copyUsingHardlinks uses hardlinks instead of copies when importing. |  | Optional: \{\} <br /> |
| `deleteEmptyFolders` _boolean_ | deleteEmptyFolders removes empty author folders after cleanup. |  | Optional: \{\} <br /> |
| `importExtraFiles` _boolean_ | importExtraFiles imports additional files alongside media. |  | Optional: \{\} <br /> |
| `extraFileExtensions` _string_ | extraFileExtensions is a comma-separated list of extra file extensions to import. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `downloadPropersAndRepacks` _string_ | downloadPropersAndRepacks controls how proper/repack releases are handled. |  | Enum: [preferAndUpgrade doNotPrefer doNotUpgrade] <br />Optional: \{\} <br /> |
| `recycleBin` _string_ | recycleBin is the path to the recycling bin folder. An empty string<br />turns the recycling bin off. |  | Optional: \{\} <br /> |
| `recycleBinCleanupDays` _integer_ | recycleBinCleanupDays is how many days files stay in the recycling bin. |  | Optional: \{\} <br /> |
| `minimumFreeSpaceWhenImporting` _integer_ | minimumFreeSpaceWhenImporting is the minimum free space in MB required. |  | Optional: \{\} <br /> |
| `rescanAfterRefresh` _string_ | rescanAfterRefresh controls when to rescan author folders. |  | Enum: [always afterManual never] <br />Optional: \{\} <br /> |
| `setPermissionsLinux` _boolean_ | setPermissionsLinux enables setting file permissions on import. |  | Optional: \{\} <br /> |
| `createEmptyAuthorFolders` _boolean_ | createEmptyAuthorFolders creates folders for authors with no files. |  | Optional: \{\} <br /> |


#### ReadarrNaming



ReadarrNaming configures author and book naming conventions.



_Appears in:_
- [ReadarrSpec](#readarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `renameBooks` _boolean_ | renameBooks enables automatic book file renaming. |  | Optional: \{\} <br /> |
| `replaceIllegalCharacters` _boolean_ | replaceIllegalCharacters replaces illegal characters in filenames. |  | Optional: \{\} <br /> |
| `standardBookFormat` _string_ | standardBookFormat is the naming format for book files. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `authorFolderFormat` _string_ | authorFolderFormat is the naming format for author folders. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### ReadarrSpec



ReadarrSpec is a Readarr instance's connection and app-wide settings. Indexers
and download clients are resources of their own that refer to it.



_Appears in:_
- [Readarr](#readarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[AppConnection](#appconnection)_ | connection defines how to connect to the Readarr instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `mediaManagement` _[ReadarrMediaManagement](#readarrmediamanagement)_ | mediaManagement configures media management settings. |  | Optional: \{\} <br /> |
| `naming` _[ReadarrNaming](#readarrnaming)_ | naming configures author and book naming formats. |  | Optional: \{\} <br /> |
| `indexerConfig` _[ReadarrIndexerConfig](#readarrindexerconfig)_ | indexerConfig configures global indexer settings. |  | Optional: \{\} <br /> |
| `downloadClientConfig` _[ReadarrDownloadClientConfig](#readarrdownloadclientconfig)_ | downloadClientConfig configures global download client behavior. |  | Optional: \{\} <br /> |
| `ui` _[ReadarrUI](#readarrui)_ | ui configures UI display settings. |  | Optional: \{\} <br /> |
| `rootFolders` _[RootFolder](#rootfolder) array_ | rootFolders defines book library root folder paths. |  | Optional: \{\} <br /> |
| `remotePathMappings` _[RemotePathMapping](#remotepathmapping) array_ | remotePathMappings translates paths reported by remote download clients.<br />Entries are matched by host and remotePath. Only operator-created entries can be pruned. |  | Optional: \{\} <br /> |
| `qualityProfiles` _[QualityProfile](#qualityprofile) array_ | qualityProfiles defines quality profiles for controlling download quality preferences. |  | Optional: \{\} <br /> |
| `customFormats` _[CustomFormat](#customformat) array_ | customFormats defines custom format scoring rules for release selection. |  | Optional: \{\} <br /> |
| `qualityDefinitions` _[QualityDefinition](#qualitydefinition) array_ | qualityDefinitions sets the size limits of the app's qualities. The app<br />has a fixed set of qualities, so none is created or pruned, and qualities<br />left out keep their limits. |  | Optional: \{\} <br /> |
| `delayProfiles` _[DelayProfile](#delayprofile) array_ | delayProfiles defines delay profiles. The app checks tagged profiles in<br />the order listed, and the default profile, the one without tags, last.<br />Only operator-created profiles can be pruned, so the default never is. |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `tags` _[Tag](#tag) array_ | tags defines tags for linking resources together. |  | Optional: \{\} <br /> |
| `notifications` _[Notification](#notification) array_ | notifications defines notification/connection configurations. |  | Optional: \{\} <br /> |


#### ReadarrStatus



ReadarrStatus defines the observed state of Readarr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Readarr](#readarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Readarr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### ReadarrUI



ReadarrUI configures UI display settings.



_Appears in:_
- [ReadarrSpec](#readarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `firstDayOfWeek` _integer_ | firstDayOfWeek is the first day of the week. 0=Sunday, 1=Monday. |  | Optional: \{\} <br /> |
| `shortDateFormat` _string_ | shortDateFormat is the short date display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `longDateFormat` _string_ | longDateFormat is the long date display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `timeFormat` _string_ | timeFormat is the time display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `theme` _string_ | theme is the UI theme. |  | Enum: [auto dark light] <br />Optional: \{\} <br /> |
| `uiLanguage` _integer_ | uiLanguage is the UI language ID. |  | Optional: \{\} <br /> |


#### ReconcileConfig



ReconcileConfig holds common reconciliation settings shared across all config CRDs.



_Appears in:_
- [AutobrrSpec](#autobrrspec)
- [BazarrSpec](#bazarrspec)
- [FlareSolverrSpec](#flaresolverrspec)
- [JellyfinSpec](#jellyfinspec)
- [LidarrSpec](#lidarrspec)
- [MaintainerrSpec](#maintainerrspec)
- [PlexSpec](#plexspec)
- [ProwlarrSpec](#prowlarrspec)
- [QBittorrentSpec](#qbittorrentspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SabnzbdSpec](#sabnzbdspec)
- [SeerrSpec](#seerrspec)
- [SonarrSpec](#sonarrspec)
- [TdarrSpec](#tdarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `intervalSeconds` _integer_ | intervalSeconds is how often the operator reconciles, in seconds.<br />Defaults to 300, and is at least 30. |  | Minimum: 30 <br />Optional: \{\} <br /> |
| `prune` _boolean_ | prune deletes resources the operator created, tracked in<br />status.managedResources, once they are removed from this spec. Resources<br />it did not create are never deleted, and a resource type with nothing<br />declared is left alone. Root folders and tags are never pruned.<br />Not every integration supports prune; one that does not rejects it with<br />InvalidConfig rather than ignoring it.<br />Default: false (only create/update, never delete). |  | Optional: \{\} <br /> |
| `driftPolicy` _string_ | driftPolicy controls whether drift is corrected or only recorded.<br />"enforce" rewrites a resource whose live state differs from this CR.<br />"observe" leaves the app untouched and only counts the difference, which<br />makes drift measurable before handing the operator write authority.<br />Default: enforce. |  | Enum: [enforce observe] <br />Optional: \{\} <br /> |
| `deletionPolicy` _string_ | deletionPolicy controls what happens to resources in the app when this CR<br />is deleted. "orphan" leaves them in place, "delete" removes the resources<br />the operator created.<br />Default: orphan. |  | Enum: [orphan delete] <br />Optional: \{\} <br /> |


#### RemotePathMapping



RemotePathMapping translates a download client's path into a path visible to the app.
Identity is the pair (host, remotePath); localPath can be updated in place.



_Appears in:_
- [LidarrSpec](#lidarrspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `host` _string_ | host must match the download client's configured hostname. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `remotePath` _string_ | remotePath is the directory reported by the download client. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `localPath` _string_ | localPath is the existing directory as seen by the app. |  | MinLength: 1 <br />Required: \{\} <br /> |


#### RootFolder



RootFolder defines a media root folder path.



_Appears in:_
- [LidarrSpec](#lidarrspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `path` _string_ | path is the filesystem path for the root folder inside the container. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `name` _string_ | name labels the root folder (Lidarr, Readarr). Defaults to the last<br />element of path. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `defaultQualityProfileName` _string_ | defaultQualityProfileName is the quality profile for content added to<br />this folder (Lidarr, Readarr; required there). Resolved to an ID at<br />reconciliation time, after declared quality profiles are applied. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `defaultMetadataProfileName` _string_ | defaultMetadataProfileName is the metadata profile for content added to<br />this folder (Lidarr, Readarr; required there). Resolved to an ID at<br />reconciliation time. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### Sabnzbd



Sabnzbd configures a Sabnzbd instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Sabnzbd` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[SabnzbdSpec](#sabnzbdspec)_ | spec is the desired state of the Sabnzbd. |  | Required: \{\} <br /> |
| `status` _[SabnzbdStatus](#sabnzbdstatus)_ | status is the observed state of the Sabnzbd. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### SabnzbdCategory



SabnzbdCategory defines a download category.



_Appears in:_
- [SabnzbdSpec](#sabnzbdspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the category name. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `dir` _string_ | dir is the subdirectory for this category (relative to complete dir). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `script` _string_ | script is the post-processing script for this category. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _string_ | priority is the download priority. default uses SABnzbd's default<br />category's priority. |  | Enum: [default paused low normal high force] <br />Optional: \{\} <br /> |
| `postProcessing` _string_ | postProcessing is how far SABnzbd processes the category's jobs: download<br />only, repair, unpack, or unpack and delete the archives. |  | Enum: [download repair unpack delete] <br />Optional: \{\} <br /> |


#### SabnzbdFolders



SabnzbdFolders configures folder paths.



_Appears in:_
- [SabnzbdSpec](#sabnzbdspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `completeDir` _string_ | completeDir is the path for completed downloads. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `incompleteDir` _string_ | incompleteDir is the path for incomplete downloads. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `tempDownloadDir` _string_ | tempDownloadDir is the temporary download path. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `nzbBackupDir` _string_ | nzbBackupDir is the path for NZB backups. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `scriptDir` _string_ | scriptDir is the path for scripts. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### SabnzbdGeneral



SabnzbdGeneral configures general SABnzbd settings.



_Appears in:_
- [SabnzbdSpec](#sabnzbdspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `downloadSpeedLimit` _string_ | downloadSpeedLimit is the download speed limit (e.g., "100M", "0" for unlimited). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `pauseOnPostProcessing` _boolean_ | pauseOnPostProcessing pauses downloading during post-processing. |  | Optional: \{\} <br /> |
| `autoSort` _boolean_ | autoSort sorts the queue by age whenever an NZB is added. |  | Optional: \{\} <br /> |
| `preCheck` _boolean_ | preCheck enables NZB pre-check for completion. |  | Optional: \{\} <br /> |
| `topOnly` _boolean_ | topOnly fetches articles only for the job at the top of the queue. |  | Optional: \{\} <br /> |
| `failHopelessJobs` _boolean_ | failHopelessJobs fails a job as soon as it can't be completed. |  | Optional: \{\} <br /> |
| `encryptedRarAction` _string_ | encryptedRarAction is what SABnzbd does with a job containing an encrypted RAR. |  | Enum: [off pause fail] <br />Optional: \{\} <br /> |
| `unwantedExtensions` _string array_ | unwantedExtensions lists file extensions, without the dot, that<br />unwantedExtensionsAction applies to. |  | Optional: \{\} <br /> |
| `unwantedExtensionsMode` _string_ | unwantedExtensionsMode decides whether unwantedExtensions lists the<br />extensions to act on or the only ones allowed. |  | Enum: [blocklist allowlist] <br />Optional: \{\} <br /> |
| `unwantedExtensionsAction` _string_ | unwantedExtensionsAction is what SABnzbd does with a job containing an<br />unwanted extension. |  | Enum: [off pause fail] <br />Optional: \{\} <br /> |
| `directUnpack` _boolean_ | directUnpack unpacks a job while it's still downloading. |  | Optional: \{\} <br /> |
| `enableAllPar` _boolean_ | enableAllPar downloads every par2 file, not only those a repair needs. |  | Optional: \{\} <br /> |
| `sfvCheck` _boolean_ | sfvCheck verifies jobs without par2 files against their SFV files. |  | Optional: \{\} <br /> |
| `safePostProcessing` _boolean_ | safePostProcessing post-processes only jobs that passed verification. |  | Optional: \{\} <br /> |
| `enableRecursive` _boolean_ | enableRecursive unpacks archives found inside archives. |  | Optional: \{\} <br /> |
| `flatUnpack` _boolean_ | flatUnpack ignores the folders inside archives when unpacking. |  | Optional: \{\} <br /> |
| `ignoreSamples` _boolean_ | ignoreSamples skips sample files. |  | Optional: \{\} <br /> |
| `deobfuscateFinalFilenames` _boolean_ | deobfuscateFinalFilenames renames obfuscated files after the job. |  | Optional: \{\} <br /> |
| `scriptCanFail` _boolean_ | scriptCanFail lets a job's post-processing script fail without failing the job. |  | Optional: \{\} <br /> |
| `cleanupList` _string array_ | cleanupList lists file extensions, without the dot, to delete after a job. |  | Optional: \{\} <br /> |
| `historyRetention` _string_ | historyRetention decides how long jobs stay in the history. |  | Enum: [all number-archive number-delete days-archive days-delete all-archive all-delete] <br />Optional: \{\} <br /> |
| `historyRetentionNumber` _integer_ | historyRetentionNumber is the number of jobs or days historyRetention keeps. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `permissions` _string_ | permissions are the octal permissions set on completed downloads, such as 775. |  | MinLength: 1 <br />Pattern: `^[0-7]\{3,4\}$` <br />Optional: \{\} <br /> |
| `downloadFree` _string_ | downloadFree is the free space the incomplete folder needs to keep<br />downloading, such as 500M. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `completeFree` _string_ | completeFree is the free space the complete folder needs to keep<br />downloading, such as 10G. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `cacheLimit` _string_ | cacheLimit is the memory used to cache articles, such as 1G. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `propagationDelay` _integer_ | propagationDelay holds back jobs posted less than this many minutes ago. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `hostWhitelist` _string array_ | hostWhitelist lists the host names SABnzbd answers on besides IP<br />addresses, localhost and names ending in .local, which cover<br />*.svc.cluster.local. It replaces SABnzbd's list, so it must include the<br />host in connection.url unless that is one of those. |  | Optional: \{\} <br /> |


#### SabnzbdServer



SabnzbdServer defines a Usenet server connection.



_Appears in:_
- [SabnzbdSpec](#sabnzbdspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name of the server. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `host` _string_ | host is the hostname of the Usenet server. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `port` _integer_ | port is the port number. |  | Maximum: 65535 <br />Minimum: 1 <br />Required: \{\} <br /> |
| `ssl` _boolean_ | ssl enables SSL/TLS connection. |  | Optional: \{\} <br /> |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the username. |  | Optional: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the password. |  | Optional: \{\} <br /> |
| `connections` _integer_ | connections is the number of simultaneous connections. |  | Optional: \{\} <br /> |
| `priority` _integer_ | priority is the server priority. 0 = highest. |  | Optional: \{\} <br /> |
| `retention` _integer_ | retention is the server retention in days. 0 = unlimited. |  | Optional: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this server is active. A new server is enabled<br />unless this is false. |  | Optional: \{\} <br /> |
| `sslVerify` _string_ | sslVerify is how strictly the server's certificate is checked when ssl is on. |  | Enum: [strict medium minimal disabled] <br />Optional: \{\} <br /> |
| `optional` _boolean_ | optional lets SABnzbd stop using the server for a while when it fails. |  | Optional: \{\} <br /> |
| `required` _boolean_ | required makes SABnzbd wait for the server when it fails instead of<br />skipping it. |  | Optional: \{\} <br /> |
| `timeout` _integer_ | timeout is how many seconds SABnzbd waits for the server to answer. |  | Minimum: 1 <br />Optional: \{\} <br /> |


#### SabnzbdSpec



SabnzbdSpec defines the desired state of Sabnzbd.



_Appears in:_
- [Sabnzbd](#sabnzbd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[AppConnection](#appconnection)_ | connection defines how to connect to the SABnzbd instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `general` _[SabnzbdGeneral](#sabnzbdgeneral)_ | general configures general SABnzbd settings. |  | Optional: \{\} <br /> |
| `servers` _[SabnzbdServer](#sabnzbdserver) array_ | servers defines Usenet server connections. |  | Optional: \{\} <br /> |
| `categories` _[SabnzbdCategory](#sabnzbdcategory) array_ | categories defines download categories. |  | Optional: \{\} <br /> |
| `folders` _[SabnzbdFolders](#sabnzbdfolders)_ | folders configures download and temp folder paths. |  | Optional: \{\} <br /> |


#### SabnzbdStatus



SabnzbdStatus defines the observed state of Sabnzbd.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Sabnzbd](#sabnzbd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Sabnzbd resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### SecretKeyRef



SecretKeyRef references a key within a Kubernetes Secret.



_Appears in:_
- [AppConnection](#appconnection)
- [AutobrrDownloadClient](#autobrrdownloadclient)
- [AutobrrFeed](#autobrrfeed)
- [AutobrrIRCChannel](#autobrrircchannel)
- [AutobrrIRCNetwork](#autobrrircnetwork)
- [BazarrAppConnection](#bazarrappconnection)
- [DownloadClient](#downloadclient)
- [JellyfinAdminUser](#jellyfinadminuser)
- [JellyfinConnection](#jellyfinconnection)
- [LidarrDownloadClientSpec](#lidarrdownloadclientspec)
- [MaintainerrArrServer](#maintainerrarrserver)
- [MaintainerrJellyfin](#maintainerrjellyfin)
- [MaintainerrPlex](#maintainerrplex)
- [MaintainerrSeerrConnection](#maintainerrseerrconnection)
- [MaintainerrServiceConnection](#maintainerrserviceconnection)
- [PlexConnection](#plexconnection)
- [ProwlarrApplication](#prowlarrapplication)
- [QBittorrentConnection](#qbittorrentconnection)
- [RadarrDownloadClientSpec](#radarrdownloadclientspec)
- [ReadarrDownloadClientSpec](#readarrdownloadclientspec)
- [SabnzbdServer](#sabnzbdserver)
- [SeerrConnection](#seerrconnection)
- [SeerrJellyfinAuth](#seerrjellyfinauth)
- [SeerrPlexAuth](#seerrplexauth)
- [SeerrServiceConnection](#seerrserviceconnection)
- [SonarrDownloadClientSpec](#sonarrdownloadclientspec)
- [TLSConfig](#tlsconfig)
- [TdarrSettings](#tdarrsettings)
- [ValueSource](#valuesource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the name of the Secret. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `key` _string_ | key is the key within the Secret. |  | MinLength: 1 <br />Required: \{\} <br /> |


#### Seerr



Seerr configures a Seerr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Seerr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[SeerrSpec](#seerrspec)_ | spec is the desired state of the Seerr. |  | Required: \{\} <br /> |
| `status` _[SeerrStatus](#seerrstatus)_ | status is the observed state of the Seerr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### SeerrConnection



SeerrConnection defines how the operator connects to Seerr.



_Appears in:_
- [SeerrSpec](#seerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url is the base URL of the Seerr instance (e.g., http://jellyseerr.media.svc.cluster.local:5055). |  | MinLength: 1 <br />Pattern: `^https?://` <br />Required: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing Seerr's API key. When set,<br />it is used instead of logging in through the media server. |  | Optional: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | tls configures TLS settings for the connection.<br />Only needed when Seerr uses HTTPS with self-signed or private CA certificates. |  | Optional: \{\} <br /> |


#### SeerrJellyfinAuth



SeerrJellyfinAuth defines the Jellyfin authentication configuration for Seerr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [SeerrSpec](#seerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `appRef` _[AppRef](#appref)_ | appRef names the Jellyfin resource whose connection URL and admin user are<br />used in place of hostname, port, useSsl, urlBase, usernameSecretRef and<br />passwordSecretRef. |  | Optional: \{\} <br /> |
| `hostname` _string_ | hostname is the Jellyfin server hostname. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the Jellyfin server port. |  | Optional: \{\} <br /> |
| `useSsl` _boolean_ | useSsl enables SSL for the Jellyfin connection. |  | Optional: \{\} <br /> |
| `urlBase` _string_ | urlBase is the subpath Jellyfin is served under, if any. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the Jellyfin username. |  | Optional: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the Jellyfin password. |  | Optional: \{\} <br /> |


#### SeerrMain



SeerrMain configures general Seerr application settings.



_Appears in:_
- [SeerrSpec](#seerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `applicationTitle` _string_ | applicationTitle is the application display title. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `applicationUrl` _string_ | applicationUrl is the external URL for the application. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `hideAvailable` _boolean_ | hideAvailable hides already-available media from discovery. |  | Optional: \{\} <br /> |
| `localLogin` _boolean_ | localLogin enables local user login. |  | Optional: \{\} <br /> |
| `mediaServerLogin` _boolean_ | mediaServerLogin enables login via the media server (Jellyfin). |  | Optional: \{\} <br /> |
| `defaultPermissions` _integer_ | defaultPermissions is the default permission bitmask for new users. |  | Optional: \{\} <br /> |
| `partialRequestsEnabled` _boolean_ | partialRequestsEnabled enables partial season/episode requests. |  | Optional: \{\} <br /> |
| `locale` _string_ | locale is the application locale (e.g., "en"). |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### SeerrNotificationAgent



SeerrNotificationAgent configures a single notification agent in Seerr.



_Appears in:_
- [SeerrSpec](#seerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `agent` _string_ | agent is the notification agent type.<br />Supported values: discord, slack, telegram, pushover, email, webhook,<br />gotify, ntfy, pushbullet, webpush. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enabled` _boolean_ | enabled enables this notification agent. |  | Optional: \{\} <br /> |
| `types` _string array_ | types are the events that send a notification. An empty list sends none. |  | items:Enum: [mediaPending mediaApproved mediaAutoApproved mediaAutoRequested mediaAvailable mediaFailed mediaDeclined issueCreated issueComment issueResolved issueReopened] <br />Optional: \{\} <br /> |
| `options` _[ConfigField](#configfield) array_ | options are the agent's settings, such as botUsername or chatId, each a<br />value or read from a Secret, such as webhookUrl or botAPI. Values keep<br />their type, so enableMentions: false is off. |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### SeerrPlexAuth



SeerrPlexAuth defines Plex authentication for Seerr setup.

_Validation:_
- MinProperties: 1

_Appears in:_
- [SeerrSpec](#seerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `appRef` _[AppRef](#appref)_ | appRef names the Plex resource whose connection URL and token are used in<br />place of hostname, port, useSsl and tokenSecretRef. |  | Optional: \{\} <br /> |
| `hostname` _string_ | hostname is the Plex server hostname. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the Plex server port. Defaults to 32400. |  | Optional: \{\} <br /> |
| `useSsl` _boolean_ | useSsl enables SSL for the Plex connection. |  | Optional: \{\} <br /> |
| `tokenSecretRef` _[SecretKeyRef](#secretkeyref)_ | tokenSecretRef references a Secret containing the Plex auth token. |  | Optional: \{\} <br /> |


#### SeerrServiceConnection



SeerrServiceConnection defines a connection to a Sonarr or Radarr instance.



_Appears in:_
- [SeerrSpec](#seerrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the display name for this service connection. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `appRef` _[AppRef](#appref)_ | appRef names the server's resource, a Sonarr in sonarr or a Radarr in<br />radarr, whose connection is used in place of hostname, port, useSsl,<br />baseUrl and apiKeySecretRef. |  | Optional: \{\} <br /> |
| `hostname` _string_ | hostname is the service hostname. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the service port. |  | Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the service API key. |  | Optional: \{\} <br /> |
| `useSsl` _boolean_ | useSsl enables SSL for the service connection. |  | Optional: \{\} <br /> |
| `baseUrl` _string_ | baseUrl is the URL base path for the service. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `activeProfileId` _integer_ | activeProfileId is used when activeProfileName is omitted. |  | Optional: \{\} <br /> |
| `activeProfileName` _string_ | activeProfileName is resolved against the service on each reconciliation.<br />When set, it takes precedence over activeProfileId; exactly one match is required. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `activeDirectory` _string_ | activeDirectory is the root folder path for media. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `is4k` _boolean_ | is4k indicates this is a 4K instance. |  | Optional: \{\} <br /> |
| `isDefault` _boolean_ | isDefault indicates this is the default service for its type. |  | Optional: \{\} <br /> |
| `seriesType` _string_ | seriesType is the series type for Sonarr (e.g., "standard", "anime").<br />Only applicable when used with Sonarr. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `minimumAvailability` _string_ | minimumAvailability is the minimum availability for Radarr.<br />Only applicable when used with Radarr. |  | Enum: [announced inCinemas released] <br />Optional: \{\} <br /> |
| `enableSeasonFolders` _boolean_ | enableSeasonFolders enables season folder organization in Sonarr.<br />Only applicable when used with Sonarr. |  | Optional: \{\} <br /> |
| `externalUrl` _string_ | externalUrl is the address users open the server at, for links in Seerr. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `syncEnabled` _boolean_ | syncEnabled scans the server for media and request status. |  | Optional: \{\} <br /> |
| `preventSearch` _boolean_ | preventSearch stops the server from searching as soon as a request is approved. |  | Optional: \{\} <br /> |
| `tagRequests` _boolean_ | tagRequests tags each request with the requesting user. |  | Optional: \{\} <br /> |
| `activeAnimeProfileName` _string_ | activeAnimeProfileName is the quality profile for anime, resolved against<br />the server on each reconciliation. Only applicable when used with Sonarr. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `activeAnimeDirectory` _string_ | activeAnimeDirectory is the root folder for anime. Only applicable when<br />used with Sonarr. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `animeSeriesType` _string_ | animeSeriesType is the series type for anime. Only applicable when used<br />with Sonarr. |  | Enum: [standard daily anime] <br />Optional: \{\} <br /> |
| `monitorNewItems` _string_ | monitorNewItems decides whether seasons added to a series later are<br />monitored. Only applicable when used with Sonarr. |  | Enum: [all none] <br />Optional: \{\} <br /> |


#### SeerrSpec



SeerrSpec defines the desired state of Seerr.



_Appears in:_
- [Seerr](#seerr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[SeerrConnection](#seerrconnection)_ | connection defines how to connect to the Seerr instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `jellyfinAuth` _[SeerrJellyfinAuth](#seerrjellyfinauth)_ | jellyfinAuth defines Jellyfin authentication settings.<br />Set this for Jellyfin-based Seerr instances. Mutually exclusive with plexAuth. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `plexAuth` _[SeerrPlexAuth](#seerrplexauth)_ | plexAuth defines Plex authentication settings.<br />Set this for Plex-based Seerr instances. Mutually exclusive with jellyfinAuth. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `libraries` _string array_ | libraries names the media server's libraries Seerr shows and takes<br />requests for. Those are enabled and the rest disabled, on every pass.<br />Left out, first-time setup enables every library and later passes leave<br />them as they are. |  | MinItems: 1 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `main` _[SeerrMain](#seerrmain)_ | main configures general application settings. |  | Optional: \{\} <br /> |
| `sonarr` _[SeerrServiceConnection](#seerrserviceconnection) array_ | sonarr lists the Sonarr servers Seerr sends TV requests to, matched by<br />name. Seerr uses the default server, and the default 4K server for 4K<br />requests. |  | Optional: \{\} <br /> |
| `radarr` _[SeerrServiceConnection](#seerrserviceconnection) array_ | radarr lists the Radarr servers Seerr sends movie requests to, matched by<br />name. Seerr uses the default server, and the default 4K server for 4K<br />requests. |  | Optional: \{\} <br /> |
| `notifications` _[SeerrNotificationAgent](#seerrnotificationagent) array_ | notifications configures notification agents (Discord, Slack, Telegram, etc.).<br />Each agent is a singleton — only one configuration per agent type. |  | Optional: \{\} <br /> |


#### SeerrStatus



SeerrStatus defines the observed state of Seerr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Seerr](#seerr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Seerr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `initialized` _boolean_ | initialized indicates whether the Seerr initial setup has been completed. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### Sonarr



Sonarr configures a Sonarr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Sonarr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[SonarrSpec](#sonarrspec)_ | spec is the desired state of the Sonarr. |  | Required: \{\} <br /> |
| `status` _[SonarrStatus](#sonarrstatus)_ | status is the observed state of the Sonarr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### SonarrDownloadClient



SonarrDownloadClient is one download client in a Sonarr instance.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `SonarrDownloadClient` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[SonarrDownloadClientSpec](#sonarrdownloadclientspec)_ | spec is the desired state of the SonarrDownloadClient. |  | Required: \{\} <br /> |
| `status` _[SonarrDownloadClientStatus](#sonarrdownloadclientstatus)_ | status is the observed state of the SonarrDownloadClient. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### SonarrDownloadClientConfig



SonarrDownloadClientConfig configures global download client behavior.



_Appears in:_
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enableCompletedDownloadHandling` _boolean_ | enableCompletedDownloadHandling auto-imports completed downloads. |  | Optional: \{\} <br /> |
| `autoRedownloadFailed` _boolean_ | autoRedownloadFailed automatically searches when a download fails. |  | Optional: \{\} <br /> |
| `autoRedownloadFailedFromInteractiveSearch` _boolean_ | autoRedownloadFailedFromInteractiveSearch auto-searches failed interactive searches. |  | Optional: \{\} <br /> |


#### SonarrDownloadClientSpec



SonarrDownloadClientSpec is one download client in a Sonarr instance.



_Appears in:_
- [SonarrDownloadClient](#sonarrdownloadclient)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sonarrRef` _[LocalObjectReference](#localobjectreference)_ | sonarrRef names the Sonarr this download client belongs to, in the same namespace. |  | Required: \{\} <br /> |
| `deletionPolicy` _string_ | deletionPolicy controls what deleting this resource does in Sonarr:<br />"delete" removes the download client, "orphan" leaves it. One that existed before<br />the operator adopted it is never deleted. | delete | Enum: [delete orphan] <br />Optional: \{\} <br /> |
| `name` _string_ | name is the display name of the download client. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `appRef` _[AppRef](#appref)_ | appRef names the download client's resource, a QBittorrent or Sabnzbd,<br />whose kind and connection are used in place of implementation, protocol,<br />host, port, useSsl, urlBase and the credentials. |  | Optional: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this download client is active. A new download<br />client is enabled unless this is false. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the download protocol. |  | Enum: [torrent usenet] <br />Optional: \{\} <br /> |
| `implementation` _string_ | implementation is the download client type (e.g., QBittorrent, Sabnzbd, NzbGet, Transmission, Deluge). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `configContract` _string_ | configContract overrides the default config contract name.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `host` _string_ | host is the hostname or IP of the download client. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port of the download client. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `useSsl` _boolean_ | useSsl enables SSL/TLS for the download client connection. |  | Optional: \{\} <br /> |
| `urlBase` _string_ | urlBase is the base URL path for the download client (e.g., "/qbittorrent"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `category` _string_ | category is the download category to use. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the download client priority. Lower values are higher priority.<br />A new download client gets 1. |  | Maximum: 50 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `removeCompletedDownloads` _boolean_ | removeCompletedDownloads controls whether completed downloads are removed.<br />A new download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `removeFailedDownloads` _boolean_ | removeFailedDownloads controls whether failed downloads are removed. A new<br />download client removes them unless this is false. |  | Optional: \{\} <br /> |
| `usernameSecretRef` _[SecretKeyRef](#secretkeyref)_ | usernameSecretRef references a Secret containing the username for authentication. |  | Optional: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | passwordSecretRef references a Secret containing the password for authentication. |  | Optional: \{\} <br /> |
| `apiKeySecretRef` _[SecretKeyRef](#secretkeyref)_ | apiKeySecretRef references a Secret containing the API key (for Usenet clients like SABnzbd/NzbGet). |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines additional implementation-specific configuration fields.<br />Use these for settings not covered by first-class fields (e.g., initialState, sequentialOrder). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### SonarrDownloadClientStatus



SonarrDownloadClientStatus is the observed state of a SonarrDownloadClient.

_Validation:_
- MinProperties: 1

_Appears in:_
- [SonarrDownloadClient](#sonarrdownloadclient)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the download client. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `id` _integer_ | id is the download client's ID in Sonarr. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records what the operator created for this resource,<br />so deletion only removes what it created. |  | Optional: \{\} <br /> |


#### SonarrIndexer



SonarrIndexer is one indexer in a Sonarr instance.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `SonarrIndexer` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[SonarrIndexerSpec](#sonarrindexerspec)_ | spec is the desired state of the SonarrIndexer. |  | Required: \{\} <br /> |
| `status` _[SonarrIndexerStatus](#sonarrindexerstatus)_ | status is the observed state of the SonarrIndexer. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### SonarrIndexerConfig



SonarrIndexerConfig configures global indexer settings.



_Appears in:_
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minimumAge` _integer_ | minimumAge is the minimum age in minutes before downloading. |  | Optional: \{\} <br /> |
| `retention` _integer_ | retention is the usenet retention in days. 0 means unlimited. |  | Optional: \{\} <br /> |
| `maximumSize` _integer_ | maximumSize is the maximum release size in MB. 0 means unlimited. |  | Optional: \{\} <br /> |
| `rssSyncInterval` _integer_ | rssSyncInterval is the RSS sync interval in minutes. 0 disables RSS. |  | Optional: \{\} <br /> |


#### SonarrIndexerSpec



SonarrIndexerSpec is one indexer in a Sonarr instance.



_Appears in:_
- [SonarrIndexer](#sonarrindexer)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sonarrRef` _[LocalObjectReference](#localobjectreference)_ | sonarrRef names the Sonarr this indexer belongs to, in the same namespace. |  | Required: \{\} <br /> |
| `deletionPolicy` _string_ | deletionPolicy controls what deleting this resource does in Sonarr:<br />"delete" removes the indexer, "orphan" leaves it. One that existed before<br />the operator adopted it is never deleted. | delete | Enum: [delete orphan] <br />Optional: \{\} <br /> |
| `name` _string_ | name is the display name of the indexer. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `enable` _boolean_ | enable controls whether this indexer is active. False turns off RSS,<br />automatic and interactive search; true turns on those not set below. |  | Optional: \{\} <br /> |
| `protocol` _string_ | protocol is the indexer protocol. |  | Enum: [torrent usenet] <br />Required: \{\} <br /> |
| `implementation` _string_ | implementation is the indexer type (e.g., Newznab, Torznab). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `configContract` _string_ | configContract is the configuration contract name for the implementation.<br />Defaults to "<implementation>Settings" if not set. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `priority` _integer_ | priority is the indexer priority. Lower values are higher priority. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `enableRss` _boolean_ | enableRss enables RSS feed polling for this indexer. A new indexer<br />polls unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableAutomaticSearch` _boolean_ | enableAutomaticSearch enables automatic search for this indexer. A new<br />indexer searches unless this or enable is false. |  | Optional: \{\} <br /> |
| `enableInteractiveSearch` _boolean_ | enableInteractiveSearch enables interactive (manual) search for this<br />indexer. A new indexer allows it unless this or enable is false. |  | Optional: \{\} <br /> |
| `tagLabels` _string array_ | tagLabels references tags by label, resolved against the app on each<br />reconciliation. A missing or ambiguous label blocks writes, and an empty<br />list clears the tags. |  | Optional: \{\} <br /> |
| `fields` _[ConfigField](#configfield) array_ | fields defines the indexer's configuration fields (e.g., baseUrl, apiKey, categories). |  | MaxProperties: 2 <br />Optional: \{\} <br /> |


#### SonarrIndexerStatus



SonarrIndexerStatus is the observed state of a SonarrIndexer.

_Validation:_
- MinProperties: 1

_Appears in:_
- [SonarrIndexer](#sonarrindexer)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the indexer. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `id` _integer_ | id is the indexer's ID in Sonarr. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records what the operator created for this resource,<br />so deletion only removes what it created. |  | Optional: \{\} <br /> |


#### SonarrMediaManagement



SonarrMediaManagement configures how Sonarr handles media files.



_Appears in:_
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `copyUsingHardlinks` _boolean_ | copyUsingHardlinks uses hardlinks instead of copies when importing. |  | Optional: \{\} <br /> |
| `deleteEmptyFolders` _boolean_ | deleteEmptyFolders removes empty series folders after cleanup. |  | Optional: \{\} <br /> |
| `importExtraFiles` _boolean_ | importExtraFiles imports additional files (subtitles, etc.) alongside media. |  | Optional: \{\} <br /> |
| `extraFileExtensions` _string_ | extraFileExtensions is a comma-separated list of extra file extensions to import. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `downloadPropersAndRepacks` _string_ | downloadPropersAndRepacks controls how proper/repack releases are handled. |  | Enum: [preferAndUpgrade doNotPrefer doNotUpgrade] <br />Optional: \{\} <br /> |
| `recycleBin` _string_ | recycleBin is the path to the recycling bin folder. An empty string<br />turns the recycling bin off. |  | Optional: \{\} <br /> |
| `recycleBinCleanupDays` _integer_ | recycleBinCleanupDays is how many days files stay in the recycling bin. |  | Optional: \{\} <br /> |
| `minimumFreeSpaceWhenImporting` _integer_ | minimumFreeSpaceWhenImporting is the minimum free space in MB required to import. |  | Optional: \{\} <br /> |
| `enableMediaInfo` _boolean_ | enableMediaInfo enables media info scanning. |  | Optional: \{\} <br /> |
| `rescanAfterRefresh` _string_ | rescanAfterRefresh controls when to rescan series folders. |  | Enum: [always afterManual never] <br />Optional: \{\} <br /> |
| `setPermissionsLinux` _boolean_ | setPermissionsLinux enables setting file permissions on import. |  | Optional: \{\} <br /> |
| `episodeTitleRequired` _string_ | episodeTitleRequired controls when episode titles are required. |  | Enum: [always bulkSeasonReleases never] <br />Optional: \{\} <br /> |
| `skipFreeSpaceCheckWhenImporting` _boolean_ | skipFreeSpaceCheckWhenImporting skips free space check during import. |  | Optional: \{\} <br /> |
| `createEmptySeriesFolders` _boolean_ | createEmptySeriesFolders creates folders for series with no files. |  | Optional: \{\} <br /> |


#### SonarrNaming



SonarrNaming configures episode and folder naming conventions.



_Appears in:_
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `renameEpisodes` _boolean_ | renameEpisodes enables automatic episode file renaming. |  | Optional: \{\} <br /> |
| `replaceIllegalCharacters` _boolean_ | replaceIllegalCharacters replaces illegal characters in filenames. |  | Optional: \{\} <br /> |
| `colonReplacementFormat` _string_ | colonReplacementFormat defines how colons are replaced in filenames. |  | Enum: [delete dash spaceDash spaceDashSpace smart] <br />Optional: \{\} <br /> |
| `multiEpisodeStyle` _string_ | multiEpisodeStyle defines how multi-episode files are named: extend<br />(S01E01-02-03), duplicate (S01E01.S01E02), repeat (S01E01E02E03), scene<br />(S01E01-E02-E03), range (S01E01-03) or prefixedRange (S01E01-E03). |  | Enum: [extend duplicate repeat scene range prefixedRange] <br />Optional: \{\} <br /> |
| `standardEpisodeFormat` _string_ | standardEpisodeFormat is the naming format for standard episodes. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `dailyEpisodeFormat` _string_ | dailyEpisodeFormat is the naming format for daily episodes. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `animeEpisodeFormat` _string_ | animeEpisodeFormat is the naming format for anime episodes. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `seriesFolderFormat` _string_ | seriesFolderFormat is the naming format for series folders. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `seasonFolderFormat` _string_ | seasonFolderFormat is the naming format for season folders. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `specialsFolderFormat` _string_ | specialsFolderFormat is the naming format for specials folders. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### SonarrSpec



SonarrSpec is a Sonarr instance's connection and app-wide settings. Indexers
and download clients are resources of their own that refer to it.



_Appears in:_
- [Sonarr](#sonarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[AppConnection](#appconnection)_ | connection defines how to connect to the Sonarr instance. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `mediaManagement` _[SonarrMediaManagement](#sonarrmediamanagement)_ | mediaManagement configures media management settings. |  | Optional: \{\} <br /> |
| `naming` _[SonarrNaming](#sonarrnaming)_ | naming configures episode and series naming formats. |  | Optional: \{\} <br /> |
| `indexerConfig` _[SonarrIndexerConfig](#sonarrindexerconfig)_ | indexerConfig configures global indexer settings. |  | Optional: \{\} <br /> |
| `downloadClientConfig` _[SonarrDownloadClientConfig](#sonarrdownloadclientconfig)_ | downloadClientConfig configures global download client behavior. |  | Optional: \{\} <br /> |
| `ui` _[SonarrUI](#sonarrui)_ | ui configures UI display settings. |  | Optional: \{\} <br /> |
| `rootFolders` _[RootFolder](#rootfolder) array_ | rootFolders defines media library root folder paths. |  | Optional: \{\} <br /> |
| `remotePathMappings` _[RemotePathMapping](#remotepathmapping) array_ | remotePathMappings translates paths reported by remote download clients.<br />Entries are matched by host and remotePath. Only operator-created entries can be pruned. |  | Optional: \{\} <br /> |
| `qualityProfiles` _[QualityProfile](#qualityprofile) array_ | qualityProfiles defines quality profiles for controlling download quality preferences. |  | Optional: \{\} <br /> |
| `customFormats` _[CustomFormat](#customformat) array_ | customFormats defines custom format scoring rules for release selection. |  | Optional: \{\} <br /> |
| `qualityDefinitions` _[QualityDefinition](#qualitydefinition) array_ | qualityDefinitions sets the size limits of the app's qualities. The app<br />has a fixed set of qualities, so none is created or pruned, and qualities<br />left out keep their limits. |  | Optional: \{\} <br /> |
| `delayProfiles` _[DelayProfile](#delayprofile) array_ | delayProfiles defines delay profiles. The app checks tagged profiles in<br />the order listed, and the default profile, the one without tags, last.<br />Only operator-created profiles can be pruned, so the default never is. |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `tags` _[Tag](#tag) array_ | tags defines tags for linking resources together. |  | Optional: \{\} <br /> |
| `notifications` _[Notification](#notification) array_ | notifications defines notification/connection configurations. |  | Optional: \{\} <br /> |


#### SonarrStatus



SonarrStatus defines the observed state of Sonarr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Sonarr](#sonarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Sonarr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### SonarrUI



SonarrUI configures UI display settings.



_Appears in:_
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `firstDayOfWeek` _integer_ | firstDayOfWeek is the first day of the week. 0=Sunday, 1=Monday. |  | Maximum: 1 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `shortDateFormat` _string_ | shortDateFormat is the short date display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `longDateFormat` _string_ | longDateFormat is the long date display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `timeFormat` _string_ | timeFormat is the time display format. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `showRelativeDates` _boolean_ | showRelativeDates shows relative dates (today, yesterday). |  | Optional: \{\} <br /> |
| `theme` _string_ | theme is the UI theme. |  | Enum: [auto dark light] <br />Optional: \{\} <br /> |
| `enableColorImpairedMode` _boolean_ | enableColorImpairedMode enables color-impaired accessibility mode. |  | Optional: \{\} <br /> |
| `uiLanguage` _integer_ | uiLanguage is the UI language ID. 1=English. |  | Optional: \{\} <br /> |


#### TLSConfig



TLSConfig configures TLS for connections to an app's API.
Use this when the target app uses HTTPS with a self-signed certificate or a private CA.



_Appears in:_
- [AppConnection](#appconnection)
- [FlareSolverrConnection](#flaresolverrconnection)
- [JellyfinConnection](#jellyfinconnection)
- [MaintainerrConnection](#maintainerrconnection)
- [PlexConnection](#plexconnection)
- [QBittorrentConnection](#qbittorrentconnection)
- [SeerrConnection](#seerrconnection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `insecureSkipVerify` _boolean_ | insecureSkipVerify disables TLS certificate verification.<br />WARNING: This makes the connection susceptible to machine-in-the-middle attacks.<br />Only use for testing or when the target app uses a self-signed certificate and<br />providing a CA is not practical. |  | Optional: \{\} <br /> |
| `caSecretRef` _[SecretKeyRef](#secretkeyref)_ | caSecretRef references a Secret containing a PEM-encoded CA certificate bundle<br />used to verify the target app's TLS certificate.<br />The Secret must contain the CA certificate under the specified key. |  | Optional: \{\} <br /> |


#### Tag



Tag defines a tag that can be used to link resources together
(download clients, indexers, notifications, etc.).



_Appears in:_
- [LidarrSpec](#lidarrspec)
- [ProwlarrSpec](#prowlarrspec)
- [RadarrSpec](#radarrspec)
- [ReadarrSpec](#readarrspec)
- [SonarrSpec](#sonarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `label` _string_ | label is the tag name. |  | MinLength: 1 <br />Required: \{\} <br /> |


#### Tdarr



Tdarr configures a Tdarr instance the operator connects to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `arrchitect.dev/v1alpha1` | | |
| `kind` _string_ | `Tdarr` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[TdarrSpec](#tdarrspec)_ | spec is the desired state of the Tdarr. |  | Required: \{\} <br /> |
| `status` _[TdarrStatus](#tdarrstatus)_ | status is the observed state of the Tdarr. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### TdarrFlow



TdarrFlow defines a transcoding flow with plugins and edges.



_Appears in:_
- [TdarrSpec](#tdarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | id is the Tdarr document ID for this flow. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `name` _string_ | name is the display name of the flow. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `description` _string_ | description is a human-readable description of the flow. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `flowPlugins` _[RawExtension](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#rawextension-runtime-pkg) array_ | flowPlugins is the raw JSON definition of flow plugin nodes, as Tdarr<br />exports it. Quote the y key of each node's position ("y": 200): YAML 1.1<br />parsers, kubectl's included, read a bare y as the boolean true. |  | Required: \{\} <br /> |
| `flowEdges` _[RawExtension](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#rawextension-runtime-pkg) array_ | flowEdges is the raw JSON definition of flow edge connections. |  | Required: \{\} <br /> |


#### TdarrLibrary



TdarrLibrary defines a media library in Tdarr.



_Appears in:_
- [TdarrSpec](#tdarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | id is the Tdarr document ID for this library. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `name` _string_ | name is the display name of the library. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `folder` _string_ | folder is the source media folder path. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `cache` _string_ | cache is the transcode cache folder path. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `container` _string_ | container is the output container format (e.g., "mkv", "mp4"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `containerFilter` _string_ | containerFilter is the container filter for scanning (e.g., "mkv,mp4,avi"). |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `foldersToIgnore` _string_ | foldersToIgnore is a comma-separated list of folder names to skip while scanning. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `foldersToIgnoreCaseInsensitive` _boolean_ | foldersToIgnoreCaseInsensitive matches foldersToIgnore regardless of case. |  | Optional: \{\} <br /> |
| `filterResolutionsSkip` _string_ | filterResolutionsSkip is a comma-separated list of resolutions, such as<br />720p,1080p, whose files the server skips without sending them to a worker. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `filterCodecsSkip` _string_ | filterCodecsSkip is a comma-separated list of video codecs, such as<br />hevc,vp9, whose files the server skips. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `filterContainersSkip` _string_ | filterContainersSkip is a comma-separated list of containers, such as<br />mkv,mp4, whose files the server skips. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `filterHardlinked` _boolean_ | filterHardlinked skips files linked in more than one place until they no<br />longer are. |  | Optional: \{\} <br /> |
| `useFsEvents` _boolean_ | useFsEvents watches the folder with file system events instead of polling. |  | Optional: \{\} <br /> |
| `folderWatchScanInterval` _integer_ | folderWatchScanInterval is how often, in seconds, folder watching polls. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `folderWatching` _boolean_ | folderWatching enables watching the folder for new files. |  | Optional: \{\} <br /> |
| `processLibrary` _boolean_ | processLibrary enables processing of files in this library. |  | Optional: \{\} <br /> |
| `processTranscodes` _boolean_ | processTranscodes enables transcode jobs for this library. |  | Optional: \{\} <br /> |
| `processHealthChecks` _boolean_ | processHealthChecks enables health check jobs for this library. |  | Optional: \{\} <br /> |
| `healthCheckMode` _string_ | healthCheckMode selects the health check to run. quick uses HandBrake to<br />read file headers; thorough decodes every frame with FFmpeg. |  | Enum: [quick thorough] <br />Optional: \{\} <br /> |
| `decisionMode` _string_ | decisionMode selects how Tdarr decides what to do with a file. |  | Enum: [flows plugins] <br />Optional: \{\} <br /> |
| `scanOnStart` _boolean_ | scanOnStart enables scanning the library when Tdarr starts. |  | Optional: \{\} <br /> |
| `scheduledScanFindNew` _boolean_ | scheduledScanFindNew enables scheduled scans for new files. |  | Optional: \{\} <br /> |
| `scannerThreadCount` _integer_ | scannerThreadCount is the number of threads used for scanning. |  | Optional: \{\} <br /> |
| `ffmpeg` _boolean_ | ffmpeg enables using FFmpeg (true) vs HandBrake (false) for transcoding. |  | Optional: \{\} <br /> |
| `priority` _integer_ | priority is the processing priority for this library. Lower values are higher priority. |  | Optional: \{\} <br /> |
| `flowId` _string_ | flowId is the Tdarr flow this library processes files with. Flows are the<br />current mechanism; classic plugin stacks are the older alternative. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `pluginStack` _string array_ | pluginStack lists the community plugin IDs to run, in order, for libraries<br />not using a flow. |  | Optional: \{\} <br /> |
| `variables` _object (keys:string, values:string)_ | variables are user-defined library variables. Flows read them as<br />\{\{\{args.variables.user.<name>\}\}\}, which is how community flows are<br />configured without editing the flow itself. A variable removed from this<br />map is deleted from Tdarr only with spec.reconcile.prune, and only if the<br />operator created it. |  | Optional: \{\} <br /> |


#### TdarrSettings



TdarrSettings configures Tdarr's server-wide options.



_Appears in:_
- [TdarrSpec](#tdarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `autoAcceptTranscodes` _boolean_ | autoAcceptTranscodes moves a successful transcode into the library or<br />output folder without waiting in staging for review. |  | Optional: \{\} <br /> |
| `stagedFileLimit` _integer_ | stagedFileLimit is how many files can wait in staging for review. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `autoPauseIfCacheFull` _boolean_ | autoPauseIfCacheFull pauses every node when a library cache has less<br />free space than autoPauseIfCacheFullThreshold. |  | Optional: \{\} <br /> |
| `autoPauseIfCacheFullThreshold` _integer_ | autoPauseIfCacheFullThreshold is the free cache space, in GB, below which<br />autoPauseIfCacheFull pauses the nodes. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `workerStallDetector` _boolean_ | workerStallDetector cancels workers that stall. |  | Optional: \{\} <br /> |
| `workerStallDetectorInterval` _integer_ | workerStallDetectorInterval is how often, in seconds, workers are checked<br />for stalls. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `ignoreSchedules` _boolean_ | ignoreSchedules runs workers outside their libraries' schedules. |  | Optional: \{\} <br /> |
| `alternateLibraries` _boolean_ | alternateLibraries takes queued files from each library in turn. |  | Optional: \{\} <br /> |
| `prioritiseLibraries` _boolean_ | prioritiseLibraries takes queued files by library priority. |  | Optional: \{\} <br /> |
| `prioritiseTranscodes` _boolean_ | prioritiseTranscodes takes transcodes before health checks. |  | Optional: \{\} <br /> |
| `prioritiseHealthChecks` _boolean_ | prioritiseHealthChecks takes health checks before transcodes. |  | Optional: \{\} <br /> |
| `runMkvpropedit` _boolean_ | runMkvpropedit runs mkvpropedit on files before the plugins. |  | Optional: \{\} <br /> |
| `pluginAutoUpdate` _boolean_ | pluginAutoUpdate keeps the community plugins up to date. |  | Optional: \{\} <br /> |
| `jobHistorySizeLimitGB` _integer_ | jobHistorySizeLimitGB caps the job history, in GB. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `backupLimit` _integer_ | backupLimit is how many server backups are kept. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `logFullCliOutput` _boolean_ | logFullCliOutput logs the whole FFmpeg and HandBrake output of each job. |  | Optional: \{\} <br /> |
| `verboseLogs` _boolean_ | verboseLogs enables verbose logging. |  | Optional: \{\} <br /> |
| `notificationsDiscordWebhookSecretRef` _[SecretKeyRef](#secretkeyref)_ | notificationsDiscordWebhookSecretRef references the Discord webhook URL<br />notifications go to. |  | Optional: \{\} <br /> |
| `notificationsTranscodeSuccess` _boolean_ | notificationsTranscodeSuccess notifies when a transcode succeeds. |  | Optional: \{\} <br /> |
| `notificationsTranscodeError` _boolean_ | notificationsTranscodeError notifies when a transcode fails. |  | Optional: \{\} <br /> |
| `notificationsTranscodeCancelled` _boolean_ | notificationsTranscodeCancelled notifies when a transcode is cancelled. |  | Optional: \{\} <br /> |
| `notificationsHealthcheckSuccess` _boolean_ | notificationsHealthcheckSuccess notifies when a health check succeeds. |  | Optional: \{\} <br /> |
| `notificationsHealthcheckError` _boolean_ | notificationsHealthcheckError notifies when a health check fails. |  | Optional: \{\} <br /> |
| `notificationsHealthcheckCancelled` _boolean_ | notificationsHealthcheckCancelled notifies when a health check is cancelled. |  | Optional: \{\} <br /> |
| `notificationsRequireReview` _boolean_ | notificationsRequireReview notifies when a file waits in staging for review. |  | Optional: \{\} <br /> |
| `notificationsServerStarted` _boolean_ | notificationsServerStarted notifies when the server starts. |  | Optional: \{\} <br /> |
| `notificationsServerUpdateReady` _boolean_ | notificationsServerUpdateReady notifies when a server update is ready. |  | Optional: \{\} <br /> |
| `notificationsCustomText` _string_ | notificationsCustomText is added to every notification. |  | MinLength: 1 <br />Optional: \{\} <br /> |


#### TdarrSpec



TdarrSpec defines the desired state of Tdarr.



_Appears in:_
- [Tdarr](#tdarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connection` _[AppConnection](#appconnection)_ | connection defines how to connect to the Tdarr instance using an API key. |  | Required: \{\} <br /> |
| `reconcile` _[ReconcileConfig](#reconcileconfig)_ | reconcile configures reconciliation behavior (e.g., sync interval). |  | Optional: \{\} <br /> |
| `libraries` _[TdarrLibrary](#tdarrlibrary) array_ | libraries defines media libraries to manage in Tdarr. |  | Optional: \{\} <br /> |
| `flows` _[TdarrFlow](#tdarrflow) array_ | flows defines transcoding flows (plugins and edges). |  | Optional: \{\} <br /> |
| `workers` _[TdarrWorkers](#tdarrworkers) array_ | workers configures the number of worker threads on each Tdarr node. |  | Optional: \{\} <br /> |
| `settings` _[TdarrSettings](#tdarrsettings)_ | settings configures Tdarr's server-wide options. Pausing nodes is left to<br />whoever runs Tdarr. |  | Optional: \{\} <br /> |


#### TdarrStatus



TdarrStatus defines the observed state of Tdarr.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Tdarr](#tdarr)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | conditions represent the current state of the Tdarr resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | observedGeneration is the most recent generation observed. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | lastSyncTime is the timestamp of the last successful reconciliation. |  | Optional: \{\} <br /> |
| `managedResources` _[ManagedResource](#managedresource) array_ | managedResources records the resources this operator created, keyed by<br />resource type. Prune only removes entries listed here, so resources<br />created outside the operator are never deleted. |  | Optional: \{\} <br /> |


#### TdarrWorkers



TdarrWorkers configures the number of worker threads on one Tdarr node.



_Appears in:_
- [TdarrSpec](#tdarrspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeName` _string_ | nodeName is the name of the Tdarr node these limits are for. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `transcodeGPU` _integer_ | transcodeGPU is the number of GPU transcode workers. |  | Optional: \{\} <br /> |
| `transcodeCPU` _integer_ | transcodeCPU is the number of CPU transcode workers. |  | Optional: \{\} <br /> |
| `healthcheckGPU` _integer_ | healthcheckGPU is the number of GPU health check workers. |  | Optional: \{\} <br /> |
| `healthcheckCPU` _integer_ | healthcheckCPU is the number of CPU health check workers. |  | Optional: \{\} <br /> |


#### ValueSource



ValueSource names where a value is read from.



_Appears in:_
- [ConfigField](#configfield)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secretKeyRef` _[SecretKeyRef](#secretkeyref)_ | secretKeyRef reads the value from a key of a Secret in the resource's namespace. |  | Required: \{\} <br /> |


