package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SabnzbdSpec defines the desired state of Sabnzbd.
type SabnzbdSpec struct {
	// connection defines how to connect to the SABnzbd instance.
	// +required
	Connection commonv1alpha1.AppConnection `json:"connection,omitempty"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// general configures general SABnzbd settings.
	// +optional
	General *SabnzbdGeneral `json:"general,omitempty"`

	// servers defines Usenet server connections.
	// +optional
	// +listType=map
	// +listMapKey=name
	Servers []SabnzbdServer `json:"servers,omitempty"`

	// categories defines download categories.
	// +optional
	// +listType=map
	// +listMapKey=name
	Categories []SabnzbdCategory `json:"categories,omitempty"`

	// folders configures download and temp folder paths.
	// +optional
	Folders *SabnzbdFolders `json:"folders,omitempty"`

	// apprise sends SABnzbd's notifications through Apprise, to services such
	// as Discord or Telegram.
	// +optional
	Apprise SabnzbdApprise `json:"apprise,omitzero"`
}

// SabnzbdGeneral configures general SABnzbd settings.
type SabnzbdGeneral struct {
	// downloadSpeedLimit is the download speed limit (e.g., "100M", "0" for unlimited).
	// +optional
	// +kubebuilder:validation:MinLength=1
	DownloadSpeedLimit string `json:"downloadSpeedLimit,omitempty"`

	// pauseOnPostProcessing pauses downloading during post-processing.
	// +optional
	PauseOnPostProcessing *bool `json:"pauseOnPostProcessing,omitempty"`

	// autoSort sorts the queue by age whenever an NZB is added.
	// +optional
	AutoSort *bool `json:"autoSort,omitempty"`

	// preCheck enables NZB pre-check for completion.
	// +optional
	PreCheck *bool `json:"preCheck,omitempty"`

	// topOnly fetches articles only for the job at the top of the queue.
	// +optional
	TopOnly *bool `json:"topOnly,omitempty"`

	// failHopelessJobs fails a job as soon as it can't be completed.
	// +optional
	FailHopelessJobs *bool `json:"failHopelessJobs,omitempty"`

	// encryptedRarAction is what SABnzbd does with a job containing an encrypted RAR.
	// +optional
	// +kubebuilder:validation:Enum=off;pause;fail
	EncryptedRarAction string `json:"encryptedRarAction,omitempty"`

	// unwantedExtensions lists file extensions, without the dot, that
	// unwantedExtensionsAction applies to.
	// +optional
	// +listType=set
	UnwantedExtensions []string `json:"unwantedExtensions,omitempty"`

	// unwantedExtensionsMode decides whether unwantedExtensions lists the
	// extensions to act on or the only ones allowed.
	// +optional
	// +kubebuilder:validation:Enum=blocklist;allowlist
	UnwantedExtensionsMode string `json:"unwantedExtensionsMode,omitempty"`

	// unwantedExtensionsAction is what SABnzbd does with a job containing an
	// unwanted extension.
	// +optional
	// +kubebuilder:validation:Enum=off;pause;fail
	UnwantedExtensionsAction string `json:"unwantedExtensionsAction,omitempty"`

	// directUnpack unpacks a job while it's still downloading.
	// +optional
	DirectUnpack *bool `json:"directUnpack,omitempty"`

	// enableAllPar downloads every par2 file, not only those a repair needs.
	// +optional
	EnableAllPar *bool `json:"enableAllPar,omitempty"`

	// sfvCheck verifies jobs without par2 files against their SFV files.
	// +optional
	SFVCheck *bool `json:"sfvCheck,omitempty"`

	// safePostProcessing post-processes only jobs that passed verification.
	// +optional
	SafePostProcessing *bool `json:"safePostProcessing,omitempty"`

	// enableRecursive unpacks archives found inside archives.
	// +optional
	EnableRecursive *bool `json:"enableRecursive,omitempty"`

	// flatUnpack ignores the folders inside archives when unpacking.
	// +optional
	FlatUnpack *bool `json:"flatUnpack,omitempty"`

	// ignoreSamples skips sample files.
	// +optional
	IgnoreSamples *bool `json:"ignoreSamples,omitempty"`

	// deobfuscateFinalFilenames renames obfuscated files after the job.
	// +optional
	DeobfuscateFinalFilenames *bool `json:"deobfuscateFinalFilenames,omitempty"`

	// scriptCanFail lets a job's post-processing script fail without failing the job.
	// +optional
	ScriptCanFail *bool `json:"scriptCanFail,omitempty"`

	// cleanupList lists file extensions, without the dot, to delete after a job.
	// +optional
	// +listType=set
	CleanupList []string `json:"cleanupList,omitempty"`

	// historyRetention decides how long jobs stay in the history.
	// +optional
	// +kubebuilder:validation:Enum=all;number-archive;number-delete;days-archive;days-delete;all-archive;all-delete
	HistoryRetention string `json:"historyRetention,omitempty"`

	// historyRetentionNumber is the number of jobs or days historyRetention keeps.
	// +optional
	// +kubebuilder:validation:Minimum=0
	HistoryRetentionNumber *int32 `json:"historyRetentionNumber,omitempty"`

	// permissions are the octal permissions set on completed downloads, such as 775.
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-7]{3,4}$`
	// +kubebuilder:validation:MinLength=1
	Permissions string `json:"permissions,omitempty"`

	// downloadFree is the free space the incomplete folder needs to keep
	// downloading, such as 500M.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DownloadFree string `json:"downloadFree,omitempty"`

	// completeFree is the free space the complete folder needs to keep
	// downloading, such as 10G.
	// +optional
	// +kubebuilder:validation:MinLength=1
	CompleteFree string `json:"completeFree,omitempty"`

	// cacheLimit is the memory used to cache articles, such as 1G.
	// +optional
	// +kubebuilder:validation:MinLength=1
	CacheLimit string `json:"cacheLimit,omitempty"`

	// propagationDelay holds back jobs posted less than this many minutes ago.
	// +optional
	// +kubebuilder:validation:Minimum=0
	PropagationDelay *int32 `json:"propagationDelay,omitempty"`

	// hostWhitelist lists the host names SABnzbd answers on besides IP
	// addresses, localhost and names ending in .local, which cover
	// *.svc.cluster.local. It replaces SABnzbd's list, so it must include the
	// host in connection.url unless that is one of those.
	// +optional
	// +listType=set
	HostWhitelist []string `json:"hostWhitelist,omitempty"`
}

