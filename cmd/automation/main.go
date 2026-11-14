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
	automationv1alpha1 "github.com/kyleseneker/media-operator/api/automation/v1alpha1"
	"github.com/kyleseneker/media-operator/internal/controller/automation"
	"github.com/kyleseneker/media-operator/internal/operator"
)

func main() {
	operator.Run("automation", automationv1alpha1.AddToScheme,
		operator.Controller{Name: "autobrr", New: func(d operator.Deps) operator.Reconciler {
			return &automation.AutobrrConfigReconciler{Client: d.Client, Scheme: d.Scheme, Recorder: d.Recorder}
		}},
	)
}
