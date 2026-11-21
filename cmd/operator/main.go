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

package main

import (
	"k8s.io/apimachinery/pkg/runtime"

	automationv1alpha1 "github.com/kyleseneker/arrchitect/api/automation/v1alpha1"
	curationv1alpha1 "github.com/kyleseneker/arrchitect/api/curation/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	transcodev1alpha1 "github.com/kyleseneker/arrchitect/api/transcode/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/automation"
	"github.com/kyleseneker/arrchitect/internal/controller/curation"
	"github.com/kyleseneker/arrchitect/internal/controller/downloads"
	"github.com/kyleseneker/arrchitect/internal/controller/indexers"
	"github.com/kyleseneker/arrchitect/internal/controller/mediaservers"
	"github.com/kyleseneker/arrchitect/internal/controller/pvr"
	"github.com/kyleseneker/arrchitect/internal/controller/requests"
	"github.com/kyleseneker/arrchitect/internal/controller/subtitles"
	"github.com/kyleseneker/arrchitect/internal/controller/transcode"
	"github.com/kyleseneker/arrchitect/internal/operator"
)

func main() {
	operator.Run([]func(*runtime.Scheme) error{
		pvrv1alpha1.AddToScheme,
		indexersv1alpha1.AddToScheme,
		subtitlesv1alpha1.AddToScheme,
		downloadsv1alpha1.AddToScheme,
		mediaserversv1alpha1.AddToScheme,
		requestsv1alpha1.AddToScheme,
		curationv1alpha1.AddToScheme,
		transcodev1alpha1.AddToScheme,
		automationv1alpha1.AddToScheme,
	},
		operator.Controller{Name: "sonarr", New: func(d operator.Deps) operator.Reconciler {
			return operator.Reconcilers{
				&pvr.SonarrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder},
				&pvr.SonarrIndexerReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder},
				&pvr.SonarrDownloadClientReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder, Apps: d.Apps},
			}
		}},
		operator.Controller{Name: "radarr", New: func(d operator.Deps) operator.Reconciler {
			return operator.Reconcilers{
				&pvr.RadarrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder},
				&pvr.RadarrIndexerReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder},
				&pvr.RadarrDownloadClientReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder, Apps: d.Apps},
			}
		}},
		operator.Controller{Name: "lidarr", New: func(d operator.Deps) operator.Reconciler {
			return operator.Reconcilers{
				&pvr.LidarrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder},
				&pvr.LidarrIndexerReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder},
				&pvr.LidarrDownloadClientReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder, Apps: d.Apps},
			}
		}},
		operator.Controller{Name: "readarr", New: func(d operator.Deps) operator.Reconciler {
			return operator.Reconcilers{
				&pvr.ReadarrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder},
				&pvr.ReadarrIndexerReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder},
				&pvr.ReadarrDownloadClientReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder, Apps: d.Apps},
			}
		}},
		operator.Controller{Name: "prowlarr", New: func(d operator.Deps) operator.Reconciler {
			return &indexers.ProwlarrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder, Apps: d.Apps}
		}},
		operator.Controller{Name: "flaresolverr", New: func(d operator.Deps) operator.Reconciler {
			return &indexers.FlareSolverrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder}
		}},
		operator.Controller{Name: "bazarr", New: func(d operator.Deps) operator.Reconciler {
			return &subtitles.BazarrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder, Apps: d.Apps}
		}},
		operator.Controller{Name: "qbittorrent", New: func(d operator.Deps) operator.Reconciler {
			return &downloads.QBittorrentReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder}
		}},
		operator.Controller{Name: "sabnzbd", New: func(d operator.Deps) operator.Reconciler {
			return &downloads.SabnzbdReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder}
		}},
		operator.Controller{Name: "jellyfin", New: func(d operator.Deps) operator.Reconciler {
			return &mediaservers.JellyfinReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder}
		}},
		operator.Controller{Name: "plex", New: func(d operator.Deps) operator.Reconciler {
			return &mediaservers.PlexReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder}
		}},
		operator.Controller{Name: "seerr", New: func(d operator.Deps) operator.Reconciler {
			return &requests.SeerrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder, Apps: d.Apps}
		}},
		operator.Controller{Name: "maintainerr", New: func(d operator.Deps) operator.Reconciler {
			return &curation.MaintainerrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder, Apps: d.Apps}
		}},
		operator.Controller{Name: "tdarr", New: func(d operator.Deps) operator.Reconciler {
			return &transcode.TdarrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder}
		}},
		operator.Controller{Name: "autobrr", New: func(d operator.Deps) operator.Reconciler {
			return &automation.AutobrrReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder, Apps: d.Apps}
		}},
	)
}