// SabnzbdServer defines a Usenet server connection.
type SabnzbdServer struct {
	// name is the display name of the server.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// host is the hostname of the Usenet server.
	// +required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host,omitempty"`

	// port is the port number.
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// ssl enables SSL/TLS connection.
	// +optional
	SSL *bool `json:"ssl,omitempty"`

	// usernameSecretRef references a Secret containing the username.
	// +optional
	UsernameSecretRef *commonv1alpha1.SecretKeyRef `json:"usernameSecretRef,omitempty"`

	// passwordSecretRef references a Secret containing the password.
	// +optional
	PasswordSecretRef *commonv1alpha1.SecretKeyRef `json:"passwordSecretRef,omitempty"`

	// connections is the number of simultaneous connections.
	// +optional
	Connections *int32 `json:"connections,omitempty"`

	// priority is the server priority. 0 = highest.
	// +optional
	Priority *int32 `json:"priority,omitempty"`

	// retention is the server retention in days. 0 = unlimited.
	// +optional
	Retention *int32 `json:"retention,omitempty"`

	// enable controls whether this server is active. A new server is enabled
	// unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// sslVerify is how strictly the server's certificate is checked when ssl is on.
	// +optional
	// +kubebuilder:validation:Enum=strict;medium;minimal;disabled
	SSLVerify string `json:"sslVerify,omitempty"`

	// optional lets SABnzbd stop using the server for a while when it fails.
	// +optional
	Optional *bool `json:"optional,omitempty"`

	// required makes SABnzbd wait for the server when it fails instead of
	// skipping it.
	// +optional
	Required *bool `json:"required,omitempty"`

	// timeout is how many seconds SABnzbd waits for the server to answer.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Timeout int32 `json:"timeout,omitempty"`
}

// SabnzbdCategory defines a download category.
type SabnzbdCategory struct {
	// name is the category name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// dir is the subdirectory for this category (relative to complete dir).
	// +optional
	// +kubebuilder:validation:MinLength=1
	Dir string `json:"dir,omitempty"`

	// script is the post-processing script for this category.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Script string `json:"script,omitempty"`

	// priority is the download priority. default uses SABnzbd's default
	// category's priority.
	// +optional
	// +kubebuilder:validation:Enum=default;paused;low;normal;high;force
	Priority string `json:"priority,omitempty"`

	// postProcessing is how far SABnzbd processes the category's jobs: download
	// only, repair, unpack, or unpack and delete the archives.
	// +optional
	// +kubebuilder:validation:Enum=download;repair;unpack;delete
	PostProcessing string `json:"postProcessing,omitempty"`
}

// SabnzbdApprise configures SABnzbd's Apprise notifications.
// +kubebuilder:validation:MinProperties=1
type SabnzbdApprise struct {
	// enabled turns Apprise notifications on.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// urlsSecretRef is the Secret key holding the Apprise URLs to notify,
	// separated by commas, such as discord://webhook_id/webhook_token.
	// +optional
	URLsSecretRef *commonv1alpha1.SecretKeyRef `json:"urlsSecretRef,omitempty"`

	// categories limits notifications to jobs in these categories; * is every
	// category.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	Categories []string `json:"categories,omitempty"`

	// events are the events to notify about. Those listed are turned on and the
	// rest off.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:Enum=startup;download;pauseResume;pp;complete;failed;diskFull;quota;newLogin;warning;error;queueDone;other
	Events []string `json:"events,omitempty"`
}

// SabnzbdFolders configures folder paths.
type SabnzbdFolders struct {
	// completeDir is the path for completed downloads.
	// +optional
	// +kubebuilder:validation:MinLength=1
	CompleteDir string `json:"completeDir,omitempty"`

	// incompleteDir is the path for incomplete downloads.
	// +optional
	// +kubebuilder:validation:MinLength=1
	IncompleteDir string `json:"incompleteDir,omitempty"`

	// tempDownloadDir is the temporary download path.
	// +optional
	// +kubebuilder:validation:MinLength=1
	TempDownloadDir string `json:"tempDownloadDir,omitempty"`

	// nzbBackupDir is the path for NZB backups.
	// +optional
	// +kubebuilder:validation:MinLength=1
	NzbBackupDir string `json:"nzbBackupDir,omitempty"`

	// scriptDir is the path for scripts.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ScriptDir string `json:"scriptDir,omitempty"`
}

// SabnzbdStatus defines the observed state of Sabnzbd.
// +kubebuilder:validation:MinProperties=1
type SabnzbdStatus struct {
	// conditions represent the current state of the Sabnzbd resource.
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

// Sabnzbd configures a Sabnzbd instance the operator connects to.
type Sabnzbd struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Sabnzbd.
	// +required
	Spec SabnzbdSpec `json:"spec,omitzero"`

	// status is the observed state of the Sabnzbd.
	// +optional
	Status SabnzbdStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SabnzbdList contains a list of Sabnzbd.
type SabnzbdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Sabnzbd `json:"items"`
}

func (c *Sabnzbd) GetConditions() *[]metav1.Condition                  { return &c.Status.Conditions }
func (c *Sabnzbd) GetObservedGeneration() *int64                       { return &c.Status.ObservedGeneration }
func (c *Sabnzbd) GetLastSyncTime() **metav1.Time                      { return &c.Status.LastSyncTime }
func (c *Sabnzbd) GetReconcileConfig() *commonv1alpha1.ReconcileConfig { return c.Spec.Reconcile }
func (c *Sabnzbd) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Sabnzbd) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
